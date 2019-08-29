package rlputil

import (
	"bytes"
	"fmt"
	"io"
)

type MyCoolType struct {
	Name string
	a, b uint
}

// EncodeRLP writes x as RLP list [a, b] that omits the Name field.
func (x *MyCoolType) EncodeRLP(w io.Writer) (err error) {
	// Note: the receiver can be a nil pointer. This allows you to
	// control the encoding of nil, but it also means that you have to
	// check for a nil receiver.
	if x == nil {
		err = Encode(w, []uint{0, 0})
	} else {
		err = Encode(w, []uint{x.a, x.b})
	}
	return err
}

func ExampleEncoder() {
	var t *MyCoolType // t is nil pointer to MyCoolType
	bs, _ := EncodeToBytes(t)
	fmt.Printf("%v → %X\n", t, bs)
	buf := new(bytes.Buffer)
	t = &MyCoolType{Name: "foobar", a: 5, b: 6}
	Encode(buf, t)
	fmt.Printf("%v → %X\n", t, buf)

	// Output:
	// <nil> → C28080
	// &{foobar 5 6} → C20506
}
