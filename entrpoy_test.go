package entropy_test

import (
	"fmt"
	"github.com/entropyio/go-entropy/accounts"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/genesis"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/server/node"
	"math/big"
	"reflect"
	"testing"
	"time"
)

var (
	key1, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	key2, _    = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	key3, _    = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	baseKey, _ = crypto.HexToECDSA("60b7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6433")

	addr1    = crypto.PubkeyToAddress(key1.PublicKey)
	addr2    = crypto.PubkeyToAddress(key2.PublicKey)
	addr3    = crypto.PubkeyToAddress(key3.PublicKey)
	coinbase = crypto.PubkeyToAddress(baseKey.PublicKey)
)

func printBlockChain(bc *blockchain.BlockChain) {
	for i := bc.CurrentBlock().Number().Uint64(); i > 0; i-- {
		b := bc.GetBlockByNumber(uint64(i))
		fmt.Printf("\n blockChain: number=%d, hash=%X, difficulty=%v\n", i, b.Hash(), b.Difficulty())
	}
}

func TestCreateChainInDB(t *testing.T) {
	var (
		db, _ = mapper.NewLevelDBDatabase("/Users/wangzhen/Desktop/blockchain/GoEntropy/data/entropy_test/chaindata", 0, 0, "test")
	)

	// Ensure that key1 has some funds in the genesis block.
	gspec := &genesis.Genesis{
		Coinbase: coinbase,
		Config:   config.TestChainConfig,
		Alloc: genesis.GenesisAlloc{
			addr1: {Balance: big.NewInt(1000000000)},
			addr2: {Balance: big.NewInt(2000000000)},
			addr3: {Balance: big.NewInt(3000000000)},
		},
	}
	genesisObj := gspec.MustCommit(db)

	// 添加一个block
	// Insert an easy and a difficult chain afterwards
	//signer := model.HomesteadSigner{}

	chain, _ := blockchain.GenerateChain(config.TestChainConfig, genesisObj, ethash.NewFaker(), db, 2, func(i int, gen *blockchain.BlockGen) {
		switch i {
		case 0:
			fmt.Println("in block 1 callback... add 1 transaction")
			// In block 2, addr1 sends some more ether to addr2.
			// addr2 passes it on to addr3.
			//tx1, _ := model.SignTx(model.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(1000), config.TxGas, nil, nil), signer, key1)
			//tx2, _ := model.SignTx(model.NewTransaction(gen.TxNonce(addr2), addr3, big.NewInt(1000), config.TxGas, nil, nil), signer, key2)
			//gen.AddTx(tx1)
			//gen.AddTx(tx2)
		case 1:
			fmt.Println("in block 2 callback... do noting")
		}
	})

	// Import the chain. This runs all block validation rules.
	blockchainObj, _ := blockchain.NewBlockChain(db, nil, gspec.Config, ethash.NewFaker(), evm.Config{}, nil)
	defer blockchainObj.Stop()

	if i, err := blockchainObj.InsertChain(chain); err != nil {
		fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
		return
	}

	state, _ := blockchainObj.State()
	fmt.Printf("last block: #%d\n", blockchainObj.CurrentBlock().Number())
	fmt.Println("balance of coinbase:", state.GetBalance(coinbase))
	fmt.Println("balance of addr1:", state.GetBalance(addr1))
	fmt.Println("balance of addr2:", state.GetBalance(addr2))
	fmt.Println("balance of addr3:", state.GetBalance(addr3))

	printBlockChain(blockchainObj)
}

func TestLoadChainFromDB(t *testing.T) {
	var (
		db, _ = mapper.NewLevelDBDatabase("/Users/wangzhen/Desktop/blockchain/GoEntropy/data/entropy_test/chaindata", 0, 0, "test")
	)

	blockchainObj, _ := blockchain.NewBlockChain(db, nil, config.TestChainConfig, ethash.NewFaker(), evm.Config{}, nil)
	defer blockchainObj.Stop()

	state, _ := blockchainObj.State()
	fmt.Printf("load from LDB last block: #%d\n", blockchainObj.CurrentBlock().Number())
	fmt.Printf("balance of coinbase: %X = %d\n", coinbase, state.GetBalance(coinbase))
	fmt.Printf("balance of addr1: %X = %d\n", addr1, state.GetBalance(addr1))
	fmt.Printf("balance of addr2: %X = %d\n", addr2, state.GetBalance(addr2))
	fmt.Printf("balance of addr3: %X = %d\n", addr3, state.GetBalance(addr3))

	// add transaction
	curNum := blockchainObj.CurrentBlock().Number().Uint64()

	chain, _ := blockchain.GenerateChain(config.TestChainConfig, blockchainObj.GetBlockByNumber(curNum), ethash.NewFaker(), db, 1, func(i int, gen *blockchain.BlockGen) {
		fmt.Println("create new block, number=", i)
		signer := model.HomesteadSigner{}

		action1 := model.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(12345), config.TxGas, nil, nil)
		tx1, _ := model.SignTx(action1, signer, key1)

		action2 := model.NewTransaction(gen.TxNonce(addr2), addr3, big.NewInt(12345), config.TxGas, nil, nil)
		tx2, _ := model.SignTx(action2, signer, key2)
		gen.AddTx(tx1)
		gen.AddTx(tx2)
	})

	if i, err := blockchainObj.InsertChain(chain); err != nil {
		fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
		return
	}

	state, _ = blockchainObj.State()
	newBase := blockchainObj.CurrentBlock().Coinbase()
	fmt.Printf("new last block: #%d\n", blockchainObj.CurrentBlock().Number())
	fmt.Printf("balance of coinbase: %X = %d\n", coinbase, state.GetBalance(coinbase))
	fmt.Printf("balance of newBase: %X = %d\n", newBase, state.GetBalance(newBase))
	fmt.Printf("balance of addr1: %X = %d\n", addr1, state.GetBalance(addr1))
	fmt.Printf("balance of addr2: %X = %d\n", addr2, state.GetBalance(addr2))
	fmt.Printf("balance of addr3: %X = %d\n", addr3, state.GetBalance(addr3))

	//printBlockChain(blockchainObj)
}

func TestMinerStart(t *testing.T) {
	nodeConfig := &node.Config{
		Name:    "entropy_test",
		DataDir: "/Users/wangzhen/Desktop/blockchain/GoEntropy/data",
	}

	ctx := &node.ServiceContext{
		Config:         nodeConfig,
		Services:       make(map[reflect.Type]node.Service),
		EventMux:       new(event.TypeMux),
		AccountManager: new(accounts.Manager),
	}

	config := &entropy.Config{}

	entropyObj, _ := entropy.New(ctx, config)
	entropyObj.SetEntropyBase(coinbase)

	fmt.Printf("entropy backend StartMining...")
	entropyObj.StartMining(1)
	time.Sleep(6000 * time.Second)

	entropyObj.StopMining()
	fmt.Printf("entropy backend StopMining")
}
