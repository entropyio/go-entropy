package filters

import (
	"context"
	"io/ioutil"
	"math/big"
	"os"
	"testing"

	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/genesis"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/event"
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
	dir, err := ioutil.TempDir("", "filtertest")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var (
		db, _      = database.NewLDBDatabase(dir, 0, 0)
		mux        = new(event.TypeMux)
		txFeed     = new(event.Feed)
		rmLogsFeed = new(event.Feed)
		logsFeed   = new(event.Feed)
		chainFeed  = new(event.Feed)
		backend    = &testBackend{mux, db, 0, txFeed, rmLogsFeed, logsFeed, chainFeed}
		key1, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1      = crypto.PubkeyToAddress(key1.PublicKey)
		addr2      = common.BytesToAddress([]byte("jeff"))
		addr3      = common.BytesToAddress([]byte("entropy"))
		addr4      = common.BytesToAddress([]byte("random addresses please"))
	)
	defer db.Close()

	genesisObj := genesis.GenesisBlockForTesting(db, addr1, big.NewInt(1000000))
	chain, receipts := blockchain.GenerateChain(config.TestChainConfig, genesisObj, ethash.NewFaker(), db, 100010, func(i int, gen *blockchain.BlockGen) {
		switch i {
		case 2403:
			receipt := makeReceipt(addr1)
			gen.AddUncheckedReceipt(receipt)
		case 1034:
			receipt := makeReceipt(addr2)
			gen.AddUncheckedReceipt(receipt)
		case 34:
			receipt := makeReceipt(addr3)
			gen.AddUncheckedReceipt(receipt)
		case 99999:
			receipt := makeReceipt(addr4)
			gen.AddUncheckedReceipt(receipt)

		}
	})
	for i, block := range chain {
		mapper.WriteBlock(db, block)
		mapper.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
		mapper.WriteHeadBlockHash(db, block.Hash())
		mapper.WriteReceipts(db, block.Hash(), block.NumberU64(), receipts[i])
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
	dir, err := ioutil.TempDir("", "filtertest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var (
		db, _      = database.NewLDBDatabase(dir, 0, 0)
		mux        = new(event.TypeMux)
		txFeed     = new(event.Feed)
		rmLogsFeed = new(event.Feed)
		logsFeed   = new(event.Feed)
		chainFeed  = new(event.Feed)
		backend    = &testBackend{mux, db, 0, txFeed, rmLogsFeed, logsFeed, chainFeed}
		key1, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr       = crypto.PubkeyToAddress(key1.PublicKey)

		hash1 = common.BytesToHash([]byte("topic1"))
		hash2 = common.BytesToHash([]byte("topic2"))
		hash3 = common.BytesToHash([]byte("topic3"))
		hash4 = common.BytesToHash([]byte("topic4"))
	)
	defer db.Close()

	genesisObj := genesis.GenesisBlockForTesting(db, addr, big.NewInt(1000000))
	chain, receipts := blockchain.GenerateChain(config.TestChainConfig, genesisObj, ethash.NewFaker(), db, 1000, func(i int, gen *blockchain.BlockGen) {
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
		case 2:
			receipt := model.NewReceipt(nil, false, 0)
			receipt.Logs = []*model.Log{
				{
					Address: addr,
					Topics:  []common.Hash{hash2},
				},
			}
			gen.AddUncheckedReceipt(receipt)
		case 998:
			receipt := model.NewReceipt(nil, false, 0)
			receipt.Logs = []*model.Log{
				{
					Address: addr,
					Topics:  []common.Hash{hash3},
				},
			}
			gen.AddUncheckedReceipt(receipt)
		case 999:
			receipt := model.NewReceipt(nil, false, 0)
			receipt.Logs = []*model.Log{
				{
					Address: addr,
					Topics:  []common.Hash{hash4},
				},
			}
			gen.AddUncheckedReceipt(receipt)
		}
	})
	for i, block := range chain {
		mapper.WriteBlock(db, block)
		mapper.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
		mapper.WriteHeadBlockHash(db, block.Hash())
		mapper.WriteReceipts(db, block.Hash(), block.NumberU64(), receipts[i])
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
