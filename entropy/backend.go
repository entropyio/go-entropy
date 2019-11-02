package entropy

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"

	"github.com/entropyio/go-entropy/accounts"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/bloombits"
	"github.com/entropyio/go-entropy/blockchain/genesis"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"

	"github.com/entropyio/go-entropy/consensus/clique"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/entropy/entropyapi"
	"github.com/entropyio/go-entropy/entropy/filters"
	"github.com/entropyio/go-entropy/entropy/gasprice"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/miner"
	"github.com/entropyio/go-entropy/rpc"
	"github.com/entropyio/go-entropy/server/node"
	"github.com/entropyio/go-entropy/server/p2p"
	"github.com/entropyio/go-entropy/server/p2p/enr"
	"sync/atomic"
)

var log = logger.NewLogger("[entropy]")

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	APIs() []rpc.API
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *blockchain.ChainIndexer)
	//SetContractBackend(bind.ContractBackend)
}

// Entropy implements the Entropy full node service.
type Entropy struct {
	config *Config

	// Channel for shutting down the service
	shutdownChan chan bool

	// Handlers
	txPool          *blockchain.TxPool
	blockchain      *blockchain.BlockChain
	protocolManager *ProtocolManager
	lesServer       LesServer

	// DB interfaces
	chainDb database.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *blockchain.ChainIndexer       // Bloom indexer operating during block imports

	APIBackend *EntropyAPIBackend

	miner       *miner.Miner
	gasPrice    *big.Int
	entropyBase common.Address

	networkID     uint64
	netRPCService *entropyapi.PublicNetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and entropybase)
}

func (s *Entropy) AddLesServer(ls LesServer) {
	s.lesServer = ls
	ls.SetBloomBitsIndexer(s.bloomIndexer)
}

// SetClient sets a rpc client which connecting to our local node.
//func (s *Entropy) SetContractBackend(backend bind.ContractBackend) {
//	// Pass the rpc client to les server if it is enabled.
//	if s.lesServer != nil {
//		s.lesServer.SetContractBackend(backend)
//	}
//}

// New creates a new Entropy object (including the
// initialisation of the common Entropy object)
func New(ctx *node.ServiceContext, configObj *Config) (*Entropy, error) {
	// Ensure configuration values are compatible and sane
	if configObj.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run entropy.Entropy in light sync mode, use les.LightEntropy")
	}
	if !configObj.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", configObj.SyncMode)
	}
	if configObj.Miner.GasPrice == nil || configObj.Miner.GasPrice.Cmp(common.Big0) <= 0 {
		log.Warning("Sanitizing invalid miner gas price", "provided", configObj.Miner.GasPrice, "updated", DefaultConfig.Miner.GasPrice)
		configObj.Miner.GasPrice = new(big.Int).Set(DefaultConfig.Miner.GasPrice)
	}
	if configObj.NoPruning && configObj.TrieDirtyCache > 0 {
		configObj.TrieCleanCache += configObj.TrieDirtyCache
		configObj.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(configObj.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(configObj.TrieDirtyCache)*1024*1024)

	// Assemble the Ethereum object
	chainDb, err := ctx.OpenDatabaseWithFreezer("chaindata", configObj.DatabaseCache, configObj.DatabaseHandles, configObj.DatabaseFreezer, "entropy/db/chaindata/")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := genesis.SetupGenesisBlock(chainDb, configObj.Genesis)
	if _, ok := genesisErr.(*config.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration.", chainConfig)

	entropy := &Entropy{
		config:         configObj,
		chainDb:        chainDb,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, chainConfig, &configObj.Ethash, configObj.Miner.Notify, configObj.Miner.Noverify, chainDb),
		shutdownChan:   make(chan bool),
		networkID:      configObj.NetworkId,
		gasPrice:       configObj.Miner.GasPrice,
		entropyBase:    configObj.Miner.EntropyBase,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, config.BloomBitsBlocks, config.BloomConfirms),
	}

	bcVersion := mapper.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Infof("Initialising Entropy protocol. versions=%s, network=%d, dbVersion=%s", ProtocolVersions, configObj.NetworkId, dbVer)

	if !configObj.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > blockchain.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Geth %s only supports v%d", *bcVersion, config.VersionWithMeta, blockchain.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < blockchain.BlockChainVersion {
			log.Warning("Upgrade blockchain database version", "from", dbVer, "to", blockchain.BlockChainVersion)
			mapper.WriteDatabaseVersion(chainDb, blockchain.BlockChainVersion)
		}
	}
	var (
		vmConfig = evm.Config{
			EnablePreimageRecording: configObj.EnablePreimageRecording,
			EWASMInterpreter:        configObj.EWASMInterpreter,
			EVMInterpreter:          configObj.EVMInterpreter,
		}
		cacheConfig = &blockchain.CacheConfig{
			TrieCleanLimit:      configObj.TrieCleanCache,
			TrieCleanNoPrefetch: configObj.NoPrefetch,
			TrieDirtyLimit:      configObj.TrieDirtyCache,
			TrieDirtyDisabled:   configObj.NoPruning,
			TrieTimeLimit:       configObj.TrieTimeout,
		}
	)
	entropy.blockchain, err = blockchain.NewBlockChain(chainDb, cacheConfig, chainConfig, entropy.engine, vmConfig, entropy.shouldPreserve)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*config.ConfigCompatError); ok {
		log.Warning("Rewinding chain to upgrade configuration", "err", compat)
		entropy.blockchain.SetHead(compat.RewindTo)
		mapper.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	entropy.bloomIndexer.Start(entropy.blockchain)

	if configObj.TxPool.Journal != "" {
		configObj.TxPool.Journal = ctx.ResolvePath(configObj.TxPool.Journal)
	}
	entropy.txPool = blockchain.NewTxPool(configObj.TxPool, chainConfig, entropy.blockchain)

	// Permit the downloader to use the trie cache allowance during fast sync
	cacheLimit := cacheConfig.TrieCleanLimit + cacheConfig.TrieDirtyLimit
	checkpoint := configObj.Checkpoint
	if checkpoint == nil {
		checkpoint = config.TrustedCheckpoints[genesisHash]
	}
	if entropy.protocolManager, err = NewProtocolManager(chainConfig, checkpoint, configObj.SyncMode, configObj.NetworkId, entropy.eventMux, entropy.txPool, entropy.engine, entropy.blockchain, chainDb, cacheLimit, configObj.Whitelist); err != nil {
		return nil, err
	}

	entropy.miner = miner.New(entropy, &configObj.Miner, chainConfig, entropy.EventMux(), entropy.engine, entropy.isLocalBlock)
	entropy.miner.SetExtra(makeExtraData(configObj.Miner.ExtraData))

	entropy.APIBackend = &EntropyAPIBackend{ctx.ExtRPCEnabled(), entropy, nil}
	gpoParams := configObj.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = configObj.Miner.GasPrice
	}
	entropy.APIBackend.gpo = gasprice.NewOracle(entropy.APIBackend, gpoParams)

	return entropy, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlputil.EncodeToBytes([]interface{}{
			uint(config.VersionMajor<<16 | config.VersionMinor<<8 | config.VersionPatch),
			"entropy",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > config.MaximumExtraDataSize {
		log.Warning("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", config.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Ethereum service
func CreateConsensusEngine(ctx *node.ServiceContext, chainConfig *config.ChainConfig, ethashConfig *ethash.Config, notify []string, noverify bool, db database.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
	if chainConfig.Clique != nil {
		return clique.New(chainConfig.Clique, db)
	}
	// Otherwise assume proof-of-work
	switch ethashConfig.PowMode {
	case ethash.ModeFake:
		log.Warning("Ethash used in fake mode")
		return ethash.NewFaker()
	case ethash.ModeTest:
		log.Warning("Ethash used in test mode")
		return ethash.NewTester(nil, noverify)
	case ethash.ModeShared:
		log.Warning("Ethash used in shared mode")
		return ethash.NewShared()
	default:
		engine := ethash.New(ethash.Config{
			CacheDir:       ctx.ResolvePath(ethashConfig.CacheDir),
			CachesInMem:    ethashConfig.CachesInMem,
			CachesOnDisk:   ethashConfig.CachesOnDisk,
			DatasetDir:     ethashConfig.DatasetDir,
			DatasetsInMem:  ethashConfig.DatasetsInMem,
			DatasetsOnDisk: ethashConfig.DatasetsOnDisk,
		}, notify, noverify)
		//	engine.SetThreads(-1) // Disable CPU mining
		engine.SetThreads(1) // FIXME: 1 for test
		log.Warning("POW used in product mode. engine threads number: 1")
		return engine
	}
}

// APIs return the collection of RPC services the Entropy package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Entropy) APIs() []rpc.API {
	apis := entropyapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the les server
	if s.lesServer != nil {
		apis = append(apis, s.lesServer.APIs()...)
	}
	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append any APIs exposed explicitly by the les server
	if s.lesServer != nil {
		apis = append(apis, s.lesServer.APIs()...)
	}

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewPublicEntropyAPI(s),
			Public:    true,
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.APIBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Entropy) ResetWithGenesisBlock(gb *model.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Entropy) EntropyBase() (eb common.Address, err error) {
	s.lock.RLock()
	entropyBase := s.entropyBase
	s.lock.RUnlock()

	if entropyBase != (common.Address{}) {
		return entropyBase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			entropyBase := accounts[0].Address

			s.lock.Lock()
			s.entropyBase = entropyBase
			s.lock.Unlock()

			log.Infof("EntropyBase automatically configured address: %X", entropyBase)
			return entropyBase, nil
		}
	}
	return common.Address{}, fmt.Errorf("entropybase must be explicitly specified")
}

// isLocalBlock checks whether the specified block is mined
// by local miner accounts.
//
// We regard two types of accounts as local miner account: etherbase
// and accounts specified via `txpool.locals` flag.
func (s *Entropy) isLocalBlock(block *model.Block) bool {
	author, err := s.engine.Author(block.Header())
	if err != nil {
		log.Warning("Failed to retrieve block author", "number", block.NumberU64(), "hash", block.Hash(), "err", err)
		return false
	}
	// Check whether the given address is etherbase.
	s.lock.RLock()
	entropyBase := s.entropyBase
	s.lock.RUnlock()
	if author == entropyBase {
		return true
	}
	// Check whether the given address is specified by `txpool.local`
	// CLI flag.
	for _, accountObj := range s.config.TxPool.Locals {
		if accountObj == author {
			return true
		}
	}
	return false
}

// shouldPreserve checks whether we should preserve the given block
// during the chain reorg depending on whether the author of block
// is a local account.
func (s *Entropy) shouldPreserve(block *model.Block) bool {
	// The reason we need to disable the self-reorg preserving for clique
	// is it can be probable to introduce a deadlock.
	//
	// e.g. If there are 7 available signers
	//
	// r1   A
	// r2     B
	// r3       C
	// r4         D
	// r5   A      [X] F G
	// r6    [X]
	//
	// In the round5, the inturn signer E is offline, so the worst case
	// is A, F and G sign the block of round5 and reject the block of opponents
	// and in the round6, the last available signer B is offline, the whole
	// network is stuck.
	if _, ok := s.engine.(*clique.Clique); ok {
		return false
	}
	return s.isLocalBlock(block)
}

// SetEtherbase sets the mining reward address.
func (s *Entropy) SetEntropyBase(entropyBase common.Address) {
	s.lock.Lock()
	s.entropyBase = entropyBase
	s.lock.Unlock()

	s.miner.SetEntropyBase(entropyBase)
}

// StartMining starts the miner with the given number of CPU threads. If mining
// is already running, this method adjust the number of threads allowed to use
// and updates the minimum price required by the transaction pool.
func (s *Entropy) StartMining(threads int) error {
	// Update the thread count within the consensus engine
	type threaded interface {
		SetThreads(threads int)
	}

	threads = 1 // FIXME: 1 thread for test
	if th, ok := s.engine.(threaded); ok {
		log.Infof("Updated mining threads. threads=%d", threads)
		if threads == 0 {
			threads = -1 // Disable the miner from within
		}
		th.SetThreads(threads)
	}
	// If the miner was not running, initialize it
	if !s.IsMining() {
		// Propagate the initial price point to the transaction pool
		s.lock.RLock()
		price := s.gasPrice
		s.lock.RUnlock()
		s.txPool.SetGasPrice(price)

		// Configure the local mining address
		eb, err := s.EntropyBase()
		if err != nil {
			log.Error("Cannot start mining without etherbase", "err", err)
			return fmt.Errorf("etherbase missing: %v", err)
		}
		if clique, ok := s.engine.(*clique.Clique); ok {
			wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
			if wallet == nil || err != nil {
				log.Error("Etherbase account unavailable locally", "err", err)
				return fmt.Errorf("signer missing: %v", err)
			}
			clique.Authorize(eb, wallet.SignData)
		}
		// If mining is started, we can disable the transaction rejection mechanism
		// introduced to speed sync times.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)

		go s.miner.Start(eb)
	}
	return nil
}

// StopMining terminates the miner, both at the consensus engine level as well as
// at the block creation level.
func (s *Entropy) StopMining() {
	// Update the thread count within the consensus engine
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := s.engine.(threaded); ok {
		th.SetThreads(-1)
	}
	// Stop the block creating itself
	s.miner.Stop()
}

func (s *Entropy) IsMining() bool      { return s.miner.Mining() }
func (s *Entropy) Miner() *miner.Miner { return s.miner }

func (s *Entropy) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *Entropy) BlockChain() *blockchain.BlockChain { return s.blockchain }
func (s *Entropy) TxPool() *blockchain.TxPool         { return s.txPool }
func (s *Entropy) EventMux() *event.TypeMux           { return s.eventMux }
func (s *Entropy) Engine() consensus.Engine           { return s.engine }
func (s *Entropy) ChainDb() database.Database         { return s.chainDb }
func (s *Entropy) IsListening() bool                  { return true } // Always listening
func (s *Entropy) EthVersion() int                    { return int(ProtocolVersions[0]) }
func (s *Entropy) NetVersion() uint64                 { return s.networkID }
func (s *Entropy) Downloader() *downloader.Downloader { return s.protocolManager.downloader }
func (s *Entropy) Synced() bool                       { return atomic.LoadUint32(&s.protocolManager.acceptTxs) == 1 }
func (s *Entropy) ArchiveMode() bool                  { return s.config.NoPruning }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Entropy) Protocols() []p2p.Protocol {
	protos := make([]p2p.Protocol, len(ProtocolVersions))
	for i, vsn := range ProtocolVersions {
		protos[i] = s.protocolManager.makeProtocol(vsn)
		protos[i].Attributes = []enr.Entry{s.currentEthEntry()}
	}
	if s.lesServer != nil {
		protos = append(protos, s.lesServer.Protocols()...)
	}
	return protos
}

// Start implements node.Service, starting all internal goroutines needed by the
// Entropy protocol implementation.
func (s *Entropy) Start(srvr *p2p.Server) error {
	s.startEthEntryUpdate(srvr.LocalNode())
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(config.BloomBitsBlocks)

	// Start the RPC service
	s.netRPCService = entropyapi.NewPublicNetAPI(srvr, s.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers >= srvr.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, srvr.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.protocolManager.Start(maxPeers)
	if s.lesServer != nil {
		s.lesServer.Start(srvr)
	}
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Entropy protocol.
func (s *Entropy) Stop() error {
	s.bloomIndexer.Close()
	s.blockchain.Stop()
	s.engine.Close()
	s.protocolManager.Stop()
	if s.lesServer != nil {
		s.lesServer.Stop()
	}
	s.txPool.Stop()
	s.miner.Stop()
	s.eventMux.Stop()

	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
