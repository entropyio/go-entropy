package entropy

import (
	"context"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy"
	"github.com/entropyio/go-entropy/accounts"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/bloombits"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/entropy/gasprice"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/miner"
	"github.com/entropyio/go-entropy/rpc"
	"math/big"
	"time"
)

// EntropyAPIBackend implements entropyapi.Backend for full nodes
type EntropyAPIBackend struct {
	extRPCEnabled       bool
	allowUnprotectedTxs bool
	entropy             *Entropy
	gpo                 *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *EntropyAPIBackend) ChainConfig() *config.ChainConfig {
	return b.entropy.blockchain.Config()
}

func (b *EntropyAPIBackend) CurrentBlock() *model.Block {
	return b.entropy.blockchain.CurrentBlock()
}

func (b *EntropyAPIBackend) SetHead(number uint64) {
	b.entropy.handler.downloader.Cancel()
	_ = b.entropy.blockchain.SetHead(number)
}

func (b *EntropyAPIBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Header, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.entropy.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.entropy.blockchain.CurrentBlock().Header(), nil
	}
	if number == rpc.FinalizedBlockNumber {
		return b.entropy.blockchain.CurrentFinalizedBlock().Header(), nil
	}
	return b.entropy.blockchain.GetHeaderByNumber(uint64(number)), nil
}

func (b *EntropyAPIBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*model.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.HeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.entropy.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.entropy.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		return header, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *EntropyAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*model.Header, error) {
	return b.entropy.blockchain.GetHeaderByHash(hash), nil
}

func (b *EntropyAPIBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Block, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.entropy.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.entropy.blockchain.CurrentBlock(), nil
	}
	if number == rpc.FinalizedBlockNumber {
		return b.entropy.blockchain.CurrentFinalizedBlock(), nil
	}
	return b.entropy.blockchain.GetBlockByNumber(uint64(number)), nil
}

func (b *EntropyAPIBackend) BlockByHash(ctx context.Context, hash common.Hash) (*model.Block, error) {
	return b.entropy.blockchain.GetBlockByHash(hash), nil
}

func (b *EntropyAPIBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*model.Block, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.BlockByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.entropy.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.entropy.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		block := b.entropy.blockchain.GetBlock(hash, header.Number.Uint64())
		if block == nil {
			return nil, errors.New("header found, but block body is missing")
		}
		return block, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *EntropyAPIBackend) PendingBlockAndReceipts() (*model.Block, model.Receipts) {
	return b.entropy.miner.PendingBlockAndReceipts()
}

func (b *EntropyAPIBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *model.Header, error) {
	// Pending state is only known by the miner
	if number == rpc.PendingBlockNumber {
		block, stateDBObj := b.entropy.miner.Pending()
		return stateDBObj, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.entropy.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *EntropyAPIBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *model.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.StateAndHeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header, err := b.HeaderByHash(ctx, hash)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			return nil, nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.entropy.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, nil, errors.New("hash is not currently canonical")
		}
		stateDb, err := b.entropy.BlockChain().StateAt(header.Root)
		return stateDb, header, err
	}
	return nil, nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *EntropyAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (model.Receipts, error) {
	return b.entropy.blockchain.GetReceiptsByHash(hash), nil
}

func (b *EntropyAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*model.Log, error) {
	db := b.entropy.ChainDb()
	number := rawdb.ReadHeaderNumber(db, hash)
	if number == nil {
		return nil, fmt.Errorf("failed to get block number for hash %#x", hash)
	}
	logs := rawdb.ReadLogs(db, hash, *number, b.entropy.blockchain.Config())
	if logs == nil {
		return nil, fmt.Errorf("failed to get logs for block #%d (0x%s)", *number, hash.TerminalString())
	}
	return logs, nil
}

func (b *EntropyAPIBackend) GetTd(ctx context.Context, hash common.Hash) *big.Int {
	if header := b.entropy.blockchain.GetHeaderByHash(hash); header != nil {
		return b.entropy.blockchain.GetTd(hash, header.Number.Uint64())
	}
	return nil
}

func (b *EntropyAPIBackend) GetEVM(ctx context.Context, msg blockchain.Message, state *state.StateDB, header *model.Header, vmConfig *evm.Config) (*evm.EVM, func() error, error) {
	vmError := func() error { return nil }
	if vmConfig == nil {
		vmConfig = b.entropy.blockchain.GetVMConfig()
	}
	txContext := blockchain.NewEVMTxContext(msg)
	vmContext := blockchain.NewEVMBlockContext(header, b.entropy.BlockChain(), nil)
	return evm.NewEVM(vmContext, txContext, state, b.entropy.blockchain.Config(), *vmConfig), vmError, nil
}

func (b *EntropyAPIBackend) SubscribeRemovedLogsEvent(ch chan<- blockchain.RemovedLogsEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *EntropyAPIBackend) SubscribePendingLogsEvent(ch chan<- []*model.Log) event.Subscription {
	return b.entropy.miner.SubscribePendingLogs(ch)
}

func (b *EntropyAPIBackend) SubscribeChainEvent(ch chan<- blockchain.ChainEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeChainEvent(ch)
}

func (b *EntropyAPIBackend) SubscribeChainHeadEvent(ch chan<- blockchain.ChainHeadEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *EntropyAPIBackend) SubscribeChainSideEvent(ch chan<- blockchain.ChainSideEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *EntropyAPIBackend) SubscribeLogsEvent(ch chan<- []*model.Log) event.Subscription {
	return b.entropy.BlockChain().SubscribeLogsEvent(ch)
}

func (b *EntropyAPIBackend) SendTx(ctx context.Context, signedTx *model.Transaction) error {
	return b.entropy.txPool.AddLocal(signedTx)
}

func (b *EntropyAPIBackend) GetPoolTransactions() (model.Transactions, error) {
	pending := b.entropy.txPool.Pending(false)
	var txs model.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *EntropyAPIBackend) GetPoolTransaction(hash common.Hash) *model.Transaction {
	return b.entropy.txPool.Get(hash)
}

func (b *EntropyAPIBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*model.Transaction, common.Hash, uint64, uint64, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(b.entropy.ChainDb(), txHash)
	return tx, blockHash, blockNumber, index, nil
}

func (b *EntropyAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.entropy.txPool.Nonce(addr), nil
}

func (b *EntropyAPIBackend) Stats() (pending int, queued int) {
	return b.entropy.txPool.Stats()
}

func (b *EntropyAPIBackend) TxPoolContent() (map[common.Address]model.Transactions, map[common.Address]model.Transactions) {
	return b.entropy.TxPool().Content()
}

func (b *EntropyAPIBackend) TxPoolContentFrom(addr common.Address) (model.Transactions, model.Transactions) {
	return b.entropy.TxPool().ContentFrom(addr)
}

func (b *EntropyAPIBackend) TxPool() *blockchain.TxPool {
	return b.entropy.TxPool()
}

func (b *EntropyAPIBackend) SubscribeNewTxsEvent(ch chan<- blockchain.NewTxsEvent) event.Subscription {
	return b.entropy.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *EntropyAPIBackend) SyncProgress() entropyio.SyncProgress {
	return b.entropy.Downloader().Progress()
}

func (b *EntropyAPIBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestTipCap(ctx)
}

func (b *EntropyAPIBackend) FeeHistory(ctx context.Context, blockCount int, lastBlock rpc.BlockNumber, rewardPercentiles []float64) (firstBlock *big.Int, reward [][]*big.Int, baseFee []*big.Int, gasUsedRatio []float64, err error) {
	return b.gpo.FeeHistory(ctx, blockCount, lastBlock, rewardPercentiles)
}

func (b *EntropyAPIBackend) ChainDb() database.Database {
	return b.entropy.ChainDb()
}

func (b *EntropyAPIBackend) EventMux() *event.TypeMux {
	return b.entropy.EventMux()
}

func (b *EntropyAPIBackend) AccountManager() *accounts.Manager {
	return b.entropy.AccountManager()
}

func (b *EntropyAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *EntropyAPIBackend) UnprotectedAllowed() bool {
	return b.allowUnprotectedTxs
}

func (b *EntropyAPIBackend) RPCGasCap() uint64 {
	return b.entropy.config.RPCGasCap
}

func (b *EntropyAPIBackend) RPCEVMTimeout() time.Duration {
	return b.entropy.config.RPCEVMTimeout
}

func (b *EntropyAPIBackend) RPCTxFeeCap() float64 {
	return b.entropy.config.RPCTxFeeCap
}

func (b *EntropyAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.entropy.bloomIndexer.Sections()
	return config.BloomBitsBlocks, sections
}

func (b *EntropyAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.entropy.bloomRequests)
	}
}

func (b *EntropyAPIBackend) Engine() consensus.Engine {
	return b.entropy.engine
}

func (b *EntropyAPIBackend) CurrentHeader() *model.Header {
	return b.entropy.blockchain.CurrentHeader()
}

func (b *EntropyAPIBackend) Miner() *miner.Miner {
	return b.entropy.Miner()
}

func (b *EntropyAPIBackend) StartMining(threads int) error {
	return b.entropy.StartMining(threads)
}

func (b *EntropyAPIBackend) StateAtBlock(ctx context.Context, block *model.Block, reexec uint64, base *state.StateDB, checkLive, preferDisk bool) (*state.StateDB, error) {
	return b.entropy.StateAtBlock(block, reexec, base, checkLive, preferDisk)
}

func (b *EntropyAPIBackend) StateAtTransaction(ctx context.Context, block *model.Block, txIndex int, reexec uint64) (blockchain.Message, evm.BlockContext, *state.StateDB, error) {
	return b.entropy.stateAtTransaction(block, txIndex, reexec)
}
