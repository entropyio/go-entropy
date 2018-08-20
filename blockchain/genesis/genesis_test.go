package genesis

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"

	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/evm"
)

func TestDefaultGenesisBlock(t *testing.T) {
	block := DefaultGenesisBlock().ToBlock(nil)
	if block.Hash() != config.MainnetGenesisHash {
		t.Errorf("wrong mainnet genesis hash, got %v, want %v", block.Hash(), config.MainnetGenesisHash)
	}
	block = DefaultTestnetGenesisBlock().ToBlock(nil)
	if block.Hash() != config.TestnetGenesisHash {
		t.Errorf("wrong testnet genesis hash, got %v, want %v", block.Hash(), config.TestnetGenesisHash)
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
			wantConfig: config.EthashChainConfig,
		},
		{
			name: "mainnet block in DB, genesis == nil",
			fn: func(db database.Database) (*config.ChainConfig, common.Hash, error) {
				DefaultGenesisBlock().MustCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   config.MainnetGenesisHash,
			wantConfig: config.EthashChainConfig,
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
			wantConfig: config.CliqueChainConfig,
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

				bc, _ := blockchain.NewBlockChain(db, nil, oldcustomg.Config, ethash.NewFullFaker(), evm.Config{})
				defer bc.Stop()

				blocks, _ := blockchain.GenerateChain(oldcustomg.Config, genesis, ethash.NewFaker(), db, 4, nil)
				bc.InsertChain(blocks)
				bc.CurrentBlock()
				// This should return a compatibility error.
				return SetupGenesisBlock(db, &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
			wantErr: &config.ConfigCompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(2),
				NewConfig:    big.NewInt(3),
				RewindTo:     1,
			},
		},
	}

	for _, test := range tests {
		db := database.NewMemDatabase()
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
			stored := mapper.ReadBlock(db, test.wantHash, 0)
			if stored.Hash() != test.wantHash {
				t.Errorf("%s: block in DB has hash %s, want %s", test.name, stored.Hash(), test.wantHash)
			}
		}
	}
}
