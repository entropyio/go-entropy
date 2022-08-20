package entropy

import (
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/accounts"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/bloombits"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state/pruner"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/consensus/beacon"
	"github.com/entropyio/go-entropy/consensus/clique"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/entropy/entropyapi"
	"github.com/entropyio/go-entropy/entropy/entropyconfig"
	"github.com/entropyio/go-entropy/entropy/filters"
	"github.com/entropyio/go-entropy/entropy/gasprice"
	"github.com/entropyio/go-entropy/entropy/protocols/ent"
	"github.com/entropyio/go-entropy/entropy/protocols/snap"
	"github.com/entropyio/go-entropy/entropy/tracers/shutdown"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/miner"
	"github.com/entropyio/go-entropy/rpc"
	"github.com/entropyio/go-entropy/server/node"
	"github.com/entropyio/go-entropy/server/p2p"
	"github.com/entropyio/go-entropy/server/p2p/dnsdisc"
	"github.com/entropyio/go-entropy/server/p2p/enode"
	"math/big"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var log = logger.NewLogger("[entropy]")

// Config contains the configuration options of the ETH protocol.
// Deprecated: use ethconfig.Config instead.
type Config = entropyconfig.Config

// Entropy implements the Entropy full node service.
type Entropy struct {
	config *entropyconfig.Config

	// Handlers
	txPool             *blockchain.TxPool
	blockchain         *blockchain.BlockChain
	handler            *handler
	ethDialCandidates  enode.Iterator
	snapDialCandidates enode.Iterator
	merger             *consensus.Merger

	// DB interfaces
	chainDb database.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests     chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer      *blockchain.ChainIndexer       // Bloom indexer operating during block imports
	closeBloomHandler chan struct{}

	APIBackend *EntropyAPIBackend

	miner       *miner.Miner
	gasPrice    *big.Int
	entropyBase common.Address

	networkID     uint64
	netRPCService *entropyapi.NetAPI

	p2pServer *p2p.Server

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and entropybase)

	shutdownTracker *shutdown.ShutdownTracker // Tracks if and when the node has shutdown ungracefully
}

// New creates a new Entropy object (including the
// initialisation of the common Entropy object)
func New(stack *node.Node, configObj *entropyconfig.Config) (*Entropy, error) {
	// Ensure configuration values are compatible and sane
	if configObj.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run entropy.Entropy in light sync mode, use les.LightEntropy")
	}
	if !configObj.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", configObj.SyncMode)
	}
	if configObj.Miner.GasPrice == nil || configObj.Miner.GasPrice.Cmp(common.Big0) <= 0 {
		log.Warning("Sanitizing invalid miner gas price", "provided", configObj.Miner.GasPrice, "updated", entropyconfig.Defaults.Miner.GasPrice)
		configObj.Miner.GasPrice = new(big.Int).Set(entropyconfig.Defaults.Miner.GasPrice)
	}
	if configObj.NoPruning && configObj.TrieDirtyCache > 0 {
		if configObj.SnapshotCache > 0 {
			configObj.TrieCleanCache += configObj.TrieDirtyCache * 3 / 5
			configObj.SnapshotCache += configObj.TrieDirtyCache * 2 / 5
		} else {
			configObj.TrieCleanCache += configObj.TrieDirtyCache
		}
		configObj.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(configObj.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(configObj.TrieDirtyCache)*1024*1024)

	// Transfer mining-related config to the ethash config.
	ethashConfig := configObj.Ethash
	ethashConfig.NotifyFull = configObj.Miner.NotifyFull

	// Assemble the Entropy object
	chainDb, err := stack.OpenDatabaseWithFreezer("chaindata", configObj.DatabaseCache, configObj.DatabaseHandles, configObj.DatabaseFreezer, "entropy/db/chaindata/", false)
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := blockchain.SetupGenesisBlockWithOverride(chainDb, configObj.Genesis, configObj.OverrideTerminalTotalDifficulty)
	if _, ok := genesisErr.(*config.CompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("")
	log.Info(strings.Repeat("-", 153))
	for _, line := range strings.Split(chainConfig.String(), "\n") {
		log.Info(line)
	}
	log.Info(strings.Repeat("-", 153))
	log.Info("")

	if err := pruner.RecoverPruning(stack.ResolvePath(""), chainDb, stack.ResolvePath(configObj.TrieCleanCacheJournal)); err != nil {
		log.Error("Failed to recover state", "error", err)
	}
	merger := consensus.NewMerger(chainDb)
	entropyObj := &Entropy{
		config:            configObj,
		merger:            merger,
		chainDb:           chainDb,
		eventMux:          stack.EventMux(),
		accountManager:    stack.AccountManager(),
		engine:            entropyconfig.CreateConsensusEngine(stack, chainConfig, &ethashConfig, configObj.Miner.Notify, configObj.Miner.Noverify, chainDb),
		closeBloomHandler: make(chan struct{}),
		networkID:         configObj.NetworkId,
		gasPrice:          configObj.Miner.GasPrice,
		entropyBase:       configObj.Miner.EntropyBase,
		bloomRequests:     make(chan chan *bloombits.Retrieval),
		bloomIndexer:      blockchain.NewBloomIndexer(chainDb, config.BloomBitsBlocks, config.BloomConfirms),
		p2pServer:         stack.Server(),
		shutdownTracker:   shutdown.NewShutdownTracker(chainDb),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Infof("Initialising Entropy protocol. network=%d, dbVersion=%s", configObj.NetworkId, dbVer)

	if !configObj.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > blockchain.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Geth %s only supports v%d", *bcVersion, config.VersionWithMeta, blockchain.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < blockchain.BlockChainVersion {
			if bcVersion != nil { // only print warning on upgrade, not on init
				log.Warning("Upgrade blockchain database version", "from", dbVer, "to", blockchain.BlockChainVersion)
			}
			rawdb.WriteDatabaseVersion(chainDb, blockchain.BlockChainVersion)
		}
	}
	var (
		vmConfig = evm.Config{
			EnablePreimageRecording: configObj.EnablePreimageRecording,
		}
		cacheConfig = &blockchain.CacheConfig{
			TrieCleanLimit:      configObj.TrieCleanCache,
			TrieCleanJournal:    stack.ResolvePath(configObj.TrieCleanCacheJournal),
			TrieCleanRejournal:  configObj.TrieCleanCacheRejournal,
			TrieCleanNoPrefetch: configObj.NoPrefetch,
			TrieDirtyLimit:      configObj.TrieDirtyCache,
			TrieDirtyDisabled:   configObj.NoPruning,
			TrieTimeLimit:       configObj.TrieTimeout,
			SnapshotLimit:       configObj.SnapshotCache,
			Preimages:           configObj.Preimages,
		}
	)
	entropyObj.blockchain, err = blockchain.NewBlockChain(chainDb, cacheConfig, chainConfig, entropyObj.engine, vmConfig, entropyObj.shouldPreserve, &configObj.TxLookupLimit)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*config.CompatError); ok {
		log.Warning("Rewinding chain to upgrade configuration", "err", compat)
		entropyObj.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	entropyObj.bloomIndexer.Start(entropyObj.blockchain)

	if configObj.TxPool.Journal != "" {
		configObj.TxPool.Journal = stack.ResolvePath(configObj.TxPool.Journal)
	}
	entropyObj.txPool = blockchain.NewTxPool(configObj.TxPool, chainConfig, entropyObj.blockchain)

	// Permit the downloader to use the trie cache allowance during fast sync
	cacheLimit := cacheConfig.TrieCleanLimit + cacheConfig.TrieDirtyLimit + cacheConfig.SnapshotLimit
	checkpoint := configObj.Checkpoint
	if checkpoint == nil {
		checkpoint = config.TrustedCheckpoints[genesisHash]
	}
	if entropyObj.handler, err = newHandler(&handlerConfig{
		Database:       chainDb,
		Chain:          entropyObj.blockchain,
		TxPool:         entropyObj.txPool,
		Merger:         merger,
		Network:        configObj.NetworkId,
		Sync:           configObj.SyncMode,
		BloomCache:     uint64(cacheLimit),
		EventMux:       entropyObj.eventMux,
		Checkpoint:     checkpoint,
		RequiredBlocks: configObj.RequiredBlocks,
	}); err != nil {
		return nil, err
	}

	entropyObj.miner = miner.New(entropyObj, &configObj.Miner, chainConfig, entropyObj.EventMux(), entropyObj.engine, entropyObj.isLocalBlock)
	entropyObj.miner.SetExtra(makeExtraData(configObj.Miner.ExtraData))

	entropyObj.APIBackend = &EntropyAPIBackend{stack.Config().ExtRPCEnabled(), stack.Config().AllowUnprotectedTxs, entropyObj, nil}
	if entropyObj.APIBackend.allowUnprotectedTxs {
		log.Info("Unprotected transactions allowed")
	}
	gpoParams := configObj.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = configObj.Miner.GasPrice
	}
	entropyObj.APIBackend.gpo = gasprice.NewOracle(entropyObj.APIBackend, gpoParams)

	// Setup DNS discovery iterators.
	dnsclient := dnsdisc.NewClient(dnsdisc.Config{})
	entropyObj.ethDialCandidates, err = dnsclient.NewIterator(entropyObj.config.EthDiscoveryURLs...)
	if err != nil {
		return nil, err
	}
	entropyObj.snapDialCandidates, err = dnsclient.NewIterator(entropyObj.config.SnapDiscoveryURLs...)
	if err != nil {
		return nil, err
	}

	// Start the RPC service
	entropyObj.netRPCService = entropyapi.NewNetAPI(entropyObj.p2pServer, configObj.NetworkId)

	// Register the backend on the node
	stack.RegisterAPIs(entropyObj.APIs())
	stack.RegisterProtocols(entropyObj.Protocols())
	stack.RegisterLifecycle(entropyObj)

	// Successful startup; push a marker and check previous unclean shutdowns.
	entropyObj.shutdownTracker.MarkStartup()

	return entropyObj, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
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

// APIs return the collection of RPC services the entropy package offers.
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
			Service:   NewEntropyAPI(s),
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewMinerAPI(s),
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   downloader.NewDownloaderAPI(s.handler.downloader, s.eventMux),
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   filters.NewFilterAPI(s.APIBackend, false, 5*time.Minute),
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewDebugAPI(s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
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
func (s *Entropy) isLocalBlock(header *model.Header) bool {
	author, err := s.engine.Author(header)
	if err != nil {
		log.Warning("Failed to retrieve block author", "number", header.Number.Uint64(), "hash", header.Hash(), "err", err)
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
func (s *Entropy) shouldPreserve(header *model.Header) bool {
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
	return s.isLocalBlock(header)
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
		var cliqueObj *clique.Clique
		if c, ok := s.engine.(*clique.Clique); ok {
			cliqueObj = c
		} else if cl, ok := s.engine.(*beacon.Beacon); ok {
			if c, ok := cl.InnerEngine().(*clique.Clique); ok {
				cliqueObj = c
			}
		}
		if cliqueObj != nil {
			wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
			if wallet == nil || err != nil {
				log.Error("EntropyBase account unavailable locally", "err", err)
				return fmt.Errorf("signer missing: %v", err)
			}
			cliqueObj.Authorize(eb, wallet.SignData)
		}
		// If mining is started, we can disable the transaction rejection mechanism
		// introduced to speed sync times.
		atomic.StoreUint32(&s.handler.acceptTxs, 1)

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

func (s *Entropy) AccountManager() *accounts.Manager      { return s.accountManager }
func (s *Entropy) BlockChain() *blockchain.BlockChain     { return s.blockchain }
func (s *Entropy) TxPool() *blockchain.TxPool             { return s.txPool }
func (s *Entropy) EventMux() *event.TypeMux               { return s.eventMux }
func (s *Entropy) Engine() consensus.Engine               { return s.engine }
func (s *Entropy) ChainDb() database.Database             { return s.chainDb }
func (s *Entropy) IsListening() bool                      { return true } // Always listening
func (s *Entropy) Downloader() *downloader.Downloader     { return s.handler.downloader }
func (s *Entropy) Synced() bool                           { return atomic.LoadUint32(&s.handler.acceptTxs) == 1 }
func (s *Entropy) SetSynced()                             { atomic.StoreUint32(&s.handler.acceptTxs, 1) }
func (s *Entropy) ArchiveMode() bool                      { return s.config.NoPruning }
func (s *Entropy) BloomIndexer() *blockchain.ChainIndexer { return s.bloomIndexer }
func (s *Entropy) Merger() *consensus.Merger              { return s.merger }
func (s *Entropy) SyncMode() downloader.SyncMode {
	mode, _ := s.handler.chainSync.modeAndLocalHead()
	return mode
}

// Protocols returns all the currently configured
// network protocols to start.
func (s *Entropy) Protocols() []p2p.Protocol {
	protos := ent.MakeProtocols((*entropyHandler)(s.handler), s.networkID, s.ethDialCandidates)
	if s.config.SnapshotCache > 0 {
		protos = append(protos, snap.MakeProtocols((*snapHandler)(s.handler), s.snapDialCandidates)...)
	}
	return protos
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Entropy protocol implementation.
func (s *Entropy) Start() error {
	ent.StartENRUpdater(s.blockchain, s.p2pServer.LocalNode())
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(config.BloomBitsBlocks)

	// Regularly update shutdown marker
	s.shutdownTracker.Start()

	// Figure out a max peers count based on the server limits
	maxPeers := s.p2pServer.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers >= s.p2pServer.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, s.p2pServer.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.handler.Start(maxPeers)
	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// Entropy protocol.
func (s *Entropy) Stop() error {
	// Stop all the peer-related stuff first.
	s.ethDialCandidates.Close()
	s.snapDialCandidates.Close()
	s.handler.Stop()

	// Then stop everything else.
	s.bloomIndexer.Close()
	close(s.closeBloomHandler)
	s.txPool.Stop()
	s.miner.Close()
	s.blockchain.Stop()
	_ = s.engine.Close()

	// Clean shutdown marker as the last thing before closing db
	s.shutdownTracker.Stop()

	_ = s.chainDb.Close()
	s.eventMux.Stop()

	return nil
}
