package gasprice

import (
	"context"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/rpc"
	"math"
	"math/big"
	"testing"
)

const testHead = 32

type testBackend struct {
	chain   *blockchain.BlockChain
	pending bool // pending block available
}

func (b *testBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Header, error) {
	if number > testHead {
		return nil, nil
	}
	if number == rpc.LatestBlockNumber {
		number = testHead
	}
	if number == rpc.PendingBlockNumber {
		if b.pending {
			number = testHead + 1
		} else {
			return nil, nil
		}
	}
	return b.chain.GetHeaderByNumber(uint64(number)), nil
}

func (b *testBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Block, error) {
	if number > testHead {
		return nil, nil
	}
	if number == rpc.LatestBlockNumber {
		number = testHead
	}
	if number == rpc.PendingBlockNumber {
		if b.pending {
			number = testHead + 1
		} else {
			return nil, nil
		}
	}
	return b.chain.GetBlockByNumber(uint64(number)), nil
}

func (b *testBackend) GetReceipts(ctx context.Context, hash common.Hash) (model.Receipts, error) {
	return b.chain.GetReceiptsByHash(hash), nil
}

func (b *testBackend) PendingBlockAndReceipts() (*model.Block, model.Receipts) {
	if b.pending {
		block := b.chain.GetBlockByNumber(testHead + 1)
		return block, b.chain.GetReceiptsByHash(block.Hash())
	}
	return nil, nil
}

func (b *testBackend) ChainConfig() *config.ChainConfig {
	return b.chain.Config()
}

func (b *testBackend) SubscribeChainHeadEvent(ch chan<- blockchain.ChainHeadEvent) event.Subscription {
	return nil
}

func newTestBackend(t *testing.T, londonBlock *big.Int, pending bool) *testBackend {
	var (
		key, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr      = crypto.PubkeyToAddress(key.PublicKey)
		configObj = *config.TestChainConfig // needs copy because it is modified below
		gspec     = &blockchain.Genesis{
			Config: &configObj,
			Alloc:  blockchain.GenesisAlloc{addr: {Balance: big.NewInt(math.MaxInt64)}},
		}
		signer = model.LatestSigner(gspec.Config)
	)
	configObj.LondonBlock = londonBlock
	configObj.ArrowGlacierBlock = londonBlock
	configObj.GrayGlacierBlock = londonBlock
	engine := ethash.NewFaker()
	db := rawdb.NewMemoryDatabase()
	genesis, err := gspec.Commit(db)
	if err != nil {
		t.Fatal(err)
	}
	// Generate testing blocks
	blocks, _ := blockchain.GenerateChain(gspec.Config, genesis, engine, db, testHead+1, func(i int, b *blockchain.BlockGen) {
		b.SetCoinbase(common.Address{1})

		var txdata model.TxData
		if londonBlock != nil && b.Number().Cmp(londonBlock) >= 0 {
			txdata = &model.DynamicFeeTx{
				ChainID:   gspec.Config.ChainID,
				Nonce:     b.TxNonce(addr),
				To:        &common.Address{},
				Gas:       30000,
				GasFeeCap: big.NewInt(100 * config.GWei),
				GasTipCap: big.NewInt(int64(i+1) * config.GWei),
				Data:      []byte{},
			}
		} else {
			txdata = &model.LegacyTx{
				Nonce:    b.TxNonce(addr),
				To:       &common.Address{},
				Gas:      21000,
				GasPrice: big.NewInt(int64(i+1) * config.GWei),
				Value:    big.NewInt(100),
				Data:     []byte{},
			}
		}
		b.AddTx(model.MustSignNewTx(key, signer, txdata))
	})
	// Construct testing chain
	diskdb := rawdb.NewMemoryDatabase()
	gspec.Commit(diskdb)
	chain, err := blockchain.NewBlockChain(diskdb, &blockchain.CacheConfig{TrieCleanNoPrefetch: true}, gspec.Config, engine, evm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create local chain, %v", err)
	}
	chain.InsertChain(blocks)
	return &testBackend{chain: chain, pending: pending}
}

func (b *testBackend) CurrentHeader() *model.Header {
	return b.chain.CurrentHeader()
}

func (b *testBackend) GetBlockByNumber(number uint64) *model.Block {
	return b.chain.GetBlockByNumber(number)
}

func TestSuggestTipCap(t *testing.T) {
	configObj := Config{
		Blocks:     3,
		Percentile: 60,
		Default:    big.NewInt(config.GWei),
	}
	var cases = []struct {
		fork   *big.Int // London fork number
		expect *big.Int // Expected gasprice suggestion
	}{
		{nil, big.NewInt(config.GWei * int64(30))},
		{big.NewInt(0), big.NewInt(config.GWei * int64(30))},  // Fork point in genesis
		{big.NewInt(1), big.NewInt(config.GWei * int64(30))},  // Fork point in first block
		{big.NewInt(32), big.NewInt(config.GWei * int64(30))}, // Fork point in last block
		{big.NewInt(33), big.NewInt(config.GWei * int64(30))}, // Fork point in the future
	}
	for _, c := range cases {
		backend := newTestBackend(t, c.fork, false)
		oracle := NewOracle(backend, configObj)

		// The gas price sampled is: 32G, 31G, 30G, 29G, 28G, 27G
		got, err := oracle.SuggestTipCap(context.Background())
		if err != nil {
			t.Fatalf("Failed to retrieve recommended gas price: %v", err)
		}
		if got.Cmp(c.expect) != 0 {
			t.Fatalf("Gas price mismatch, want %d, got %d", c.expect, got)
		}
	}
}
