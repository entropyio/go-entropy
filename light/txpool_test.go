package light

import (
	"context"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/evm"
	"math"
	"math/big"
	"testing"
	"time"
)

type testTxRelay struct {
	send, discard, mined chan int
}

func (self *testTxRelay) Send(txs model.Transactions) {
	self.send <- len(txs)
}

func (self *testTxRelay) NewHead(head common.Hash, mined []common.Hash, rollback []common.Hash) {
	m := len(mined)
	if m != 0 {
		self.mined <- m
	}
}

func (self *testTxRelay) Discard(hashes []common.Hash) {
	self.discard <- len(hashes)
}

const poolTestTxs = 1000
const poolTestBlocks = 100

// test tx 0..n-1
var testTx [poolTestTxs]*model.Transaction

// txs sent before block i
func sentTx(i int) int {
	return int(math.Pow(float64(i)/float64(poolTestBlocks), 0.9) * poolTestTxs)
}

// txs included in block i or before that (minedTx(i) <= sentTx(i))
func minedTx(i int) int {
	return int(math.Pow(float64(i)/float64(poolTestBlocks), 1.1) * poolTestTxs)
}

func txPoolTestChainGen(i int, block *blockchain.BlockGen) {
	s := minedTx(i)
	e := minedTx(i + 1)
	for i := s; i < e; i++ {
		block.AddTx(testTx[i])
	}
}

func TestTxPool(t *testing.T) {
	for i := range testTx {
		testTx[i], _ = model.SignTx(model.NewTransaction(uint64(i), acc1Addr, big.NewInt(10000), config.TxGas, big.NewInt(config.InitialBaseFee), nil), model.HomesteadSigner{}, testBankKey)
	}

	var (
		sdb   = rawdb.NewMemoryDatabase()
		ldb   = rawdb.NewMemoryDatabase()
		gspec = blockchain.Genesis{
			Alloc:   blockchain.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
			BaseFee: big.NewInt(config.InitialBaseFee),
		}
		genesis = gspec.MustCommit(sdb)
	)
	gspec.MustCommit(ldb)
	// Assemble the test environment
	blockchainObj, _ := blockchain.NewBlockChain(sdb, nil, config.TestChainConfig, ethash.NewFullFaker(), evm.Config{}, nil, nil)
	gchain, _ := blockchain.GenerateChain(config.TestChainConfig, genesis, ethash.NewFaker(), sdb, poolTestBlocks, txPoolTestChainGen)
	if _, err := blockchainObj.InsertChain(gchain); err != nil {
		panic(err)
	}

	odr := &testOdr{sdb: sdb, ldb: ldb, indexerConfig: TestClientIndexerConfig}
	relay := &testTxRelay{
		send:    make(chan int, 1),
		discard: make(chan int, 1),
		mined:   make(chan int, 1),
	}
	lightchain, _ := NewLightChain(odr, config.TestChainConfig, ethash.NewFullFaker(), nil)
	txPermanent = 50
	pool := NewTxPool(config.TestChainConfig, lightchain, relay)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	for ii, block := range gchain {
		i := ii + 1
		s := sentTx(i - 1)
		e := sentTx(i)
		for i := s; i < e; i++ {
			pool.Add(ctx, testTx[i])
			got := <-relay.send
			exp := 1
			if got != exp {
				t.Errorf("relay.Send expected len = %d, got %d", exp, got)
			}
		}

		if _, err := lightchain.InsertHeaderChain([]*model.Header{block.Header()}, 1); err != nil {
			panic(err)
		}

		got := <-relay.mined
		exp := minedTx(i) - minedTx(i-1)
		if got != exp {
			t.Errorf("relay.NewHead expected len(mined) = %d, got %d", exp, got)
		}

		exp = 0
		if i > int(txPermanent)+1 {
			exp = minedTx(i-int(txPermanent)-1) - minedTx(i-int(txPermanent)-2)
		}
		if exp != 0 {
			got = <-relay.discard
			if got != exp {
				t.Errorf("relay.Discard expected len = %d, got %d", exp, got)
			}
		}
	}
}