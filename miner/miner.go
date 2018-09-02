package miner

import (
	"fmt"
	"sync/atomic"

	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/logger"
)

var log = logger.NewLogger("[miner]")

// Backend wraps all methods required for mining.
type Backend interface {
	BlockChain() *blockchain.BlockChain
	TxPool() *blockchain.TxPool
}

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	mux *event.TypeMux

	worker *worker

	coinbase common.Address
	entropy      Backend
	engine   consensus.Engine
	exitCh   chan struct{}

	canStart    int32 // can start indicates whether we can start the mining operation
	shouldStart int32 // should start indicates whether we should start after sync
}

func New(entropy Backend, config *config.ChainConfig, mux *event.TypeMux, engine consensus.Engine) *Miner {
	miner := &Miner{
		entropy:      entropy,
		mux:      mux,
		engine:   engine,
		exitCh:   make(chan struct{}),
		worker:   newWorker(config, engine, entropy, mux),
		canStart: 1,
	}
	go miner.update()

	return miner
}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (miner *Miner) update() {
	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer events.Unsubscribe()

	for {
		select {
		case ev := <-events.Chan():
			if ev == nil {
				return
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				atomic.StoreInt32(&miner.canStart, 0)
				if miner.Mining() {
					miner.Stop()
					atomic.StoreInt32(&miner.shouldStart, 1)
					log.Info("Mining aborted due to sync")
				}
			case downloader.DoneEvent, downloader.FailedEvent:
				shouldStart := atomic.LoadInt32(&miner.shouldStart) == 1

				atomic.StoreInt32(&miner.canStart, 1)
				atomic.StoreInt32(&miner.shouldStart, 0)
				if shouldStart {
					miner.Start(miner.coinbase)
				}
				// stop immediately and ignore all further pending events
				return
			}
		case <-miner.exitCh:
			return
		}
	}
}

func (miner *Miner) Start(coinbase common.Address) {
	log.Warningf("miner start with coinbase : %X", coinbase)
	atomic.StoreInt32(&miner.shouldStart, 1)
	miner.SetEntropyBase(coinbase)

	if atomic.LoadInt32(&miner.canStart) == 0 {
		log.Info("Network syncing, will start miner afterwards")
		return
	}
	miner.worker.start()
}

func (miner *Miner) Stop() {
	log.Warningf("miner stop: 0x%x", miner.coinbase)
	miner.worker.stop()
	atomic.StoreInt32(&miner.shouldStart, 0)
}

func (miner *Miner) Close() {
	miner.worker.close()
	close(miner.exitCh)
}

func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}

func (miner *Miner) HashRate() uint64 {
	if pow, ok := miner.engine.(consensus.PoW); ok {
		return uint64(pow.Hashrate())
	}
	return 0
}

func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > config.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), config.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
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

func (miner *Miner) SetEntropyBase(addr common.Address) {
	miner.coinbase = addr
	miner.worker.setEntropyBase(addr)
}
