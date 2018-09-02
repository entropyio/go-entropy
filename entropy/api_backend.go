package entropy

import (
	"context"
	"math/big"

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

// EthAPIBackend implements entropyapi.Backend for full nodes
type EthAPIBackend struct {
	entropy *Entropy
	gpo *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *EthAPIBackend) ChainConfig() *config.ChainConfig {
	return b.entropy.chainConfig
}

func (b *EthAPIBackend) CurrentBlock() *model.Block {
	return b.entropy.blockchain.CurrentBlock()
}

func (b *EthAPIBackend) SetHead(number uint64) {
	b.entropy.protocolManager.downloader.Cancel()
	b.entropy.blockchain.SetHead(number)
}

func (b *EthAPIBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*model.Header, error) {
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

func (b *EthAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*model.Header, error) {
	return b.entropy.blockchain.GetHeaderByHash(hash), nil
}

func (b *EthAPIBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*model.Block, error) {
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

func (b *EthAPIBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *model.Header, error) {
	// Pending state is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block, stateDBObj := b.entropy.miner.Pending()
		return stateDBObj, block.Header(), nil
	}

	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, blockNr)
	if header == nil || err != nil {
		return nil, nil, err
	}
	stateDb, err := b.entropy.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *EthAPIBackend) GetBlock(ctx context.Context, hash common.Hash) (*model.Block, error) {
	return b.entropy.blockchain.GetBlockByHash(hash), nil
}

func (b *EthAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (model.Receipts, error) {
	if number := mapper.ReadHeaderNumber(b.entropy.chainDb, hash); number != nil {
		return mapper.ReadReceipts(b.entropy.chainDb, hash, *number), nil
	}
	return nil, nil
}

func (b *EthAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*model.Log, error) {
	number := mapper.ReadHeaderNumber(b.entropy.chainDb, hash)
	if number == nil {
		return nil, nil
	}
	receipts := mapper.ReadReceipts(b.entropy.chainDb, hash, *number)
	if receipts == nil {
		return nil, nil
	}
	logs := make([][]*model.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (b *EthAPIBackend) GetTd(blockHash common.Hash) *big.Int {
	return b.entropy.blockchain.GetTdByHash(blockHash)
}

func (b *EthAPIBackend) GetEVM(ctx context.Context, msg blockchain.Message, state *state.StateDB, header *model.Header, vmCfg evm.Config) (*evm.EVM, func() error, error) {
	state.SetBalance(msg.From(), mathutil.MaxBig256)
	vmError := func() error { return nil }

	vmContext := blockchain.NewEVMContext(msg, header, b.entropy.BlockChain(), nil)
	return evm.NewEVM(vmContext, state, b.entropy.chainConfig, vmCfg), vmError, nil
}

func (b *EthAPIBackend) SubscribeRemovedLogsEvent(ch chan<- blockchain.RemovedLogsEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainEvent(ch chan<- blockchain.ChainEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeChainEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainHeadEvent(ch chan<- blockchain.ChainHeadEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainSideEvent(ch chan<- blockchain.ChainSideEvent) event.Subscription {
	return b.entropy.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *EthAPIBackend) SubscribeLogsEvent(ch chan<- []*model.Log) event.Subscription {
	return b.entropy.BlockChain().SubscribeLogsEvent(ch)
}

func (b *EthAPIBackend) SendTx(ctx context.Context, signedTx *model.Transaction) error {
	return b.entropy.txPool.AddLocal(signedTx)
}

func (b *EthAPIBackend) GetPoolTransactions() (model.Transactions, error) {
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

func (b *EthAPIBackend) GetPoolTransaction(hash common.Hash) *model.Transaction {
	return b.entropy.txPool.Get(hash)
}

func (b *EthAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.entropy.txPool.State().GetNonce(addr), nil
}

func (b *EthAPIBackend) Stats() (pending int, queued int) {
	return b.entropy.txPool.Stats()
}

func (b *EthAPIBackend) TxPoolContent() (map[common.Address]model.Transactions, map[common.Address]model.Transactions) {
	return b.entropy.TxPool().Content()
}

func (b *EthAPIBackend) SubscribeNewTxsEvent(ch chan<- blockchain.NewTxsEvent) event.Subscription {
	return b.entropy.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *EthAPIBackend) Downloader() *downloader.Downloader {
	return b.entropy.Downloader()
}

func (b *EthAPIBackend) ProtocolVersion() int {
	return b.entropy.EthVersion()
}

func (b *EthAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *EthAPIBackend) ChainDb() database.Database {
	return b.entropy.ChainDb()
}

func (b *EthAPIBackend) EventMux() *event.TypeMux {
	return b.entropy.EventMux()
}

func (b *EthAPIBackend) AccountManager() *account.Manager {
	return b.entropy.AccountManager()
}

func (b *EthAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.entropy.bloomIndexer.Sections()
	return config.BloomBitsBlocks, sections
}

func (b *EthAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.entropy.bloomRequests)
	}
}
