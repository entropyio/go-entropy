package ethash

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common/mathutil"
	"github.com/entropyio/go-entropy/config"
)

type diffTest struct {
	ParentTimestamp    uint64
	ParentDifficulty   *big.Int
	CurrentTimestamp   uint64
	CurrentBlocknumber *big.Int
	CurrentDifficulty  *big.Int
}

func (d *diffTest) UnmarshalJSON(b []byte) (err error) {
	var ext struct {
		ParentTimestamp    string
		ParentDifficulty   string
		CurrentTimestamp   string
		CurrentBlocknumber string
		CurrentDifficulty  string
	}
	if err := json.Unmarshal(b, &ext); err != nil {
		return err
	}

	d.ParentTimestamp = mathutil.MustParseUint64(ext.ParentTimestamp)
	d.ParentDifficulty = mathutil.MustParseBig256(ext.ParentDifficulty)
	d.CurrentTimestamp = mathutil.MustParseUint64(ext.CurrentTimestamp)
	d.CurrentBlocknumber = mathutil.MustParseBig256(ext.CurrentBlocknumber)
	d.CurrentDifficulty = mathutil.MustParseBig256(ext.CurrentDifficulty)

	return nil
}

func TestCalcDifficulty(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "tests", "testdata", "BasicTests", "difficulty.json"))
	if err != nil {
		t.Skip(err)
	}
	defer file.Close()

	tests := make(map[string]diffTest)
	err = json.NewDecoder(file).Decode(&tests)
	if err != nil {
		t.Fatal(err)
	}

	configObj := &config.ChainConfig{HomesteadBlock: big.NewInt(1150000)}

	for name, test := range tests {
		number := new(big.Int).Sub(test.CurrentBlocknumber, big.NewInt(1))
		diff := CalcDifficulty(configObj, test.CurrentTimestamp, &model.Header{
			Number:     number,
			Time:       test.ParentTimestamp,
			Difficulty: test.ParentDifficulty,
		})
		if diff.Cmp(test.CurrentDifficulty) != 0 {
			t.Error(name, "failed. Expected", test.CurrentDifficulty, "and calculated", diff)
		}
	}
}
