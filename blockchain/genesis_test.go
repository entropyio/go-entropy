package blockchain

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/evm"
	"math/big"
	"reflect"
	"testing"
)

func TestInvalidCliqueConfig(t *testing.T) {
	block := DefaultTestnetGenesisBlock()
	block.ExtraData = []byte{}
	if _, err := block.Commit(nil); err == nil {
		t.Fatal("Expected error on invalid clique config")
	}
}

func TestSetupGenesis(t *testing.T) {
	var (
		customghash = common.HexToHash("0x89c99d90b79719238d2645c7642f2c9295246e80775b38cfd162b696817fbd50")
		customg     = Genesis{
			Config: &config.ChainConfig{HomesteadBlock: big.NewInt(3)},
			Alloc: GenesisAlloc{
				{1}: {Balance: big.NewInt(1), Storage: map[common.Hash]common.Hash{{1}: {1}}},
			},
		}
		oldcustomg = customg
	)
	oldcustomg.Config = &config.ChainConfig{HomesteadBlock: big.NewInt(2)}
	tests := []struct {
		name       string
		fn         func(database.Database) (*config.ChainConfig, common.Hash, error)
		wantConfig *config.ChainConfig
		wantHash   common.Hash
		wantErr    error
	}{
		{
			name: "genesis without ChainConfig",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				return SetupGenesisBlock(db, new(Genesis))
			},
			wantErr:    errGenesisNoConfig,
			wantConfig: config.EthashChainConfig,
		},
		{
			name: "no block in DB, genesis == nil",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   config.MainnetGenesisHash,
			wantConfig: config.MainnetChainConfig,
		},
		{
			name: "mainnet block in DB, genesis == nil",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				DefaultGenesisBlock().MustCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   config.MainnetGenesisHash,
			wantConfig: config.MainnetChainConfig,
		},
		{
			name: "custom block in DB, genesis == nil",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				customg.MustCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "custom block in DB, genesis == testnet",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				customg.MustCommit(db)
				return SetupGenesisBlock(db, DefaultTestnetGenesisBlock())
			},
			wantErr:    &GenesisMismatchError{Stored: customghash, New: config.TestnetGenesisHash},
			wantHash:   config.TestnetGenesisHash,
			wantConfig: config.TestnetChainConfig,
		},
		{
			name: "compatible config in DB",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				oldcustomg.MustCommit(db)
				return SetupGenesisBlock(db, &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "incompatible config in DB",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				// Commit the 'old' genesis block with Homestead transition at #2.
				// Advance to block #4, past the homestead transition block of customg.
				genesis := oldcustomg.MustCommit(db)

				bc, _ := NewBlockChain(db, nil, oldcustomg.Config, ethash.NewFullFaker(), evm.Config{}, nil, nil)
				defer bc.Stop()

				blocks, _ := GenerateChain(oldcustomg.Config, genesis, ethash.NewFaker(), db, 4, nil)
				bc.InsertChain(blocks)
				bc.CurrentBlock()
				// This should return a compatibility error.
				return SetupGenesisBlock(db, &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
			wantErr: &config.CompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(2),
				NewConfig:    big.NewInt(3),
				RewindTo:     1,
			},
		},
	}

	for _, test := range tests {
		db := rawdb.NewMemoryDatabase()
		configObj, hash, err := test.fn(db)
		// Check the return values.
		if !reflect.DeepEqual(err, test.wantErr) {
			spewObj := spew.ConfigState{DisablePointerAddresses: true, DisableCapacities: true}
			t.Errorf("%s: returned error %#v, want %#v", test.name, spewObj.NewFormatter(err), spewObj.NewFormatter(test.wantErr))
		}
		if !reflect.DeepEqual(configObj, test.wantConfig) {
			t.Errorf("%s:\nreturned %v\nwant     %v", test.name, configObj, test.wantConfig)
		}
		if hash != test.wantHash {
			t.Errorf("%s: returned hash %s, want %s", test.name, hash.Hex(), test.wantHash.Hex())
		} else if err == nil {
			// Check database content.
			stored := rawdb.ReadBlock(db, test.wantHash, 0)
			if stored.Hash() != test.wantHash {
				t.Errorf("%s: block in DB has hash %s, want %s", test.name, stored.Hash(), test.wantHash)
			}
		}
	}
}

// TestGenesisHashes checks the congruity of default genesis data to
// corresponding hardcoded genesis hash values.
func TestGenesisHashes(t *testing.T) {
	for i, c := range []struct {
		genesis *Genesis
		want    common.Hash
	}{
		{DefaultGenesisBlock(), config.MainnetGenesisHash},
		{DefaultTestnetGenesisBlock(), config.TestnetGenesisHash},
	} {
		// Test via MustCommit
		if have := c.genesis.MustCommit(rawdb.NewMemoryDatabase()).Hash(); have != c.want {
			t.Errorf("case: %d a), want: %s, got: %s", i, c.want.Hex(), have.Hex())
		}
		// Test via ToBlock
		if have := c.genesis.ToBlock(nil).Hash(); have != c.want {
			t.Errorf("case: %d a), want: %s, got: %s", i, c.want.Hex(), have.Hex())
		}
	}
}

func TestGenesis_Commit(t *testing.T) {
	genesis := &Genesis{
		BaseFee: big.NewInt(config.InitialBaseFee),
		Config:  config.TestChainConfig,
		// difficulty is nil
	}

	db := rawdb.NewMemoryDatabase()
	genesisBlock, err := genesis.Commit(db)
	if err != nil {
		t.Fatal(err)
	}

	if genesis.Difficulty != nil {
		t.Fatalf("assumption wrong")
	}

	// This value should have been set as default in the ToBlock method.
	if genesisBlock.Difficulty().Cmp(config.GenesisDifficulty) != 0 {
		t.Errorf("assumption wrong: want: %d, got: %v", config.GenesisDifficulty, genesisBlock.Difficulty())
	}

	// Expect the stored total difficulty to be the difficulty of the genesis block.
	stored := rawdb.ReadTd(db, genesisBlock.Hash(), genesisBlock.NumberU64())

	if stored.Cmp(genesisBlock.Difficulty()) != 0 {
		t.Errorf("inequal difficulty; stored: %v, genesisBlock: %v", stored, genesisBlock.Difficulty())
	}
}

func TestReadWriteGenesisAlloc(t *testing.T) {
	var (
		db    = rawdb.NewMemoryDatabase()
		alloc = &GenesisAlloc{
			{1}: {Balance: big.NewInt(1), Storage: map[common.Hash]common.Hash{{1}: {1}}},
			{2}: {Balance: big.NewInt(2), Storage: map[common.Hash]common.Hash{{2}: {2}}},
		}
		hash = common.HexToHash("0xdeadbeef")
	)
	alloc.write(db, hash)

	var reload GenesisAlloc
	err := reload.UnmarshalJSON(rawdb.ReadGenesisState(db, hash))
	if err != nil {
		t.Fatalf("Failed to load genesis state %v", err)
	}
	if len(reload) != len(*alloc) {
		t.Fatal("Unexpected genesis allocation")
	}
	for addr, account := range reload {
		want, ok := (*alloc)[addr]
		if !ok {
			t.Fatal("Account is not found")
		}
		if !reflect.DeepEqual(want, account) {
			t.Fatal("Unexpected account")
		}
	}
}
