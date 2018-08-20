package blockchain

import (
	"fmt"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
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
