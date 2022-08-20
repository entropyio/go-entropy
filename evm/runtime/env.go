package runtime

import (
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/evm"
)

func NewEnv(cfg *Config) *evm.EVM {
	txContext := evm.TxContext{
		Origin:   cfg.Origin,
		GasPrice: cfg.GasPrice,
	}
	blockContext := evm.BlockContext{
		CanTransfer: blockchain.CanTransfer,
		Transfer:    blockchain.Transfer,
		GetHash:     cfg.GetHashFn,
		Coinbase:    cfg.Coinbase,
		BlockNumber: cfg.BlockNumber,
		Time:        cfg.Time,
		Difficulty:  cfg.Difficulty,
		GasLimit:    cfg.GasLimit,
		BaseFee:     cfg.BaseFee,
	}

	return evm.NewEVM(blockContext, txContext, cfg.State, cfg.ChainConfig, cfg.EVMConfig)
}
