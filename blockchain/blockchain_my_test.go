package blockchain

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/genesis"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/evm"
	"math/big"
	"testing"
)

func printBlockChain(bc *BlockChain) {
	for i := bc.CurrentBlock().Number().Uint64(); i > 0; i-- {
		b := bc.GetBlockByNumber(uint64(i))
		fmt.Printf("\n blockChain: hash=%x difficulty=%v\n", b.Hash(), b.Difficulty())
	}
}

func TestMyBlockChain(t *testing.T) {
	// 1. create a full blockchain database
	_, blockchain, err := newCanonical(ethash.NewFaker(), 0, true)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	// 2. 在跟节点下创建两个block
	blocks := makeBlockChain(blockchain.CurrentBlock(), 2, ethash.NewFullFaker(), blockchain.db, 0)
	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatalf("Failed to insert block: %v", err)
	}

	// 3. 添加一个block
	// Insert an easy and a difficult chain afterwards
	easyBlocks, _ := GenerateChain(config.TestChainConfig, blockchain.CurrentBlock(), ethash.NewFaker(), blockchain.db, 1, func(i int, b *BlockGen) {
		fmt.Println("easyBlocks callback...")
	})
	if _, err := blockchain.InsertChain(easyBlocks); err != nil {
		t.Fatalf("failed to insert easy chain: %v", err)
	}

	// 3. 添加一个block
	diffBlocks, _ := GenerateChain(config.TestChainConfig, blockchain.CurrentBlock(), ethash.NewFaker(), blockchain.db, 1, func(i int, b *BlockGen) {
		fmt.Println("diffBlocks callback...")
	})
	if _, err := blockchain.InsertChain(diffBlocks); err != nil {
		t.Fatalf("failed to insert difficult chain: %v", err)
	}

	//if blocks[len(blocks)-1].Hash() != mapper.ReadHeadBlockHash(blockchain.db) {
	//	t.Fatalf("Write/Get HeadBlockHash failed")
	//}

	printBlockChain(blockchain)
}

func TestCreateChainInDB(t *testing.T) {
	var (
		key1, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _    = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _    = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		baseKey, _ = crypto.HexToECDSA("60b7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6433")
		addr1      = crypto.PubkeyToAddress(key1.PublicKey)
		addr2      = crypto.PubkeyToAddress(key2.PublicKey)
		addr3      = crypto.PubkeyToAddress(key3.PublicKey)
		coinbase   = crypto.PubkeyToAddress(baseKey.PublicKey)
		db, _      = mapper.NewLevelDBDatabase("/Users/wangzhen/Desktop/blockchain/test_entropy", 0, 0, "test")
	)

	// Ensure that key1 has some funds in the genesis block.
	gspec := &genesis.Genesis{
		Coinbase: coinbase,
		Config:   config.TestChainConfig,
		Alloc: genesis.GenesisAlloc{
			addr1: {Code: []byte("addr1"), Balance: big.NewInt(1000000)},
			addr2: {Code: []byte("addr2"), Balance: big.NewInt(2000000)},
			addr3: {Code: []byte("addr3"), Balance: big.NewInt(3000000)},
		},
	}
	genesisObj := gspec.MustCommit(db)

	// 添加一个block
	// Insert an easy and a difficult chain afterwards
	//signer := model.HomesteadSigner{}

	chain, _ := GenerateChain(config.TestChainConfig, genesisObj, ethash.NewFaker(), db, 2, func(i int, gen *BlockGen) {
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
	blockchain, _ := NewBlockChain(db, nil, gspec.Config, ethash.NewFaker(), evm.Config{}, nil)
	defer blockchain.Stop()

	if i, err := blockchain.InsertChain(chain); err != nil {
		fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
		return
	}

	state, _ := blockchain.State()
	fmt.Printf("last block: #%d\n", blockchain.CurrentBlock().Number())
	fmt.Println("balance of coinbase:", state.GetBalance(coinbase))
	fmt.Println("balance of addr1:", state.GetBalance(addr1))
	fmt.Println("balance of addr2:", state.GetBalance(addr2))
	fmt.Println("balance of addr3:", state.GetBalance(addr3))
}

func TestLoadChainFromDB(t *testing.T) {
	var (
		key1, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _    = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _    = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		baseKey, _ = crypto.HexToECDSA("60b7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6433")
		addr1      = crypto.PubkeyToAddress(key1.PublicKey)
		addr2      = crypto.PubkeyToAddress(key2.PublicKey)
		addr3      = crypto.PubkeyToAddress(key3.PublicKey)
		coinbase   = crypto.PubkeyToAddress(baseKey.PublicKey)
		db, _      = mapper.NewLevelDBDatabase("/Users/wangzhen/Desktop/blockchain/test_entropy", 0, 0, "test")
	)

	blockchain, _ := NewBlockChain(db, nil, config.TestChainConfig, ethash.NewFaker(), evm.Config{}, nil)
	defer blockchain.Stop()

	state, _ := blockchain.State()
	fmt.Printf("load from LDB last block: #%d\n", blockchain.CurrentBlock().Number())
	fmt.Printf("balance of coinbase: %X = %d \n", coinbase, 0)
	fmt.Printf("balance of addr1: %X = %d\n", addr1, 0)
	fmt.Printf("balance of addr2: %X = %d\n", addr2, 0)
	fmt.Printf("balance of addr3: %X = %d\n", addr3, state.GetBalance(addr3))

	//chain, _ := GenerateChain(config.TestChainConfig, blockchain.GetBlockByNumber(2), ethash.NewFaker(), db, 1, func(i int, gen *BlockGen) {
	//		fmt.Println("create new block, number=", i)
	//		// In block 2, addr1 sends some more ether to addr2.
	//		// addr2 passes it on to addr3.
	//		//tx1, _ := model.SignTx(model.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(1000), config.TxGas, nil, nil), signer, key1)
	//		//tx2, _ := model.SignTx(model.NewTransaction(gen.TxNonce(addr2), addr3, big.NewInt(1000), config.TxGas, nil, nil), signer, key2)
	//		//gen.AddTx(tx1)
	//		//gen.AddTx(tx2)
	//})
	//
	//if i, err := blockchain.InsertChain(chain); err != nil {
	//	fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
	//	return
	//}
	//
	//state, _ = blockchain.State()
	//fmt.Printf("new last block: #%d\n", blockchain.CurrentBlock().Number())
	//fmt.Println("balance of coinbase:", state.GetBalance(coinbase))
	//fmt.Println("balance of addr1:", state.GetBalance(addr1))
	//fmt.Println("balance of addr2:", state.GetBalance(addr2))
	//fmt.Println("balance of addr3:", state.GetBalance(addr3))
}
