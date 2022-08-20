package native

import (
	"encoding/json"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/entropy/tracers"
	"github.com/entropyio/go-entropy/evm"
	"math/big"
	"time"
)

func init() {
	register("noopTracer", newNoopTracer)
}

// noopTracer is a go implementation of the Tracer interface which
// performs no action. It's mostly useful for testing purposes.
type noopTracer struct{}

// newNoopTracer returns a new noop tracer.
func newNoopTracer(ctx *tracers.Context) tracers.Tracer {
	return &noopTracer{}
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *noopTracer) CaptureStart(env *evm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *noopTracer) CaptureEnd(output []byte, gasUsed uint64, _ time.Duration, err error) {
}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *noopTracer) CaptureState(pc uint64, op evm.OpCode, gas, cost uint64, scope *evm.ScopeContext, rData []byte, depth int, err error) {
}

// CaptureFault implements the EVMLogger interface to trace an execution fault.
func (t *noopTracer) CaptureFault(pc uint64, op evm.OpCode, gas, cost uint64, _ *evm.ScopeContext, depth int, err error) {
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *noopTracer) CaptureEnter(typ evm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
}

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *noopTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
}

func (*noopTracer) CaptureTxStart(gasLimit uint64) {}

func (*noopTracer) CaptureTxEnd(restGas uint64) {}

// GetResult returns an empty json object.
func (t *noopTracer) GetResult() (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *noopTracer) Stop(err error) {
}
