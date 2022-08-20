package tracers

import (
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/entropy/tracers/tracelogger"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/tests"
	"math/big"
	"testing"
)

func BenchmarkTransactionTrace(b *testing.B) {
	key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	from := crypto.PubkeyToAddress(key.PublicKey)
	gas := uint64(1000000) // 1M gas
	to := common.HexToAddress("0x00000000000000000000000000000000deadbeef")
	signer := model.LatestSignerForChainID(big.NewInt(1337))
	tx, err := model.SignNewTx(key, signer,
		&model.LegacyTx{
			Nonce:    1,
			GasPrice: big.NewInt(500),
			Gas:      gas,
			To:       &to,
		})
	if err != nil {
		b.Fatal(err)
	}
	txContext := evm.TxContext{
		Origin:   from,
		GasPrice: tx.GasPrice(),
	}
	context := evm.BlockContext{
		CanTransfer: blockchain.CanTransfer,
		Transfer:    blockchain.Transfer,
		Coinbase:    common.Address{},
		BlockNumber: new(big.Int).SetUint64(uint64(5)),
		Time:        new(big.Int).SetUint64(uint64(5)),
		Difficulty:  big.NewInt(0xffffffff),
		GasLimit:    gas,
		BaseFee:     big.NewInt(8),
	}
	alloc := blockchain.GenesisAlloc{}
	// The code pushes 'deadbeef' into memory, then the other params, and calls CREATE2, then returns
	// the address
	loop := []byte{
		byte(evm.JUMPDEST), //  [ count ]
		byte(evm.PUSH1), 0, // jumpdestination
		byte(evm.JUMP),
	}
	alloc[common.HexToAddress("0x00000000000000000000000000000000deadbeef")] = blockchain.GenesisAccount{
		Nonce:   1,
		Code:    loop,
		Balance: big.NewInt(1),
	}
	alloc[from] = blockchain.GenesisAccount{
		Nonce:   1,
		Code:    []byte{},
		Balance: big.NewInt(500000000000000),
	}
	_, stateDB := tests.MakePreState(rawdb.NewMemoryDatabase(), alloc, false)
	// Create the tracer, the EVM environment and run it
	tracer := tracelogger.NewStructLogger(&tracelogger.Config{
		Debug: false,
		//DisableStorage: true,
		//EnableMemory: false,
		//EnableReturnData: false,
	})
	evmObj := evm.NewEVM(context, txContext, stateDB, config.TestChainConfig, evm.Config{Debug: true, Tracer: tracer})
	msg, err := tx.AsMessage(signer, nil)
	if err != nil {
		b.Fatalf("failed to prepare transaction for tracing: %v", err)
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		snap := stateDB.Snapshot()
		st := blockchain.NewStateTransition(evmObj, msg, new(blockchain.GasPool).AddGas(tx.Gas()))
		_, err = st.TransitionDb()
		if err != nil {
			b.Fatal(err)
		}
		stateDB.RevertToSnapshot(snap)
		if have, want := len(tracer.StructLogs()), 244752; have != want {
			b.Fatalf("trace wrong, want %d steps, have %d", want, have)
		}
		tracer.Reset()
	}
}
