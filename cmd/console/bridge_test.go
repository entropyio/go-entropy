package console

import (
	"github.com/dop251/goja"
	"github.com/entropyio/go-entropy/cmd/console/jsre"
	"testing"
)

// TestUndefinedAsParam ensures that personal functions can receive
// `undefined` as a parameter.
func TestUndefinedAsParam(t *testing.T) {
	b := bridge{}
	call := jsre.Call{}
	call.Arguments = []goja.Value{goja.Undefined()}

	b.UnlockAccount(call)
	b.Sign(call)
	b.Sleep(call)
}

// TestNullAsParam ensures that personal functions can receive
// `null` as a parameter.
func TestNullAsParam(t *testing.T) {
	b := bridge{}
	call := jsre.Call{}
	call.Arguments = []goja.Value{goja.Null()}

	b.UnlockAccount(call)
	b.Sign(call)
	b.Sleep(call)
}