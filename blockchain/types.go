package blockchain

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/evm"
)

// Validator is an interface which defines the standard for block validation. It
// is only responsible for validating block contents, as the header validation is
// done by the specific consensus engines.
//
type Validator interface {
	// ValidateBody validates the given block's content.
	ValidateBody(block *model.Block) error

	// ValidateState validates the given statedb and optionally the receipts and
	// gas used.
	ValidateState(block, parent *model.Block, state *state.StateDB, receipts model.Receipts, usedGas uint64) error
}

// Processor is an interface for processing blocks using a given initial state.
//
// Process takes the block to be processed and the statedb upon which the
// initial state is based. It should return the receipts generated, amount
// of gas used in the process and return an error if any of the internal rules
// failed.
type Processor interface {
	Process(block *model.Block, statedb *state.StateDB, cfg evm.Config) (model.Receipts, []*model.Log, uint64, error)
}
