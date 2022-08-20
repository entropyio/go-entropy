package light

import (
	"bytes"
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/database/trie"
	"github.com/entropyio/go-entropy/evm"
	"math/big"
	"testing"
)

func TestNodeIterator(t *testing.T) {
	var (
		fulldb  = rawdb.NewMemoryDatabase()
		lightdb = rawdb.NewMemoryDatabase()
		gspec   = blockchain.Genesis{
			Alloc:   blockchain.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
			BaseFee: big.NewInt(config.InitialBaseFee),
		}
		genesis = gspec.MustCommit(fulldb)
	)
	gspec.MustCommit(lightdb)
	chainObj, _ := blockchain.NewBlockChain(fulldb, nil, config.TestChainConfig, ethash.NewFullFaker(), evm.Config{}, nil, nil)
	gchain, _ := blockchain.GenerateChain(config.TestChainConfig, genesis, ethash.NewFaker(), fulldb, 4, testChainGen)
	if _, err := chainObj.InsertChain(gchain); err != nil {
		panic(err)
	}

	ctx := context.Background()
	odr := &testOdr{sdb: fulldb, ldb: lightdb, indexerConfig: TestClientIndexerConfig}
	head := chainObj.CurrentHeader()
	lightTrie, _ := NewStateDatabase(ctx, head, odr).OpenTrie(head.Root)
	fullTrie, _ := state.NewDatabase(fulldb).OpenTrie(head.Root)
	if err := diffTries(fullTrie, lightTrie); err != nil {
		t.Fatal(err)
	}
}

func diffTries(t1, t2 state.Trie) error {
	i1 := trie.NewIterator(t1.NodeIterator(nil))
	i2 := trie.NewIterator(t2.NodeIterator(nil))
	for i1.Next() && i2.Next() {
		if !bytes.Equal(i1.Key, i2.Key) {
			spew.Dump(i2)
			return fmt.Errorf("tries have different keys %x, %x", i1.Key, i2.Key)
		}
		if !bytes.Equal(i1.Value, i2.Value) {
			return fmt.Errorf("tries differ at key %x", i1.Key)
		}
	}
	switch {
	case i1.Err != nil:
		return fmt.Errorf("full trie iterator error: %v", i1.Err)
	case i2.Err != nil:
		return fmt.Errorf("light trie iterator error: %v", i1.Err)
	case i1.Next():
		return fmt.Errorf("full trie iterator has more k/v pairs")
	case i2.Next():
		return fmt.Errorf("light trie iterator has more k/v pairs")
	}
	return nil
}
