package rawdb

import (
	"bytes"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
	"golang.org/x/crypto/sha3"
	"hash"
	"math/big"
	"testing"
)

// testHasher is the helper tool for transaction/receipt list hashing.
// The original hasher is trie, in order to get rid of import cycle,
// use the testing hasher instead.
type testHasher struct {
	hasher hash.Hash
}

func newHasher() *testHasher {
	return &testHasher{hasher: sha3.NewLegacyKeccak256()}
}

func (h *testHasher) Reset() {
	h.hasher.Reset()
}

func (h *testHasher) Update(key, val []byte) {
	h.hasher.Write(key)
	h.hasher.Write(val)
}

func (h *testHasher) Hash() common.Hash {
	return common.BytesToHash(h.hasher.Sum(nil))
}

// Tests that positional lookup metadata can be stored and retrieved.
func TestLookupStorage(t *testing.T) {
	tests := []struct {
		name                        string
		writeTxLookupEntriesByBlock func(database.Writer, *model.Block)
	}{
		{
			"DatabaseV6",
			func(db database.Writer, block *model.Block) {
				WriteTxLookupEntriesByBlock(db, block)
			},
		},
		{
			"DatabaseV4-V5",
			func(db database.Writer, block *model.Block) {
				for _, tx := range block.Transactions() {
					db.Put(txLookupKey(tx.Hash()), block.Hash().Bytes())
				}
			},
		},
		{
			"DatabaseV3",
			func(db database.Writer, block *model.Block) {
				for index, tx := range block.Transactions() {
					entry := LegacyTxLookupEntry{
						BlockHash:  block.Hash(),
						BlockIndex: block.NumberU64(),
						Index:      uint64(index),
					}
					data, _ := rlp.EncodeToBytes(entry)
					db.Put(txLookupKey(tx.Hash()), data)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := NewMemoryDatabase()

			tx1 := model.NewTransaction(1, common.BytesToAddress([]byte{0x11}), big.NewInt(111), 1111, big.NewInt(11111), []byte{0x11, 0x11, 0x11})
			tx2 := model.NewTransaction(2, common.BytesToAddress([]byte{0x22}), big.NewInt(222), 2222, big.NewInt(22222), []byte{0x22, 0x22, 0x22})
			tx3 := model.NewTransaction(3, common.BytesToAddress([]byte{0x33}), big.NewInt(333), 3333, big.NewInt(33333), []byte{0x33, 0x33, 0x33})
			txs := []*model.Transaction{tx1, tx2, tx3}

			block := model.NewBlock(&model.Header{Number: big.NewInt(314)}, txs, nil, nil, newHasher())

			// Check that no transactions entries are in a pristine database
			for i, tx := range txs {
				if txn, _, _, _ := ReadTransaction(db, tx.Hash()); txn != nil {
					t.Fatalf("tx #%d [%x]: non existent transaction returned: %v", i, tx.Hash(), txn)
				}
			}
			// Insert all the transactions into the database, and verify contents
			WriteCanonicalHash(db, block.Hash(), block.NumberU64())
			WriteBlock(db, block)
			tc.writeTxLookupEntriesByBlock(db, block)

			for i, tx := range txs {
				if txn, hash, number, index := ReadTransaction(db, tx.Hash()); txn == nil {
					t.Fatalf("tx #%d [%x]: transaction not found", i, tx.Hash())
				} else {
					if hash != block.Hash() || number != block.NumberU64() || index != uint64(i) {
						t.Fatalf("tx #%d [%x]: positional metadata mismatch: have %x/%d/%d, want %x/%v/%v", i, tx.Hash(), hash, number, index, block.Hash(), block.NumberU64(), i)
					}
					if tx.Hash() != txn.Hash() {
						t.Fatalf("tx #%d [%x]: transaction mismatch: have %v, want %v", i, tx.Hash(), txn, tx)
					}
				}
			}
			// Delete the transactions and check purge
			for i, tx := range txs {
				DeleteTxLookupEntry(db, tx.Hash())
				if txn, _, _, _ := ReadTransaction(db, tx.Hash()); txn != nil {
					t.Fatalf("tx #%d [%x]: deleted transaction returned: %v", i, tx.Hash(), txn)
				}
			}
		})
	}
}

func TestDeleteBloomBits(t *testing.T) {
	// Prepare testing data
	db := NewMemoryDatabase()
	for i := uint(0); i < 2; i++ {
		for s := uint64(0); s < 2; s++ {
			WriteBloomBits(db, i, s, config.MainnetGenesisHash, []byte{0x01, 0x02})
		}
	}
	check := func(bit uint, section uint64, head common.Hash, exist bool) {
		bits, _ := ReadBloomBits(db, bit, section, head)
		if exist && !bytes.Equal(bits, []byte{0x01, 0x02}) {
			t.Fatalf("Bloombits mismatch")
		}
		if !exist && len(bits) > 0 {
			t.Fatalf("Bloombits should be removed")
		}
	}
	// Check the existence of written data.
	check(0, 0, config.MainnetGenesisHash, true)

	// Check the existence of deleted data.
	DeleteBloombits(db, 0, 0, 1)
	check(0, 0, config.MainnetGenesisHash, false)
	check(0, 1, config.MainnetGenesisHash, true)

	// Check the existence of deleted data.
	DeleteBloombits(db, 0, 0, 2)
	check(0, 0, config.MainnetGenesisHash, false)
	check(0, 1, config.MainnetGenesisHash, false)

	// Bit1 shouldn't be affect.
	check(1, 0, config.MainnetGenesisHash, true)
	check(1, 1, config.MainnetGenesisHash, true)
}
