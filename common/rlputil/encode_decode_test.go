package rlputil

import (
	"bytes"
	"fmt"
	"testing"
)

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
