package rlputil

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

type Alloc struct {
	Addr, Balance *big.Int
}

func TestEncodeMy(t *testing.T) {
	//runEncTests(t, EncodeToBytes)
	val := "abcd1234"
	fmt.Println("orignal val: ", val)
	buf := new(bytes.Buffer)
	fmt.Println("original buffer: ", buf)
	err := Encode(buf, val)
	fmt.Println("encode buffer: ", buf)
	fmt.Println("encode error: ", err)

	var val2 string
	Decode(buf, &val2)
	fmt.Println("decode buffer: ", val2)
}

func TestGenesisData(t *testing.T) {
	// set address and balance array
	addr := new(big.Int)
	addr.SetString("5f471f58567e430cf3442c5a06f7d00e6e816548", 16)
	bal := new(big.Int)
	bal.SetString("20000000000000000000000000", 10)
	p := []Alloc{
		{
			addr,
			bal,
		},
	}
	// encode and output
	data, err := EncodeToBytes(p)
	if err != nil {
		panic(err)
	}
	output := strconv.QuoteToASCII(string(data))
	fmt.Println(output)
}

func TestDecodeMy(t *testing.T) {
	var p []Alloc
	// 543940206678041799644575356477183881336580040008
	// 200000000000000000000

	var data = "\xe0\u07d4_G\x1fXV~C\f\xf3D,Z\x06\xf7\xd0\x0en\x81eH\x89\n\u05ce\xbcZ\xc6 \x00\x00"
	var stream = NewStream(strings.NewReader(data), 0)
	err := stream.Decode(&p)
	fmt.Println(p)
	if err != nil {
		panic(err)
	}
}
