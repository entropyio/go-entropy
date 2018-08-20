// Package entropyapi implements the general Entropy API functions.
package entropyapi

import (
	"context"
	"math/big"

	"github.com/entropyio/go-entropy/account"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/rpc"
)

// Backend interface provides the common API services (that are provided by
// both full and light clients) with access to necessary functions.
type Backend interface {
	// General Entropy API
	Downloader() *downloader.Downloader
	ProtocolVersion() int
	SuggestPrice(ctx context.Context) (*big.Int, error)
	ChainDb() database.Database
	EventMux() *event.TypeMux
	AccountManager() *account.Manager

	// BlockChain API
	SetHead(number uint64)
	HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*model.Header, error)
	BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*model.Block, error)
	StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *model.Header, error)
	GetBlock(ctx context.Context, blockHash common.Hash) (*model.Block, error)
	GetReceipts(ctx context.Context, blockHash common.Hash) (model.Receipts, error)
	GetTd(blockHash common.Hash) *big.Int
	GetEVM(ctx context.Context, msg blockchain.Message, state *state.StateDB, header *model.Header, vmCfg evm.Config) (*evm.EVM, func() error, error)
	SubscribeChainEvent(ch chan<- blockchain.ChainEvent) event.Subscription
	SubscribeChainHeadEvent(ch chan<- blockchain.ChainHeadEvent) event.Subscription
	SubscribeChainSideEvent(ch chan<- blockchain.ChainSideEvent) event.Subscription

	// TxPool API
	SendTx(ctx context.Context, signedTx *model.Transaction) error
	GetPoolTransactions() (model.Transactions, error)
	GetPoolTransaction(txHash common.Hash) *model.Transaction
	GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error)
	Stats() (pending int, queued int)
	TxPoolContent() (map[common.Address]model.Transactions, map[common.Address]model.Transactions)
	SubscribeNewTxsEvent(chan<- blockchain.NewTxsEvent) event.Subscription

	ChainConfig() *config.ChainConfig
	CurrentBlock() *model.Block
}

func GetAPIs(apiBackend Backend) []rpc.API {
	nonceLock := new(AddrLocker)
	return []rpc.API{
		{
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewPublicEntropyAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewPublicBlockChainAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewPublicTransactionPoolAPI(apiBackend, nonceLock),
			Public:    true,
		}, {
			Namespace: "txpool",
			Version:   "1.0",
			Service:   NewPublicTxPoolAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(apiBackend),
		}, {
			Namespace: "entropy",
			Version:   "1.0",
			Service:   NewPublicAccountAPI(apiBackend.AccountManager()),
			Public:    true,
		}, {
			Namespace: "personal",
			Version:   "1.0",
			Service:   NewPrivateAccountAPI(apiBackend, nonceLock),
			Public:    false,
		},
	}
}
