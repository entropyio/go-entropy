package blockchain

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
)

// NewTxsEvent is posted when a batch of transactions enter the transaction pool.
type NewTxsEvent struct{ Txs []*model.Transaction }

// PendingLogsEvent is posted pre mining and notifies of pending logs.
type PendingLogsEvent struct {
	Logs []*model.Log
}

// NewMinedBlockEvent is posted when a block has been imported.
type NewMinedBlockEvent struct{ Block *model.Block }

// RemovedLogsEvent is posted when a reorg happens
type RemovedLogsEvent struct{ Logs []*model.Log }

type ChainEvent struct {
	Block *model.Block
	Hash  common.Hash
	Logs  []*model.Log
}

type ChainSideEvent struct {
	Block *model.Block
}

type ChainHeadEvent struct{ Block *model.Block }
