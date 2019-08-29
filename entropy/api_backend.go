package entropy

import (
	"context"
	"math/big"

	"errors"
	"github.com/entropyio/go-entropy/account"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/bloombits"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/mathutil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/entropy/gasprice"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/rpc"
)

// EntropyAPIBackend implements entropyapi.Backend for full nodes
type EntropyAPIBackend struct {
	extRPCEnabled bool
	entropy       *Entropy
	gpo           *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *EntropyAPIBackend) ChainConfig() *config.ChainConfig {
	return b.entropy.blockchain.Config()
}

func (b *EntropyAPIBackend) CurrentBlock() *model.Block {
	return b.entropy.blockchain.CurrentBlock()
}

func (b *EntropyAPIBackend) SetHead(number uint64) {
	b.entropy.protocolManager.downloader.Cancel()
	b.entropy.blockchain.SetHead(number)
}

func (b *EntropyAPIBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*model.Header, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.entropy.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.entropy.blockchain.CurrentBlock().Header(), nil
	}
	return b.entropy.blockchain.GetHeaderByNumber(uint64(blockNr)), nil
}

func (b *EntropyAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*model.Header, error) {
	return b.entropy.blockchain.GetHeaderByHash(hash), nil
}

func (b *EntropyAPIBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*model.Block, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.entropy.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.entropy.blockchain.CurrentBlock(), nil
	}
	return b.entropy.blockchain.GetBlockByNumber(uint64(blockNr)), nil
}

func (b *EntropyAPIBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *model.Header, error) {
	// Pending state is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block, stateDBObj := b.entropy.miner.Pending()
		return stateDBObj, block.Header(), nil
	}

	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, blockNr)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.entropy.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *EntropyAPIBackend) GetBlock(ctx context.Context, hash common.Hash) (*model.Block, error) {
	return b.entropy.blockchain.GetBlockByHash(hash), nil
}

func (b *EntropyAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (model.Receipts, error) {
	return b.entropy.blockchain.GetReceiptsByHash(hash), nil
}

func (b *EntropyAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*model.Log, error) {
	receipts := b.entropy.blockchain.GetReceiptsByHash(hash)
	if receipts == nil {
		return nil, nil
	}
	logs := make([][]*model.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (b *EntropyAPIBackend) GetTd(blockHash common.Hash) *big.Int {
	return b.entropy.blockchain.GetTdByHash(blockHash)
}

func (b *EntropyAPIBackend) GetEVM(ctx context.Context, msg blockchain.Message, state *state.StateDB, header *model.Header) (*evm.EVM, func() error, error) {
	state.SetBalance(msg.From(), mathutil.MaxBig256)
	vmError := func() error { return nil }

	vmContext := blockchain.NewEVMContext(msg, header, b.entropy.BlockChain(), nil)
	return evm.NewEVM(vmContext, state, b.entropy.blockchain.Config(), *b.entropy.blockchain.GetVMConfig()), vmError, nil
}

func (b *EntropyAPIBackend) SubscribeRemovedLogsEvent(ch chan<- blockchain.RemovedLogsEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeRemovedLogsEvent(ch)
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
	pending, err := b.entropy.txPool.Pending()
	if err != nil {
		return nil, err
	}
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
	tx, blockHash, blockNumber, index := mapper.ReadTransaction(b.entropy.ChainDb(), txHash)
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

func (b *EntropyAPIBackend) SubscribeNewTxsEvent(ch chan<- blockchain.NewTxsEvent) event.Subscription {
	return b.entropy.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *EntropyAPIBackend) Downloader() *downloader.Downloader {
	return b.entropy.Downloader()
}

func (b *EntropyAPIBackend) ProtocolVersion() int {
	return b.entropy.EthVersion()
}

func (b *EntropyAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *EntropyAPIBackend) ChainDb() database.Database {
	return b.entropy.ChainDb()
}

func (b *EntropyAPIBackend) EventMux() *event.TypeMux {
	return b.entropy.EventMux()
}

func (b *EntropyAPIBackend) AccountManager() *account.Manager {
	return b.entropy.AccountManager()
}

func (b *EntropyAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *EntropyAPIBackend) RPCGasCap() *big.Int {
	return b.entropy.config.RPCGasCap
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
