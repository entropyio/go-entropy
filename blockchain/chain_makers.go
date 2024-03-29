package blockchain

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/consensus/misc"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/evm"
	"math/big"
)

// BlockGen creates blocks for testing.
// See GenerateChain for a detailed explanation.
type BlockGen struct {
	i       int
	parent  *model.Block
	chain   []*model.Block
	header  *model.Header
	statedb *state.StateDB

	gasPool  *GasPool
	txs      []*model.Transaction
	receipts []*model.Receipt
	uncles   []*model.Header

	config *config.ChainConfig
	engine consensus.Engine
}

// SetCoinbase sets the coinbase of the generated block.
// It can be called at most once.
func (b *BlockGen) SetCoinbase(addr common.Address) {
	if b.gasPool != nil {
		if len(b.txs) > 0 {
			panic("coinbase must be set before adding transactions")
		}
		panic("coinbase can only be set once")
	}
	b.header.Coinbase = addr
	b.gasPool = new(GasPool).AddGas(b.header.GasLimit)
}

// SetExtra sets the extra data field of the generated block.
func (b *BlockGen) SetExtra(data []byte) {
	b.header.Extra = data
}

// SetNonce sets the nonce field of the generated block.
func (b *BlockGen) SetNonce(nonce model.BlockNonce) {
	b.header.Nonce = nonce
}

// SetDifficulty sets the difficulty field of the generated block. This method is
// useful for Clique tests where the difficulty does not depend on time. For the
// ethash tests, please use OffsetTime, which implicitly recalculates the diff.
func (b *BlockGen) SetDifficulty(diff *big.Int) {
	b.header.Difficulty = diff
}

// AddTx adds a transaction to the generated block. If no coinbase has
// been set, the block's coinbase is set to the zero address.
//
// AddTx panics if the transaction cannot be executed. In addition to
// the protocol-imposed limitations (gas limit, etc.), there are some
// further limitations on the content of transactions that can be
// added. Notably, contract code relying on the BLOCKHASH instruction
// will panic during execution.
func (b *BlockGen) AddTx(tx *model.Transaction) {
	b.AddTxWithChain(nil, tx)
}

// AddTxWithChain adds a transaction to the generated block. If no coinbase has
// been set, the block's coinbase is set to the zero address.
//
// AddTxWithChain panics if the transaction cannot be executed. In addition to
// the protocol-imposed limitations (gas limit, etc.), there are some
// further limitations on the content of transactions that can be
// added. If contract code relies on the BLOCKHASH instruction,
// the block in chain will be returned.
func (b *BlockGen) AddTxWithChain(bc *BlockChain, tx *model.Transaction) {
	if b.gasPool == nil {
		b.SetCoinbase(common.Address{})
	}
	b.statedb.Prepare(tx.Hash(), len(b.txs))
	receipt, err := ApplyTransaction(b.config, bc, &b.header.Coinbase, b.gasPool, b.statedb, b.header, tx, &b.header.GasUsed, evm.Config{})
	if err != nil {
		panic(err)
	}
	b.txs = append(b.txs, tx)
	b.receipts = append(b.receipts, receipt)
}

// GetBalance returns the balance of the given address at the generated block.
func (b *BlockGen) GetBalance(addr common.Address) *big.Int {
	return b.statedb.GetBalance(addr)
}

// AddUncheckedTx forcefully adds a transaction to the block without any
// validation.
//
// AddUncheckedTx will cause consensus failures when used during real
// chain processing. This is best used in conjunction with raw block insertion.
func (b *BlockGen) AddUncheckedTx(tx *model.Transaction) {
	b.txs = append(b.txs, tx)
}

// Number returns the block number of the block being generated.
func (b *BlockGen) Number() *big.Int {
	return new(big.Int).Set(b.header.Number)
}

// BaseFee returns the EIP-1559 base fee of the block being generated.
func (b *BlockGen) BaseFee() *big.Int {
	return new(big.Int).Set(b.header.BaseFee)
}

// AddUncheckedReceipt forcefully adds a receipts to the block without a
// backing transaction.
//
// AddUncheckedReceipt will cause consensus failures when used during real
// chain processing. This is best used in conjunction with raw block insertion.
func (b *BlockGen) AddUncheckedReceipt(receipt *model.Receipt) {
	b.receipts = append(b.receipts, receipt)
}

// TxNonce returns the next valid transaction nonce for the
// account at addr. It panics if the account does not exist.
func (b *BlockGen) TxNonce(addr common.Address) uint64 {
	if !b.statedb.Exist(addr) {
		panic("account does not exist")
	}
	return b.statedb.GetNonce(addr)
}

// AddUncle adds an uncle header to the generated block.
func (b *BlockGen) AddUncle(h *model.Header) {
	// The uncle will have the same timestamp and auto-generated difficulty
	h.Time = b.header.Time

	var parent *model.Header
	for i := b.i - 1; i >= 0; i-- {
		if b.chain[i].Hash() == h.ParentHash {
			parent = b.chain[i].Header()
			break
		}
	}
	if parent == nil {
		panic("block has no parent.") // should never happened
	}

	chainReader := &fakeChainReader{config: b.config}
	h.Difficulty = b.engine.CalcDifficulty(chainReader, b.header.Time, parent)

	// The gas limit and price should be derived from the parent
	h.GasLimit = parent.GasLimit
	if b.config.IsLondon(h.Number) {
		h.BaseFee = misc.CalcBaseFee(b.config, parent)
		if !b.config.IsLondon(parent.Number) {
			parentGasLimit := parent.GasLimit * config.ElasticityMultiplier
			h.GasLimit = CalcGasLimit(parentGasLimit, parentGasLimit)
		}
	}
	b.uncles = append(b.uncles, h)
}

// PrevBlock returns a previously generated block by number. It panics if
// num is greater or equal to the number of the block being generated.
// For index -1, PrevBlock returns the parent block given to GenerateChain.
func (b *BlockGen) PrevBlock(index int) *model.Block {
	if index >= b.i {
		panic(fmt.Errorf("block index %d out of range (%d,%d)", index, -1, b.i))
	}
	if index == -1 {
		return b.parent
	}
	return b.chain[index]
}

// OffsetTime modifies the time instance of a block, implicitly changing its
// associated difficulty. It's useful to test scenarios where forking is not
// tied to chain length directly.
func (b *BlockGen) OffsetTime(seconds int64) {
	b.header.Time += uint64(seconds)
	if b.header.Time <= b.parent.Header().Time {
		panic("block time out of range")
	}
	chainreader := &fakeChainReader{config: b.config}
	b.header.Difficulty = b.engine.CalcDifficulty(chainreader, b.header.Time, b.parent.Header())
}

// GenerateChain creates a chain of n blocks. The first block's
// parent will be the provided parent. db is used to store
// intermediate states and should contain the parent's state trie.
//
// The generator function is called with a new block generator for
// every block. Any transactions and uncles added to the generator
// become part of the block. If gen is nil, the blocks will be empty
// and their coinbase will be the zero address.
//
// Blocks created by GenerateChain do not contain valid proof of work
// values. Inserting them into BlockChain requires use of FakePow or
// a similar non-validating proof of work implementation.
func GenerateChain(configObj *config.ChainConfig, parent *model.Block, engine consensus.Engine, db database.Database, n int, gen func(int, *BlockGen)) ([]*model.Block, []model.Receipts) {
	if configObj == nil {
		configObj = config.TestChainConfig
	}
	blocks, receipts := make(model.Blocks, n), make([]model.Receipts, n)
	chainReader := &fakeChainReader{config: configObj}
	genBlock := func(i int, parent *model.Block, stateDB *state.StateDB) (*model.Block, model.Receipts) {
		log.Debugf("chain maker genBlock: number=%d, parent=%X", i, parent.Hash())

		b := &BlockGen{i: i, chain: blocks, parent: parent, statedb: stateDB, config: configObj, engine: engine}
		b.header = makeHeader(chainReader, parent, stateDB, b.engine)

		// Set the difficulty for clique block. The chain maker doesn't have access
		// to a chain, so the difficulty will be left unset (nil). Set it here to the
		// correct value.
		if b.header.Difficulty == nil {
			if configObj.TerminalTotalDifficulty == nil {
				// Clique chain
				b.header.Difficulty = big.NewInt(2)
			} else {
				// Post-merge chain
				b.header.Difficulty = big.NewInt(0)
			}
		}
		// Execute any user modifications to the block
		if gen != nil {
			log.Debugf("genblock callback, index=%d, blockNum=%d", i, b.header.Number)
			gen(i, b)
		}

		if b.engine != nil {
			log.Debugf("commit stateDB for blockNum=%d", b.header.Number)

			// Finalize and seal the block -- TODO: set claudeContext here
			block, _ := b.engine.FinalizeAndAssemble(chainReader, b.header, stateDB, b.txs, b.uncles, b.receipts)

			// Write state changes to db
			root, err := stateDB.Commit(false)
			if err != nil {
				panic(fmt.Sprintf("state write error: %v", err))
			}
			if err := stateDB.Database().TrieDB().Commit(root, false, nil); err != nil {
				panic(fmt.Sprintf("trie write error: %v", err))
			}
			return block, b.receipts
		}
		return nil, nil
	}
	for i := 0; i < n; i++ {
		statedb, err := state.New(parent.Root(), state.NewDatabase(db), nil)
		if err != nil {
			panic(err)
		}
		block, receipt := genBlock(i, parent, statedb)
		blocks[i] = block
		receipts[i] = receipt
		parent = block
	}
	log.Debugf("chain maker GenerateChain. blocks:%d, receipts:%d, nodes:%d", len(blocks), len(receipts), n)
	return blocks, receipts
}

func makeHeader(chain consensus.ChainReader, parent *model.Block, state *state.StateDB, engine consensus.Engine) *model.Header {
	log.Debugf("chain maker makeHeader. parent:%d", parent.NumberU64(), engine, state)

	var time uint64
	if parent.Time() == 0 {
		time = 10
	} else {
		time = parent.Time() + 10 // block time is fixed at 10 seconds
	}
	header := &model.Header{
		Root:       state.IntermediateRoot(false),
		ParentHash: parent.Hash(),
		Coinbase:   parent.Coinbase(),
		Difficulty: engine.CalcDifficulty(chain, time, &model.Header{
			Number:     parent.Number(),
			Time:       time - 10,
			Difficulty: parent.Difficulty(),
			UncleHash:  parent.UncleHash(),
		}),
		GasLimit: parent.GasLimit(),
		Number:   new(big.Int).Add(parent.Number(), common.Big1),
		Time:     time,
		// ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	if chain.Config().IsLondon(header.Number) {
		header.BaseFee = misc.CalcBaseFee(chain.Config(), parent.Header())
		if !chain.Config().IsLondon(parent.Number()) {
			parentGasLimit := parent.GasLimit() * config.ElasticityMultiplier
			header.GasLimit = CalcGasLimit(parentGasLimit, parentGasLimit)
		}
	}
	return header
}

// makeHeaderChain creates a deterministic chain of headers rooted at parent.
func makeHeaderChain(parent *model.Header, n int, engine consensus.Engine, db database.Database, seed int) []*model.Header {
	log.Debug("chain maker makeHeaderChain", n, parent, engine, seed)
	blocks := makeBlockChain(model.NewBlockWithHeader(parent), n, engine, db, seed)
	headers := make([]*model.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	return headers
}

// makeBlockChain creates a deterministic chain of blocks rooted at parent.
func makeBlockChain(parent *model.Block, n int, engine consensus.Engine, db database.Database, seed int) []*model.Block {
	log.Debug("chain maker makeBlockChain", n, parent, engine, seed)
	blocks, _ := GenerateChain(config.TestChainConfig, parent, engine, db, n, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})
	return blocks
}

type fakeChainReader struct {
	config *config.ChainConfig
}

// Config returns the chain configuration.
func (cr *fakeChainReader) Config() *config.ChainConfig {
	return cr.config
}

func (cr *fakeChainReader) CurrentHeader() *model.Header                { return nil }
func (cr *fakeChainReader) GetHeaderByNumber(uint64) *model.Header      { return nil }
func (cr *fakeChainReader) GetHeaderByHash(common.Hash) *model.Header   { return nil }
func (cr *fakeChainReader) GetHeader(common.Hash, uint64) *model.Header { return nil }
func (cr *fakeChainReader) GetBlock(common.Hash, uint64) *model.Block   { return nil }
func (cr *fakeChainReader) GetTd(common.Hash, uint64) *big.Int          { return nil }
