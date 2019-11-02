package state

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func BenchmarkCutOriginal(b *testing.B) {
	value := common.HexToHash("0x01")
	for i := 0; i < b.N; i++ {
		bytes.TrimLeft(value[:], "\x00")
	}
}

func BenchmarkCutsetterFn(b *testing.B) {
	value := common.HexToHash("0x01")
	cutSetFn := func(r rune) bool {
		return int32(r) == int32(0)
	}
	for i := 0; i < b.N; i++ {
		bytes.TrimLeftFunc(value[:], cutSetFn)
	}
}

func BenchmarkCutCustomTrim(b *testing.B) {
	value := common.HexToHash("0x01")
	for i := 0; i < b.N; i++ {
		common.TrimLeftZeroes(value[:])
	}
}

func xTestFuzzCutter(t *testing.T) {
	rand.Seed(time.Now().Unix())
	for {
		v := make([]byte, 20)
		zeroes := rand.Intn(21)
		rand.Read(v[zeroes:])
		exp := bytes.TrimLeft(v[:], "\x00")
		got := common.TrimLeftZeroes(v)
		if !bytes.Equal(exp, got) {

			fmt.Printf("Input %x\n", v)
			fmt.Printf("Exp %x\n", exp)
			fmt.Printf("Got %x\n", got)
			t.Fatalf("Error")
		}
		//break
	}
}
