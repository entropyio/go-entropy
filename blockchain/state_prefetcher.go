package blockchain

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/evm"
	"sync/atomic"
)

// statePrefetcher is a basic Prefetcher, which blindly executes a block on top
// of an arbitrary state with the goal of prefetching potentially useful state
// data from disk before the main block processor start executing.
type statePrefetcher struct {
	config *config.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// newStatePrefetcher initialises a new statePrefetcher.
func newStatePrefetcher(config *config.ChainConfig, bc *BlockChain, engine consensus.Engine) *statePrefetcher {
	return &statePrefetcher{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Prefetch processes the state changes according to the Entropy rules by running
// the transaction messages using the statedb, but any changes are discarded. The
// only goal is to pre-cache transaction signatures and state trie nodes.
func (p *statePrefetcher) Prefetch(block *model.Block, statedb *state.StateDB, cfg evm.Config, interrupt *uint32) {
	var (
		header       = block.Header()
		gaspool      = new(GasPool).AddGas(block.GasLimit())
		blockContext = NewEVMBlockContext(header, p.bc, nil)
		vm           = evm.NewEVM(blockContext, evm.TxContext{}, statedb, p.config, cfg)
		signer       = model.MakeSigner(p.config, header.Number)
	)
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		// If block precaching was interrupted, abort
		if interrupt != nil && atomic.LoadUint32(interrupt) == 1 {
			return
		}
		// Convert the transaction into an executable message and pre-cache its sender
		msg, err := tx.AsMessage(signer, header.BaseFee)
		if err != nil {
			return // Also invalid block, bail out
		}
		statedb.Prepare(tx.Hash(), i)
		if err := precacheTransaction(msg, p.config, gaspool, statedb, header, vm); err != nil {
			return // Ugh, something went horribly wrong, bail out
		}
	}
}

// precacheTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. The goal is not to execute
// the transaction successfully, rather to warm up touched data slots.
func precacheTransaction(msg model.Message, conf *config.ChainConfig, gaspool *GasPool, statedb *state.StateDB, header *model.Header, vm *evm.EVM) error {
	// Update the evm with the new transaction context.
	vm.Reset(NewEVMTxContext(msg), statedb)
	// Add addresses to access list if applicable
	_, err := ApplyMessage(vm, msg, gaspool)
	return err
}
