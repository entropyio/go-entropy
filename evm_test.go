package entropy_test

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/evm/runtime"
	"math/big"
	"testing"
)

func TestEVM_Call(t *testing.T) {
	from := common.HexToAddress("0xf7fe84ec6d79bb7ae74ee5c301a551b0440b27e2")
	to := common.HexToAddress("0xaaf9025f1d9c2d2d36175011e7eca37c453174d0")
	data := common.Hex2Bytes("c6888fa1000000000000000000000000000000000000000000000000000000000000000c")
	contractCode := common.Hex2Bytes("60606040526000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff168063c6888fa114603d575b600080fd5b3415604757600080fd5b605b60048080359060200190919050506071565b6040518082815260200191505060405180910390f35b60006007820290505b9190505600a165627a7a7230582067d7c851e14e862886b6f53dad6825135557fb3a4b691350c94ea5b80605f6770029")
	gas := uint64(9223372036854754343)
	value := big.NewInt(0)

	cfg := runtime.Config{
		Coinbase:    from,
		BlockNumber: value,
		Origin:      from,
		GasLimit:    gas,
		GasPrice:    big.NewInt(10000),
		Difficulty:  big.NewInt(21000),
		Time:        big.NewInt(1536026016957),
		ChainConfig: &config.ChainConfig{
			ChainID:        value,
			HomesteadBlock: big.NewInt(0),
			// Various consensus engines
			Ethash:        config.EthashChainConfig.Ethash,
			Clique:        config.CliqueChainConfig.Clique,
			Claude:        config.ClaudeChainConfig.Claude,
			ConsensusType: config.ConsensusTypeEthash,
		},
	}

	stateDb, _ := state.New(common.Hash{}, state.NewDatabase(mapper.NewMemoryDatabase()))
	stateDb.SetState(from, common.HexToHash("0xf7fe84ec6d79bb7ae74ee5c301a551b0440b27e2"), common.HexToHash("0xf7fe84ec6d79bb7ae74ee5c301a551b0440b27e2"))
	stateDb.AddBalance(from, big.NewInt(420000000000000000))

	stateDb.SetState(to, common.HexToHash("0xaaf9025f1d9c2d2d36175011e7eca37c453174d0"), common.HexToHash("0xaaf9025f1d9c2d2d36175011e7eca37c453174d0"))
	stateDb.SetCode(to, contractCode)
	stateDb.Commit(false)

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
	cfg.State = stateDb

	env := evm.NewEVM(context, stateDb, cfg.ChainConfig, cfg.EVMConfig)

	//func (evm *EVM) Call(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int)
	ret, retgas, vmerr := env.Call(evm.AccountRef(from), to, data, gas, value)

	fmt.Printf("contract call result: %x\n", ret)
	fmt.Println(retgas)
	fmt.Println(vmerr)
}
