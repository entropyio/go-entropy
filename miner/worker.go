package miner

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"

	"github.com/deckarep/golang-set"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
)

const (
	// resultQueueSize is the size of channel listening to sealing result.
	resultQueueSize = 10
	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
	// chainSideChanSize is the size of channel listening to ChainSideEvent.
	chainSideChanSize = 10
	miningLogAtDepth  = 5
)

// Env is the worker's current environment and holds all of the current state information.
type Env struct {
	config *config.ChainConfig
	signer model.Signer

	state     *state.StateDB      // apply state changes here
	ancestors mapset.Set          // ancestor set (used for checking uncle parent validity)
	family    mapset.Set          // family set (used for checking uncle invalidity)
	uncles    mapset.Set          // uncle set
	tcount    int                 // tx count in cycle
	gasPool   *blockchain.GasPool // available gas used to pack transactions

	header   *model.Header
	txs      []*model.Transaction
	receipts []*model.Receipt
}

func (env *Env) commitTransaction(tx *model.Transaction, bc *blockchain.BlockChain, coinbase common.Address, gp *blockchain.GasPool) (error, []*model.Log) {
	snap := env.state.Snapshot()

	receipt, _, err := blockchain.ApplyTransaction(env.config, bc, &coinbase, gp, env.state, env.header, tx, &env.header.GasUsed, evm.Config{})
	if err != nil {
		env.state.RevertToSnapshot(snap)
		return err, nil
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)

	return nil, receipt.Logs
}

func (env *Env) commitTransactions(mux *event.TypeMux, txs *model.TransactionsByPriceAndNonce, bc *blockchain.BlockChain, coinbase common.Address) {
	if env.gasPool == nil {
		env.gasPool = new(blockchain.GasPool).AddGas(env.header.GasLimit)
	}

	var coalescedLogs []*model.Log

	for {
		// If we don't have enough gas for any further transactions then we're done
		if env.gasPool.Gas() < config.TxGas {
			log.Debug("Not enough gas for further transactions", "have", env.gasPool, "want", config.TxGas)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		from, _ := model.Sender(env.signer, tx)
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.Protected() && !env.config.IsHomestead(env.header.Number) {
			log.Debug("Ignoring reply protected transaction", "hash", tx.Hash(), "homestead", env.config.HomesteadBlock)

			txs.Pop()
			continue
		}
		// Start executing the transaction
		env.state.Prepare(tx.Hash(), common.Hash{}, env.tcount)

		err, logs := env.commitTransaction(tx, bc, coinbase, env.gasPool)
		switch err {
		case blockchain.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Debug("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case blockchain.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			log.Debug("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case blockchain.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Debug("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			env.tcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if len(coalescedLogs) > 0 || env.tcount > 0 {
		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*model.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(model.Log)
			*cpy[i] = *l
		}
		go func(logs []*model.Log, tcount int) {
			if len(logs) > 0 {
				mux.Post(blockchain.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(blockchain.PendingStateEvent{})
			}
		}(cpy, env.tcount)
	}
}

// task contains all information for consensus engine sealing and result submitting.
type task struct {
	receipts  []*model.Receipt
	state     *state.StateDB
	block     *model.Block
	createdAt time.Time
}

// worker is the main object which takes care of submitting new work to consensus engine
// and gathering the sealing result.
type worker struct {
	config *config.ChainConfig
	engine consensus.Engine
	eth    Backend
	chain  *blockchain.BlockChain

	// Subscriptions
	mux          *event.TypeMux
	txsCh        chan blockchain.NewTxsEvent
	txsSub       event.Subscription
	chainHeadCh  chan blockchain.ChainHeadEvent
	chainHeadSub event.Subscription
	chainSideCh  chan blockchain.ChainSideEvent
	chainSideSub event.Subscription

	// Channels
	newWork  chan struct{}
	taskCh   chan *task
	resultCh chan *task
	exitCh   chan struct{}

	current        *Env                         // An environment for current running cycle.
	possibleUncles map[common.Hash]*model.Block // A set of side blocks as the possible uncle blocks.
	unconfirmed    *unconfirmedBlocks           // A set of locally mined blocks pending canonicalness confirmations.

	mu       sync.RWMutex // The lock used to protect the coinbase and extra fields
	coinbase common.Address
	extra    []byte

	snapshotMu    sync.RWMutex // The lock used to protect the block snapshot and state snapshot
	snapshotBlock *model.Block
	snapshotState *state.StateDB

	// atomic status counters
	running int32 // The indicator whether the consensus engine is running or not.

	// Test hooks
	newTaskHook  func(*task)      // Method to call upon receiving a new sealing task
	skipSealHook func(*task) bool // Method to decide whether skipping the sealing.
	fullTaskHook func()           // Method to call before pushing the full sealing task
}

func newWorker(config *config.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux) *worker {
	worker := &worker{
		config:         config,
		engine:         engine,
		eth:            eth,
		mux:            mux,
		chain:          eth.BlockChain(),
		possibleUncles: make(map[common.Hash]*model.Block),
		unconfirmed:    newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
		txsCh:          make(chan blockchain.NewTxsEvent, txChanSize),
		chainHeadCh:    make(chan blockchain.ChainHeadEvent, chainHeadChanSize),
		chainSideCh:    make(chan blockchain.ChainSideEvent, chainSideChanSize),
		newWork:        make(chan struct{}, 1),
		taskCh:         make(chan *task),
		resultCh:       make(chan *task, resultQueueSize),
		exitCh:         make(chan struct{}),
	}
	// Subscribe NewTxsEvent for tx pool
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainSideSub = eth.BlockChain().SubscribeChainSideEvent(worker.chainSideCh)

	go worker.mainLoop()
	go worker.resultLoop()
	go worker.taskLoop()

	// Submit first work to initialize pending state.
	worker.newWork <- struct{}{}
	return worker
}

// setEntropyBase sets the entropyBase used to initialize the block coinbase field.
func (w *worker) setEntropyBase(addr common.Address) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.coinbase = addr
}

// setExtra sets the content used to initialize the block extra field.
func (w *worker) setExtra(extra []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.extra = extra
}

// pending returns the pending state and corresponding block.
func (w *worker) pending() (*model.Block, *state.StateDB) {
	// return a snapshot to avoid contention on currentMu mutex
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	if w.snapshotState == nil {
		return nil, nil
	}
	return w.snapshotBlock, w.snapshotState.Copy()
}

// pendingBlock returns pending block.
func (w *worker) pendingBlock() *model.Block {
	// return a snapshot to avoid contention on currentMu mutex
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	return w.snapshotBlock
}

// start sets the running status as 1 and triggers new work submitting.
func (w *worker) start() {
	atomic.StoreInt32(&w.running, 1)
	w.newWork <- struct{}{}
}

// stop sets the running status as 0.
func (w *worker) stop() {
	atomic.StoreInt32(&w.running, 0)
}

// isRunning returns an indicator whether worker is running or not.
func (w *worker) isRunning() bool {
	return atomic.LoadInt32(&w.running) == 1
}

// close terminates all background threads maintained by the worker and cleans up buffered channels.
// Note the worker does not support being closed multiple times.
func (w *worker) close() {
	close(w.exitCh)
	// Clean up buffered channels
	for empty := false; !empty; {
		select {
		case <-w.resultCh:
		default:
			empty = true
		}
	}
}

// mainLoop is a standalone goroutine to regenerate the sealing task based on the received event.
func (w *worker) mainLoop() {
	defer w.txsSub.Unsubscribe()
	defer w.chainHeadSub.Unsubscribe()
	defer w.chainSideSub.Unsubscribe()

	for {
		select {
		case <-w.newWork:
			// Submit a work when the worker is created or started.
			w.commitNewWork()

		case <-w.chainHeadCh:
			// Resubmit a work for new cycle once worker receives chain head event.
			w.commitNewWork()

		case ev := <-w.chainSideCh:
			if _, exist := w.possibleUncles[ev.Block.Hash()]; exist {
				continue
			}
			// Add side block to possible uncle block set.
			w.possibleUncles[ev.Block.Hash()] = ev.Block
			// If our mining block contains less than 2 uncle blocks,
			// add the new uncle block if valid and regenerate a mining block.
			if w.isRunning() && w.current != nil && w.current.uncles.Cardinality() < 2 {
				start := time.Now()
				if err := w.commitUncle(w.current, ev.Block.Header()); err == nil {
					var uncles []*model.Header
					w.current.uncles.Each(func(item interface{}) bool {
						hash, ok := item.(common.Hash)
						if !ok {
							return false
						}
						uncle, exist := w.possibleUncles[hash]
						if !exist {
							return false
						}
						uncles = append(uncles, uncle.Header())
						return true
					})
					w.commit(uncles, nil, true, start)
				}
			}

		case ev := <-w.txsCh:
			// Apply transactions to the pending state if we're not mining.
			//
			// Note all transactions received may not be continuous with transactions
			// already included in the current mining block. These transactions will
			// be automatically eliminated.
			if !w.isRunning() && w.current != nil {
				w.mu.Lock()
				coinbase := w.coinbase
				w.mu.Unlock()

				txs := make(map[common.Address]model.Transactions)
				for _, tx := range ev.Txs {
					acc, _ := model.Sender(w.current.signer, tx)
					txs[acc] = append(txs[acc], tx)
				}
				txset := model.NewTransactionsByPriceAndNonce(w.current.signer, txs)
				w.current.commitTransactions(w.mux, txset, w.chain, coinbase)
				w.updateSnapshot()
			} else {
				// If we're mining, but nothing is being processed, wake on new transactions
				if w.config.Clique != nil && w.config.Clique.Period == 0 {
					w.commitNewWork()
				}
			}

			// System stopped
		case <-w.exitCh:
			return
		case <-w.txsSub.Err():
			return
		case <-w.chainHeadSub.Err():
			return
		case <-w.chainSideSub.Err():
			return
		}
	}
}

// seal pushes a sealing task to consensus engine and submits the result.
func (w *worker) seal(t *task, stop <-chan struct{}) {
	var (
		err error
		res *task
	)

	if w.skipSealHook != nil && w.skipSealHook(t) {
		return
	}

	if t.block, err = w.engine.Seal(w.chain, t.block, stop); t.block != nil {
		log.Info("Successfully sealed new block", "number", t.block.Number(), "hash", t.block.Hash(),
			"elapsed", common.PrettyDuration(time.Since(t.createdAt)))
		res = t
	} else {
		if err != nil {
			log.Warning("Block sealing failed", "err", err)
		}
		res = nil
	}
	select {
	case w.resultCh <- res:
	case <-w.exitCh:
	}
}

// taskLoop is a standalone goroutine to fetch sealing task from the generator and
// push them to consensus engine.
func (w *worker) taskLoop() {
	var stopCh chan struct{}

	// interrupt aborts the in-flight sealing task.
	interrupt := func() {
		if stopCh != nil {
			close(stopCh)
			stopCh = nil
		}
	}
	for {
		select {
		case task := <-w.taskCh:
			if w.newTaskHook != nil {
				w.newTaskHook(task)
			}
			interrupt()
			stopCh = make(chan struct{})
			go w.seal(task, stopCh)
		case <-w.exitCh:
			interrupt()
			return
		}
	}
}

// resultLoop is a standalone goroutine to handle sealing result submitting
// and flush relative data to the database.
func (w *worker) resultLoop() {
	for {
		select {
		case result := <-w.resultCh:
			if result == nil {
				continue
			}
			block := result.block

			// Update the block hash in all logs since it is now available and not when the
			// receipt/log of individual transactions were created.
			for _, r := range result.receipts {
				for _, l := range r.Logs {
					l.BlockHash = block.Hash()
				}
			}
			for _, log := range result.state.Logs() {
				log.BlockHash = block.Hash()
			}
			// Commit block and state to database.
			stat, err := w.chain.WriteBlockWithState(block, result.receipts, result.state)
			if err != nil {
				log.Error("Failed writing block to chain", "err", err)
				continue
			}
			// Broadcast the block and announce chain insertion event
			w.mux.Post(blockchain.NewMinedBlockEvent{Block: block})
			var (
				events []interface{}
				logs   = result.state.Logs()
			)
			switch stat {
			case blockchain.CanonStatTy:
				events = append(events, blockchain.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})
				events = append(events, blockchain.ChainHeadEvent{Block: block})
			case blockchain.SideStatTy:
				events = append(events, blockchain.ChainSideEvent{Block: block})
			}
			w.chain.PostChainEvents(events, logs)

			// Insert the block into the set of pending ones to resultLoop for confirmations
			w.unconfirmed.Insert(block.NumberU64(), block.Hash())

		case <-w.exitCh:
			return
		}
	}
}

// makeCurrent creates a new environment for the current cycle.
func (w *worker) makeCurrent(parent *model.Block, header *model.Header) error {
	state, err := w.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	env := &Env{
		config:    w.config,
		signer:    model.NewEIP155Signer(w.config.ChainID),
		state:     state,
		ancestors: mapset.NewSet(),
		family:    mapset.NewSet(),
		uncles:    mapset.NewSet(),
		header:    header,
	}

	// when 08 is processed ancestors contain 07 (quick block)
	for _, ancestor := range w.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			env.family.Add(uncle.Hash())
		}
		env.family.Add(ancestor.Hash())
		env.ancestors.Add(ancestor.Hash())
	}

	// Keep track of transactions which return errors so they can be removed
	env.tcount = 0
	w.current = env
	return nil
}

// commitUncle adds the given block to uncle block set, returns error if failed to add.
func (w *worker) commitUncle(env *Env, uncle *model.Header) error {
	hash := uncle.Hash()
	if env.uncles.Contains(hash) {
		return fmt.Errorf("uncle not unique")
	}
	if !env.ancestors.Contains(uncle.ParentHash) {
		return fmt.Errorf("uncle's parent unknown (%x)", uncle.ParentHash[0:4])
	}
	if env.family.Contains(hash) {
		return fmt.Errorf("uncle already in family (%x)", hash)
	}
	env.uncles.Add(uncle.Hash())
	return nil
}

// updateSnapshot updates pending snapshot block and state.
// Note this function assumes the current variable is thread safe.
func (w *worker) updateSnapshot() {
	w.snapshotMu.Lock()
	defer w.snapshotMu.Unlock()

	var uncles []*model.Header
	w.current.uncles.Each(func(item interface{}) bool {
		hash, ok := item.(common.Hash)
		if !ok {
			return false
		}
		uncle, exist := w.possibleUncles[hash]
		if !exist {
			return false
		}
		uncles = append(uncles, uncle.Header())
		return true
	})

	w.snapshotBlock = model.NewBlock(
		w.current.header,
		w.current.txs,
		uncles,
		w.current.receipts,
	)

	w.snapshotState = w.current.state.Copy()
}

// commitNewWork generates several new sealing tasks based on the parent block.
func (w *worker) commitNewWork() {
	w.mu.RLock()
	defer w.mu.RUnlock()

	tstart := time.Now()
	parent := w.chain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	header := &model.Header{
		ParentHash:    parent.Hash(),
		Number:        num.Add(num, common.Big1),
		GasLimit:      blockchain.CalcGasLimit(parent),
		Extra:         w.extra,
		Time:          big.NewInt(tstamp),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	// Only set the coinbase if our consensus engine is running (avoid spurious block rewards)
	if w.isRunning() {
		if w.coinbase == (common.Address{}) {
			log.Error("Refusing to mine without etherbase")
			return
		}
		header.Coinbase = w.coinbase
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}

	// Could potentially happen if starting to mine in an odd state.
	err := w.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// Create the current work task and check any fork transitions needed
	env := w.current

	// compute uncles for the new block.
	var (
		uncles    []*model.Header
		badUncles []common.Hash
	)
	for hash, uncle := range w.possibleUncles {
		if len(uncles) == 2 {
			break
		}
		if err := w.commitUncle(env, uncle.Header()); err != nil {
			log.Debug("Bad uncle found and will be removed", "hash", hash)
			log.Debug(fmt.Sprint(uncle))

			badUncles = append(badUncles, hash)
		} else {
			log.Debug("Committing new uncle to block", "hash", hash)
			uncles = append(uncles, uncle.Header())
		}
	}
	for _, hash := range badUncles {
		delete(w.possibleUncles, hash)
	}

	// Create an empty block based on temporary copied state for sealing in advance without waiting block
	// execution finished.
	w.commit(uncles, nil, false, tstart)

	// Fill the block with all available pending transactions.
	pending, err := w.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return
	}
	// Short circuit if there is no available pending transactions
	if len(pending) == 0 {
		w.updateSnapshot()
		return
	}
	txs := model.NewTransactionsByPriceAndNonce(w.current.signer, pending)
	env.commitTransactions(w.mux, txs, w.chain, w.coinbase)

	w.commit(uncles, w.fullTaskHook, true, tstart)
}

// commit runs any post-transaction state modifications, assembles the final block
// and commits new work if consensus engine is running.
func (w *worker) commit(uncles []*model.Header, interval func(), update bool, start time.Time) error {
	// Deep copy receipts here to avoid interaction between different tasks.
	receipts := make([]*model.Receipt, len(w.current.receipts))
	for i, l := range w.current.receipts {
		receipts[i] = new(model.Receipt)
		*receipts[i] = *l
	}
	s := w.current.state.Copy()
	block, err := w.engine.Finalize(w.chain, w.current.header, s, w.current.txs, uncles, w.current.receipts, nil)
	if err != nil {
		return err
	}
	if w.isRunning() {
		if interval != nil {
			interval()
		}
		select {
		case w.taskCh <- &task{receipts: receipts, state: s, block: block, createdAt: time.Now()}:
			w.unconfirmed.Shift(block.NumberU64() - 1)
			log.Info("Commit new mining work", "number", block.Number(), "txs", w.current.tcount, "uncles", len(uncles),
				"elapsed", common.PrettyDuration(time.Since(start)))
		case <-w.exitCh:
			log.Info("Worker has exited")
		}
	}
	if update {
		w.updateSnapshot()
	}
	return nil
}
