package entropy

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"

	"github.com/entropyio/go-entropy/account"
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

	"github.com/entropyio/go-entropy/consensus/claude"
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
	"sync/atomic"
)

var log = logger.NewLogger("[entropy]")

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *blockchain.ChainIndexer)
}

// Entropy implements the Entropy full node service.
type Entropy struct {
	config      *Config
	chainConfig *config.ChainConfig

	// Channel for shutting down the service
	shutdownChan chan bool // Channel for shutting down the Entropy

	// Handlers
	txPool          *blockchain.TxPool
	blockchain      *blockchain.BlockChain
	protocolManager *ProtocolManager
	lesServer       LesServer

	// DB interfaces
	chainDb database.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *account.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *blockchain.ChainIndexer       // Bloom indexer operating during block imports

	APIBackend *EthAPIBackend

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

// New creates a new Entropy object (including the
// initialisation of the common Entropy object)
func New(ctx *node.ServiceContext, configObj *Config) (*Entropy, error) {
	if configObj.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run entropy.Entropy in light sync mode, use les.LightEntropy")
	}
	if !configObj.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", configObj.SyncMode)
	}
	chainDb, err := CreateDB(ctx, configObj, "chaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := genesis.SetupGenesisBlock(chainDb, configObj.Genesis)
	if _, ok := genesisErr.(*config.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration.", chainConfig)

	eth := &Entropy{
		config:         configObj,
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, chainConfig, &configObj.Ethash, configObj.MinerNotify, chainDb),
		shutdownChan:   make(chan bool),
		networkID:      configObj.NetworkId,
		gasPrice:       configObj.GasPrice,
		entropyBase:    configObj.EntropyBase,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, config.BloomBitsBlocks),
	}

	log.Infof("Initialising Entropy protocol. versions=%s, network=%d", ProtocolVersions, configObj.NetworkId)

	if !configObj.SkipBcVersionCheck {
		bcVersion := mapper.ReadDatabaseVersion(chainDb)
		if bcVersion != blockchain.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run entropy upgradedb.\n", bcVersion, blockchain.BlockChainVersion)
		}
		mapper.WriteDatabaseVersion(chainDb, blockchain.BlockChainVersion)
	}
	var (
		vmConfig    = evm.Config{EnablePreimageRecording: configObj.EnablePreimageRecording}
		cacheConfig = &blockchain.CacheConfig{Disabled: configObj.NoPruning, TrieNodeLimit: configObj.TrieCache, TrieTimeLimit: configObj.TrieTimeout}
	)
	eth.blockchain, err = blockchain.NewBlockChain(chainDb, cacheConfig, eth.chainConfig, eth.engine, vmConfig)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*config.ConfigCompatError); ok {
		log.Warning("Rewinding chain to upgrade configuration", "err", compat)
		eth.blockchain.SetHead(compat.RewindTo)
		mapper.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	eth.bloomIndexer.Start(eth.blockchain)

	if configObj.TxPool.Journal != "" {
		configObj.TxPool.Journal = ctx.ResolvePath(configObj.TxPool.Journal)
	}
	eth.txPool = blockchain.NewTxPool(configObj.TxPool, eth.chainConfig, eth.blockchain)

	if eth.protocolManager, err = NewProtocolManager(eth.chainConfig, configObj.SyncMode, configObj.NetworkId, eth.eventMux, eth.txPool, eth.engine, eth.blockchain, chainDb); err != nil {
		return nil, err
	}
	eth.miner = miner.New(eth, eth.chainConfig, eth.EventMux(), eth.engine)
	eth.miner.SetExtra(makeExtraData(configObj.ExtraData))

	eth.APIBackend = &EthAPIBackend{eth, nil}
	gpoParams := configObj.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = configObj.GasPrice
	}
	eth.APIBackend.gpo = gasprice.NewOracle(eth.APIBackend, gpoParams)

	return eth, nil
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

// CreateDB creates the chain database.
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (database.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*database.LDBDatabase); ok {
		db.Meter("entropy/db/chaindata/")
	}
	return db, nil
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Entropy service
func CreateConsensusEngine(ctx *node.ServiceContext, chainConfig *config.ChainConfig, ethashConfig *ethash.Config, notify []string, db database.Database) consensus.Engine {
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
		return ethash.NewTester(nil)
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
		}, notify)
		//	engine.SetThreads(-1) // Disable CPU mining
		engine.SetThreads(1) // TODO: 1 for test
		log.Warning("POW used in product mode. engine threads number: 1")
		return engine
	}
}

// APIs return the collection of RPC services the Entropy package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Entropy) APIs() []rpc.API {
	apis := entropyapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

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
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
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

			//log.Infof("EntropyBase automatically configured address: %x", entropybase)
			return entropyBase, nil
		}
	}
	return common.Address{}, fmt.Errorf("entropybase must be explicitly specified")
}

// SetEntropyBase sets the mining reward address.
func (s *Entropy) SetEntropyBase(entropyBase common.Address) {
	s.lock.Lock()
	s.entropyBase = entropyBase
	s.lock.Unlock()

	s.miner.SetEntropyBase(entropyBase)
}

func (s *Entropy) StartMining(local bool) error {
	eb, err := s.EntropyBase()
	log.Info("StartMining with eb: ", eb)
	if err != nil {
		log.Error("Cannot start mining without entropyBase", "err", err)
		return fmt.Errorf("entropyBase missing: %v", err)
	}
	if cliqueEngine, ok := s.engine.(*clique.Clique); ok {
		wallet, err := s.accountManager.Find(account.Account{Address: eb})
		if wallet == nil || err != nil {
			log.Error("EntropyBase account unavailable locally", "err", err)
			return fmt.Errorf("signer missing: %v", err)
		}
		cliqueEngine.Authorize(eb, wallet.SignHash)
	}
	// add Claude dpos support
	if claudeEngine, ok := s.engine.(*claude.Claude); ok {
		wallet, err := s.accountManager.Find(account.Account{Address: eb})
		if wallet == nil || err != nil {
			log.Error("EntropyBase account unavailable locally", "err", err)
			return fmt.Errorf("signer missing: %v", err)
		}
		claudeEngine.Authorize(eb, wallet.SignHash)
	}
	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so none will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)
	}
	go s.miner.Start(eb)
	return nil
}

func (s *Entropy) StopMining()         { s.miner.Stop() }
func (s *Entropy) IsMining() bool      { return s.miner.Mining() }
func (s *Entropy) Miner() *miner.Miner { return s.miner }

func (s *Entropy) AccountManager() *account.Manager   { return s.accountManager }
func (s *Entropy) BlockChain() *blockchain.BlockChain { return s.blockchain }
func (s *Entropy) TxPool() *blockchain.TxPool         { return s.txPool }
func (s *Entropy) EventMux() *event.TypeMux           { return s.eventMux }
func (s *Entropy) Engine() consensus.Engine           { return s.engine }
func (s *Entropy) ChainDb() database.Database         { return s.chainDb }
func (s *Entropy) IsListening() bool                  { return true } // Always listening
func (s *Entropy) EthVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *Entropy) NetVersion() uint64                 { return s.networkID }
func (s *Entropy) Downloader() *downloader.Downloader { return s.protocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Entropy) Protocols() []p2p.Protocol {
	if s.lesServer == nil {
		return s.protocolManager.SubProtocols
	}
	return append(s.protocolManager.SubProtocols, s.lesServer.Protocols()...)
}

// Start implements node.Service, starting all internal goroutines needed by the
// Entropy protocol implementation.
func (s *Entropy) Start(srvr *p2p.Server) error {
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers()

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
