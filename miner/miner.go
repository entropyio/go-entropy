package miner

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/logger"
	"math/big"
	"sync"
	"time"
)

var log = logger.NewLogger("[miner]")

// Backend wraps all methods required for mining. Only full node is capable
// to offer all the functions here.
type Backend interface {
	BlockChain() *blockchain.BlockChain
	TxPool() *blockchain.TxPool
}

// Config is the configuration parameters of mining.
type Config struct {
	EntropyBase common.Address `toml:",omitempty"` // Public address for block mining rewards (default = first account)
	Notify      []string       `toml:",omitempty"` // HTTP URL list to be notified of new work packages(only useful in ethash).
	NotifyFull  bool           `toml:",omitempty"` // Notify with pending block headers instead of work packages
	ExtraData   hexutil.Bytes  `toml:",omitempty"` // Block extra data set by the miner
	GasFloor    uint64         // Target gas floor for mined blocks.
	GasCeil     uint64         // Target gas ceiling for mined blocks.
	GasPrice    *big.Int       // Minimum gas price for mining a transaction
	Recommit    time.Duration  // The time interval for miner to re-create mining work.
	Noverify    bool           // Disable remote mining solution verification(only useful in ethash).
}

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	mux *event.TypeMux

	worker *worker

	coinbase common.Address
	eth      Backend
	engine   consensus.Engine
	exitCh   chan struct{}
	startCh  chan common.Address
	stopCh   chan struct{}

	wg sync.WaitGroup
}

func New(eth Backend, config *Config, chainConfig *config.ChainConfig, mux *event.TypeMux, engine consensus.Engine, isLocalBlock func(header *model.Header) bool) *Miner {
	miner := &Miner{
		eth:     eth,
		mux:     mux,
		engine:  engine,
		exitCh:  make(chan struct{}),
		startCh: make(chan common.Address),
		stopCh:  make(chan struct{}),
		worker:  newWorker(config, chainConfig, engine, eth, mux, isLocalBlock, true),
	}
	miner.wg.Add(1)
	go miner.update()
	return miner
}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (miner *Miner) update() {
	defer miner.wg.Done()

	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer func() {
		if !events.Closed() {
			events.Unsubscribe()
		}
	}()

	shouldStart := false
	canStart := true
	dlEventCh := events.Chan()
	for {
		select {
		case ev := <-dlEventCh:
			if ev == nil {
				// Unsubscription done, stop listening
				dlEventCh = nil
				continue
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				wasMining := miner.Mining()
				miner.worker.stop()
				canStart = false
				if wasMining {
					// Resume mining after sync was finished
					shouldStart = true
					log.Info("Mining aborted due to sync")
				}
			case downloader.FailedEvent:
				canStart = true
				if shouldStart {
					miner.SetEntropyBase(miner.coinbase)
					miner.worker.start()
				}
			case downloader.DoneEvent:
				canStart = true
				if shouldStart {
					miner.SetEntropyBase(miner.coinbase)
					miner.worker.start()
				}
				// Stop reacting to downloader events
				events.Unsubscribe()
			}
		case addr := <-miner.startCh:
			miner.SetEntropyBase(addr)
			if canStart {
				miner.worker.start()
			}
			shouldStart = true
		case <-miner.stopCh:
			shouldStart = false
			miner.worker.stop()
		case <-miner.exitCh:
			miner.worker.close()
			return
		}
	}
}

func (miner *Miner) Start(coinbase common.Address) {
	log.Warningf("miner start with coinbase : %X", coinbase)
	miner.startCh <- coinbase
}

func (miner *Miner) Stop() {
	log.Warningf("miner stop at: 0x%x", miner.coinbase)
	miner.stopCh <- struct{}{}
}

func (miner *Miner) Close() {
	close(miner.exitCh)
	miner.wg.Wait()
}

func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}

func (miner *Miner) Hashrate() uint64 {
	if pow, ok := miner.engine.(consensus.PoW); ok {
		return uint64(pow.Hashrate())
	}
	return 0
}

func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > config.MaximumExtraDataSize {
		return fmt.Errorf("Extra exceeds max length. %d > %v", len(extra), config.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
}

// SetRecommitInterval sets the interval for sealing work resubmitting.
func (miner *Miner) SetRecommitInterval(interval time.Duration) {
	miner.worker.setRecommitInterval(interval)
}

// Pending returns the currently pending block and associated state.
func (miner *Miner) Pending() (*model.Block, *state.StateDB) {
	return miner.worker.pending()
}

// PendingBlock returns the currently pending block.
//
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
func (miner *Miner) PendingBlock() *model.Block {
	return miner.worker.pendingBlock()
}

// PendingBlockAndReceipts returns the currently pending block and corresponding receipts.
func (miner *Miner) PendingBlockAndReceipts() (*model.Block, model.Receipts) {
	return miner.worker.pendingBlockAndReceipts()
}

func (miner *Miner) SetEntropyBase(addr common.Address) {
	miner.coinbase = addr
	miner.worker.setEntropyBase(addr)
}

// SetGasCeil sets the gaslimit to strive for when mining blocks post 1559.
// For pre-1559 blocks, it sets the ceiling.
func (miner *Miner) SetGasCeil(ceil uint64) {
	miner.worker.setGasCeil(ceil)
}

// EnablePreseal turns on the preseal mining feature. It's enabled by default.
// Note this function shouldn't be exposed to API, it's unnecessary for users
// (miners) to actually know the underlying detail. It's only for outside project
// which uses this library.
func (miner *Miner) EnablePreseal() {
	miner.worker.enablePreseal()
}

// DisablePreseal turns off the preseal mining feature. It's necessary for some
// fake consensus engine which can seal blocks instantaneously.
// Note this function shouldn't be exposed to API, it's unnecessary for users
// (miners) to actually know the underlying detail. It's only for outside project
// which uses this library.
func (miner *Miner) DisablePreseal() {
	miner.worker.disablePreseal()
}

// SubscribePendingLogs starts delivering logs from pending transactions
// to the given channel.
func (miner *Miner) SubscribePendingLogs(ch chan<- []*model.Log) event.Subscription {
	return miner.worker.pendingLogsFeed.Subscribe(ch)
}

// GetSealingBlockAsync requests to generate a sealing block according to the
// given parameters. Regardless of whether the generation is successful or not,
// there is always a result that will be returned through the result channel.
// The difference is that if the execution fails, the returned result is nil
// and the concrete error is dropped silently.
func (miner *Miner) GetSealingBlockAsync(parent common.Hash, timestamp uint64, coinbase common.Address, random common.Hash, noTxs bool) (chan *model.Block, error) {
	resCh, _, err := miner.worker.getSealingBlock(parent, timestamp, coinbase, random, noTxs)
	if err != nil {
		return nil, err
	}
	return resCh, nil
}

// GetSealingBlockSync creates a sealing block according to the given parameters.
// If the generation is failed or the underlying work is already closed, an error
// will be returned.
func (miner *Miner) GetSealingBlockSync(parent common.Hash, timestamp uint64, coinbase common.Address, random common.Hash, noTxs bool) (*model.Block, error) {
	resCh, errCh, err := miner.worker.getSealingBlock(parent, timestamp, coinbase, random, noTxs)
	if err != nil {
		return nil, err
	}
	return <-resCh, <-errCh
}
