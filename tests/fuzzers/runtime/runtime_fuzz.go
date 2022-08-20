package runtime

import (
	"github.com/entropyio/go-entropy/evm/runtime"
)

// Fuzz is the basic entry point for the go-fuzz tool
//
// This returns 1 for valid parsable/runable code, 0
// for invalid opcode.
func Fuzz(input []byte) int {
	_, _, err := runtime.Execute(input, input, &runtime.Config{
		GasLimit: 12000000,
	})
	// invalid opcode
	if err != nil && len(err.Error()) > 6 && err.Error()[:7] == "invalid" {
		return 0
	}
	return 1
}
