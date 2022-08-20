package blockchain

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common/crypto"
)

// Tests that transactions can be added to strict lists and list contents and
// nonce boundaries are correctly maintained.
func TestStrictTxListAdd(t *testing.T) {
	// Generate a list of transactions to insert
	key, _ := crypto.GenerateKey()

	txs := make(model.Transactions, 1024)
	for i := 0; i < len(txs); i++ {
		txs[i] = transaction(uint64(i), 0, key)
	}
	// Insert the transactions in a random order
	list := newTxList(true)
	for _, v := range rand.Perm(len(txs)) {
		list.Add(txs[v], DefaultTxPoolConfig.PriceBump)
	}
	// Verify internal state
	if len(list.txs.items) != len(txs) {
		t.Errorf("transaction count mismatch: have %d, want %d", len(list.txs.items), len(txs))
	}
	for i, tx := range txs {
		if list.txs.items[tx.Nonce()] != tx {
			t.Errorf("item %d: transaction mismatch: have %v, want %v", i, list.txs.items[tx.Nonce()], tx)
		}
	}
}

func BenchmarkTxListAdd(b *testing.B) {
	// Generate a list of transactions to insert
	key, _ := crypto.GenerateKey()

	txs := make(model.Transactions, 100000)
	for i := 0; i < len(txs); i++ {
		txs[i] = transaction(uint64(i), 0, key)
	}
	// Insert the transactions in a random order
	priceLimit := big.NewInt(int64(DefaultTxPoolConfig.PriceLimit))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list := newTxList(true)
		for _, v := range rand.Perm(len(txs)) {
			list.Add(txs[v], DefaultTxPoolConfig.PriceBump)
			list.Filter(priceLimit, DefaultTxPoolConfig.PriceBump)
		}
	}
}
