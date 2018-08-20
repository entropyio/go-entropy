package runtime

import (
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/blockchain"
)

func NewEnv(cfg *Config) *evm.EVM {
	context := evm.Context{
		CanTransfer: blockchain.CanTransfer,
		Transfer:    blockchain.Transfer,
		GetHash:     func(uint64) common.Hash { return common.Hash{} },

		Origin:      cfg.Origin,
		Coinbase:    cfg.Coinbase,
		BlockNumber: cfg.BlockNumber,
		Time:        cfg.Time,
		Difficulty:  cfg.Difficulty,
		GasLimit:    cfg.GasLimit,
		GasPrice:    cfg.GasPrice,
	}

	return evm.NewEVM(context, cfg.State, cfg.ChainConfig, cfg.EVMConfig)
}
