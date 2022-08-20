package tests

import (
	"github.com/entropyio/go-entropy/config"
	"math/big"
	"testing"
)

var (
	mainnetChainConfig = config.ChainConfig{
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(1150000),
		EIP150Block:    big.NewInt(2463000),
	}
)

func TestDifficulty(t *testing.T) {
	t.Parallel()

	dt := new(testMatcher)
	// Not difficulty-tests
	dt.skipLoad("hexencodetest.*")
	dt.skipLoad("crypto.*")
	dt.skipLoad("blockgenesistest\\.json")
	dt.skipLoad("genesishashestest\\.json")
	dt.skipLoad("keyaddrtest\\.json")
	dt.skipLoad("txtest\\.json")

	// files are 2 years old, contains strange values
	dt.skipLoad("difficultyCustomHomestead\\.json")
	dt.skipLoad("difficultyMorden\\.json")
	dt.skipLoad("difficultyOlimpic\\.json")

	dt.config("Frontier", config.ChainConfig{})

	dt.config("Homestead", config.ChainConfig{
		HomesteadBlock: big.NewInt(0),
	})
	dt.config("MainNetwork", mainnetChainConfig)
	dt.config("CustomMainNetwork", mainnetChainConfig)
	dt.config("difficulty.json", mainnetChainConfig)

	dt.walk(t, difficultyTestDir, func(t *testing.T, name string, test *DifficultyTest) {
		cfg := dt.findConfig(t)
		if test.ParentDifficulty.Cmp(config.MinimumDifficulty) < 0 {
			t.Skip("difficulty below minimum")
			return
		}
		if err := dt.checkFailure(t, test.Run(cfg)); err != nil {
			t.Error(err)
		}
	})
}
