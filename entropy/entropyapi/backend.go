// Package entropyapi implements the general Entropy API functions.
package entropyapi

import (
	"context"
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
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/rpc"
	"math/big"
	"time"
)

// Backend interface provides the common API services (that are provided by
// both full and light clients) with access to necessary functions.
type Backend interface {
	// General Entropy API
	SyncProgress() entropyio.SyncProgress

	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	FeeHistory(ctx context.Context, blockCount int, lastBlock rpc.BlockNumber, rewardPercentiles []float64) (*big.Int, [][]*big.Int, []*big.Int, []float64, error)
	ChainDb() database.Database
	AccountManager() *accounts.Manager
	ExtRPCEnabled() bool
	RPCGasCap() uint64            // global gas cap for eth_call over rpc: DoS protection
	RPCEVMTimeout() time.Duration // global timeout for eth_call over rpc: DoS protection
	RPCTxFeeCap() float64         // global tx fee cap for all transaction related APIs
	UnprotectedAllowed() bool     // allows only for EIP155 transactions.

	// Blockchain API
	SetHead(number uint64)
	HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Header, error)
	HeaderByHash(ctx context.Context, hash common.Hash) (*model.Header, error)
	HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*model.Header, error)
	CurrentHeader() *model.Header
	CurrentBlock() *model.Block
	BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Block, error)
	BlockByHash(ctx context.Context, hash common.Hash) (*model.Block, error)
	BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*model.Block, error)
	StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *model.Header, error)
	StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *model.Header, error)
	PendingBlockAndReceipts() (*model.Block, model.Receipts)
	GetReceipts(ctx context.Context, hash common.Hash) (model.Receipts, error)
	GetTd(ctx context.Context, hash common.Hash) *big.Int
	GetEVM(ctx context.Context, msg blockchain.Message, state *state.StateDB, header *model.Header, vmConfig *evm.Config) (*evm.EVM, func() error, error)
	SubscribeChainEvent(ch chan<- blockchain.ChainEvent) event.Subscription
	SubscribeChainHeadEvent(ch chan<- blockchain.ChainHeadEvent) event.Subscription
	SubscribeChainSideEvent(ch chan<- blockchain.ChainSideEvent) event.Subscription

	// Transaction pool API
	SendTx(ctx context.Context, signedTx *model.Transaction) error
	GetTransaction(ctx context.Context, txHash common.Hash) (*model.Transaction, common.Hash, uint64, uint64, error)
	GetPoolTransactions() (model.Transactions, error)
	GetPoolTransaction(txHash common.Hash) *model.Transaction
	GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error)
	Stats() (pending int, queued int)
	TxPoolContent() (map[common.Address]model.Transactions, map[common.Address]model.Transactions)
	TxPoolContentFrom(addr common.Address) (model.Transactions, model.Transactions)
	SubscribeNewTxsEvent(chan<- blockchain.NewTxsEvent) event.Subscription

	// Filter API
	BloomStatus() (uint64, uint64)
	GetLogs(ctx context.Context, blockHash common.Hash) ([][]*model.Log, error)
	ServiceFilter(ctx context.Context, session *bloombits.MatcherSession)
	SubscribeLogsEvent(ch chan<- []*model.Log) event.Subscription
	SubscribePendingLogsEvent(ch chan<- []*model.Log) event.Subscription
	SubscribeRemovedLogsEvent(ch chan<- blockchain.RemovedLogsEvent) event.Subscription

	ChainConfig() *config.ChainConfig
	Engine() consensus.Engine
}

func GetAPIs(apiBackend Backend) []rpc.API {
	nonceLock := new(AddrLocker)
	return []rpc.API{
		{
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewEntropyAPI(apiBackend),
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewBlockChainAPI(apiBackend),
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewTransactionAPI(apiBackend, nonceLock),
		}, {
			Namespace: "txpool",
			Version:   "1.0",
			Service:   NewTxPoolAPI(apiBackend),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewDebugAPI(apiBackend),
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewEntropyAccountAPI(apiBackend.AccountManager()),
		}, {
			Namespace: "personal",
			Version:   "1.0",
			Service:   NewPersonalAccountAPI(apiBackend, nonceLock),
		},
	}
}
