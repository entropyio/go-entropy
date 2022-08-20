package consensus

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/rpc"
	"math/big"
)

// ChainHeaderReader defines a small collection of methods needed to access the local
// blockchain during header verification.
type ChainHeaderReader interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *config.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() *model.Header

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *model.Header

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *model.Header

	// GetHeaderByHash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash common.Hash) *model.Header

	// GetTd retrieves the total difficulty from the database by hash and number.
	GetTd(hash common.Hash, number uint64) *big.Int
}

// ChainReader defines a small collection of methods needed to access the local
// blockchain during header and/or uncle verification.
type ChainReader interface {
	ChainHeaderReader

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *model.Block
}

// Engine is an algorithm agnostic consensus engine.
type Engine interface {
	// Author retrieves the Entropy address of the account that minted the given
	// block, which may be different from the header's coinbase if a consensus
	// engine is based on signatures.
	Author(header *model.Header) (common.Address, error)

	// VerifyHeader checks whether a header conforms to the consensus rules of a
	// given engine. Verifying the seal may be done optionally here, or explicitly
	// via the VerifySeal method.
	VerifyHeader(chain ChainHeaderReader, header *model.Header, seal bool) error

	// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
	// concurrently. The method returns a quit channel to abort the operations and
	// a results channel to retrieve the async verifications (the order is that of
	// the input slice).
	VerifyHeaders(chain ChainHeaderReader, headers []*model.Header, seals []bool) (chan<- struct{}, <-chan error)

	// VerifyUncles verifies that the given block's uncles conform to the consensus
	// rules of a given engine.
	VerifyUncles(chain ChainReader, block *model.Block) error

	// Prepare initializes the consensus fields of a block header according to the
	// rules of a particular engine. The changes are executed inline.
	Prepare(chain ChainHeaderReader, header *model.Header) error

	// Finalize runs any post-transaction state modifications (e.g. block rewards)
	// but does not assemble the block.
	//
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	Finalize(chain ChainHeaderReader, header *model.Header, state *state.StateDB, txs []*model.Transaction,
		uncles []*model.Header)

	// FinalizeAndAssemble runs any post-transaction state modifications (e.g. block
	// rewards) and assembles the final block.
	//
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	FinalizeAndAssemble(chain ChainHeaderReader, header *model.Header, state *state.StateDB, txs []*model.Transaction,
		uncles []*model.Header, receipts []*model.Receipt) (*model.Block, error)

	// Seal generates a new sealing request for the given input block and pushes
	// the result into the given channel.
	//
	// Note, the method returns immediately and will send the result async. More
	// than one result may also be returned depending on the consensus algorithm.
	Seal(chain ChainHeaderReader, block *model.Block, results chan<- *model.Block, stop <-chan struct{}) error

	// SealHash returns the hash of a block prior to it being sealed.
	SealHash(header *model.Header) common.Hash

	// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
	// that a new block should have.
	CalcDifficulty(chain ChainHeaderReader, time uint64, parent *model.Header) *big.Int

	// APIs returns the RPC APIs this consensus engine provides.
	APIs(chain ChainHeaderReader) []rpc.API

	// Close terminates any background threads maintained by the consensus engine.
	Close() error
}

// PoW is a consensus engine based on proof-of-work.
type PoW interface {
	Engine

	// Hashrate returns the current mining hashrate of a PoW consensus engine.
	Hashrate() float64
}
