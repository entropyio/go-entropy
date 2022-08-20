package ethash

import (
	"encoding/binary"
	"encoding/json"
	"github.com/entropyio/go-entropy/common"
	"math/big"
	"math/rand"
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

func randSlice(min, max uint32) []byte {
	var b = make([]byte, 4)
	rand.Read(b)
	a := binary.LittleEndian.Uint32(b)
	size := min + a%(max-min)
	out := make([]byte, size)
	rand.Read(out)
	return out
}

func TestDifficultyCalculators(t *testing.T) {
	rand.Seed(2)
	for i := 0; i < 5000; i++ {
		// 1 to 300 seconds diff
		var timeDelta = uint64(1 + rand.Uint32()%3000)
		diffBig := new(big.Int).SetBytes(randSlice(2, 10))
		if diffBig.Cmp(config.MinimumDifficulty) < 0 {
			diffBig.Set(config.MinimumDifficulty)
		}
		//rand.Read(difficulty)
		header := &model.Header{
			Difficulty: diffBig,
			Number:     new(big.Int).SetUint64(rand.Uint64() % 50_000_000),
			Time:       rand.Uint64() - timeDelta,
		}
		if rand.Uint32()&1 == 0 {
			header.UncleHash = model.EmptyUncleHash
		}
		bombDelay := new(big.Int).SetUint64(rand.Uint64() % 50_000_000)
		for i, pair := range []struct {
			bigFn  func(time uint64, parent *model.Header) *big.Int
			u256Fn func(time uint64, parent *model.Header) *big.Int
		}{
			{FrontierDifficultyCalculator, CalcDifficultyFrontierU256},
			{HomesteadDifficultyCalculator, CalcDifficultyHomesteadU256},
			{DynamicDifficultyCalculator(bombDelay), MakeDifficultyCalculatorU256(bombDelay)},
		} {
			time := header.Time + timeDelta
			want := pair.bigFn(time, header)
			have := pair.u256Fn(time, header)
			if want.BitLen() > 256 {
				continue
			}
			if want.Cmp(have) != 0 {
				t.Fatalf("pair %d: want %x have %x\nparent.Number: %x\np.Time: %x\nc.Time: %x\nBombdelay: %v\n", i, want, have,
					header.Number, header.Time, time, bombDelay)
			}
		}
	}
}

func BenchmarkDifficultyCalculator(b *testing.B) {
	x1 := makeDifficultyCalculator(big.NewInt(1000000))
	x2 := MakeDifficultyCalculatorU256(big.NewInt(1000000))
	h := &model.Header{
		ParentHash: common.Hash{},
		UncleHash:  model.EmptyUncleHash,
		Difficulty: big.NewInt(0xffffff),
		Number:     big.NewInt(500000),
		Time:       1000000,
	}
	b.Run("big-frontier", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			calcDifficultyFrontier(1000014, h)
		}
	})
	b.Run("u256-frontier", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			CalcDifficultyFrontierU256(1000014, h)
		}
	})
	b.Run("big-homestead", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			calcDifficultyHomestead(1000014, h)
		}
	})
	b.Run("u256-homestead", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			CalcDifficultyHomesteadU256(1000014, h)
		}
	})
	b.Run("big-generic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			x1(1000014, h)
		}
	})
	b.Run("u256-generic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			x2(1000014, h)
		}
	})
}
