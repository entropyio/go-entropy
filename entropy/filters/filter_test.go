package filters

import (
	"context"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database/rawdb"
	"math/big"
	"testing"
)

func makeReceipt(addr common.Address) *model.Receipt {
	receipt := model.NewReceipt(nil, false, 0)
	receipt.Logs = []*model.Log{
		{Address: addr},
	}
	receipt.Bloom = model.CreateBloom(model.Receipts{receipt})
	return receipt
}

func BenchmarkFilters(b *testing.B) {
	dir := b.TempDir()

	var (
		db, _   = rawdb.NewLevelDBDatabase(dir, 0, 0, "", false)
		backend = &testBackend{db: db}
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = common.BytesToAddress([]byte("jeff"))
		addr3   = common.BytesToAddress([]byte("entropy"))
		addr4   = common.BytesToAddress([]byte("random addresses please"))
	)
	defer db.Close()

	genesis := blockchain.GenesisBlockForTesting(db, addr1, big.NewInt(1000000))
	chain, receipts := blockchain.GenerateChain(config.TestChainConfig, genesis, ethash.NewFaker(), db, 100010, func(i int, gen *blockchain.BlockGen) {
		switch i {
		case 2403:
			receipt := makeReceipt(addr1)
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(999, common.HexToAddress("0x999"), big.NewInt(999), 999, gen.BaseFee(), nil))
		case 1034:
			receipt := makeReceipt(addr2)
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(999, common.HexToAddress("0x999"), big.NewInt(999), 999, gen.BaseFee(), nil))
		case 34:
			receipt := makeReceipt(addr3)
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(999, common.HexToAddress("0x999"), big.NewInt(999), 999, gen.BaseFee(), nil))
		case 99999:
			receipt := makeReceipt(addr4)
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(999, common.HexToAddress("0x999"), big.NewInt(999), 999, gen.BaseFee(), nil))

		}
	})
	for i, block := range chain {
		rawdb.WriteBlock(db, block)
		rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
		rawdb.WriteHeadBlockHash(db, block.Hash())
		rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), receipts[i])
	}
	b.ResetTimer()

	filter := NewRangeFilter(backend, 0, -1, []common.Address{addr1, addr2, addr3, addr4}, nil)

	for i := 0; i < b.N; i++ {
		logs, _ := filter.Logs(context.Background())
		if len(logs) != 4 {
			b.Fatal("expected 4 logs, got", len(logs))
		}
	}
}

func TestFilters(t *testing.T) {
	dir := t.TempDir()

	var (
		db, _   = rawdb.NewLevelDBDatabase(dir, 0, 0, "", false)
		backend = &testBackend{db: db}
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr    = crypto.PubkeyToAddress(key1.PublicKey)

		hash1 = common.BytesToHash([]byte("topic1"))
		hash2 = common.BytesToHash([]byte("topic2"))
		hash3 = common.BytesToHash([]byte("topic3"))
		hash4 = common.BytesToHash([]byte("topic4"))
	)
	defer db.Close()

	genesis := blockchain.GenesisBlockForTesting(db, addr, big.NewInt(1000000))
	chain, receipts := blockchain.GenerateChain(config.TestChainConfig, genesis, ethash.NewFaker(), db, 1000, func(i int, gen *blockchain.BlockGen) {
		switch i {
		case 1:
			receipt := model.NewReceipt(nil, false, 0)
			receipt.Logs = []*model.Log{
				{
					Address: addr,
					Topics:  []common.Hash{hash1},
				},
			}
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(1, common.HexToAddress("0x1"), big.NewInt(1), 1, gen.BaseFee(), nil))
		case 2:
			receipt := model.NewReceipt(nil, false, 0)
			receipt.Logs = []*model.Log{
				{
					Address: addr,
					Topics:  []common.Hash{hash2},
				},
			}
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(2, common.HexToAddress("0x2"), big.NewInt(2), 2, gen.BaseFee(), nil))

		case 998:
			receipt := model.NewReceipt(nil, false, 0)
			receipt.Logs = []*model.Log{
				{
					Address: addr,
					Topics:  []common.Hash{hash3},
				},
			}
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(998, common.HexToAddress("0x998"), big.NewInt(998), 998, gen.BaseFee(), nil))
		case 999:
			receipt := model.NewReceipt(nil, false, 0)
			receipt.Logs = []*model.Log{
				{
					Address: addr,
					Topics:  []common.Hash{hash4},
				},
			}
			gen.AddUncheckedReceipt(receipt)
			gen.AddUncheckedTx(model.NewTransaction(999, common.HexToAddress("0x999"), big.NewInt(999), 999, gen.BaseFee(), nil))
		}
	})
	for i, block := range chain {
		rawdb.WriteBlock(db, block)
		rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
		rawdb.WriteHeadBlockHash(db, block.Hash())
		rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), receipts[i])
	}

	filter := NewRangeFilter(backend, 0, -1, []common.Address{addr}, [][]common.Hash{{hash1, hash2, hash3, hash4}})

	logs, _ := filter.Logs(context.Background())
	if len(logs) != 4 {
		t.Error("expected 4 log, got", len(logs))
	}

	filter = NewRangeFilter(backend, 900, 999, []common.Address{addr}, [][]common.Hash{{hash3}})
	logs, _ = filter.Logs(context.Background())
	if len(logs) != 1 {
		t.Error("expected 1 log, got", len(logs))
	}
	if len(logs) > 0 && logs[0].Topics[0] != hash3 {
		t.Errorf("expected log[0].Topics[0] to be %x, got %x", hash3, logs[0].Topics[0])
	}

	filter = NewRangeFilter(backend, 990, -1, []common.Address{addr}, [][]common.Hash{{hash3}})
	logs, _ = filter.Logs(context.Background())
	if len(logs) != 1 {
		t.Error("expected 1 log, got", len(logs))
	}
	if len(logs) > 0 && logs[0].Topics[0] != hash3 {
		t.Errorf("expected log[0].Topics[0] to be %x, got %x", hash3, logs[0].Topics[0])
	}

	filter = NewRangeFilter(backend, 1, 10, nil, [][]common.Hash{{hash1, hash2}})

	logs, _ = filter.Logs(context.Background())
	if len(logs) != 2 {
		t.Error("expected 2 log, got", len(logs))
	}

	failHash := common.BytesToHash([]byte("fail"))
	filter = NewRangeFilter(backend, 0, -1, nil, [][]common.Hash{{failHash}})

	logs, _ = filter.Logs(context.Background())
	if len(logs) != 0 {
		t.Error("expected 0 log, got", len(logs))
	}

	failAddr := common.BytesToAddress([]byte("failmenow"))
	filter = NewRangeFilter(backend, 0, -1, []common.Address{failAddr}, nil)

	logs, _ = filter.Logs(context.Background())
	if len(logs) != 0 {
		t.Error("expected 0 log, got", len(logs))
	}

	filter = NewRangeFilter(backend, 0, -1, nil, [][]common.Hash{{failHash}, {hash1}})

	logs, _ = filter.Logs(context.Background())
	if len(logs) != 0 {
		t.Error("expected 0 log, got", len(logs))
	}
}
