package main

import (
	"encoding/json"
	"io"
	"math/big"
	"time"

	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/common/mathutil"
)

type JSONLogger struct {
	encoder *json.Encoder
	cfg     *evm.LogConfig
}

// NewJSONLogger creates a new EVM tracer that prints execution steps as JSON objects
// into the provided stream.
func NewJSONLogger(cfg *evm.LogConfig, writer io.Writer) *JSONLogger {
	return &JSONLogger{json.NewEncoder(writer), cfg}
}

func (l *JSONLogger) CaptureStart(from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) error {
	return nil
}

// CaptureState outputs state information on the logger.
func (l *JSONLogger) CaptureState(env *evm.EVM, pc uint64, op evm.OpCode, gas, cost uint64, memory *evm.Memory, stack *evm.Stack, contract *evm.Contract, depth int, err error) error {
	log := evm.StructLog{
		Pc:         pc,
		Op:         op,
		Gas:        gas,
		GasCost:    cost,
		MemorySize: memory.Len(),
		Storage:    nil,
		Depth:      depth,
		Err:        err,
	}
	if !l.cfg.DisableMemory {
		log.Memory = memory.Data()
	}
	if !l.cfg.DisableStack {
		log.Stack = stack.Data()
	}
	return l.encoder.Encode(log)
}

// CaptureFault outputs state information on the logger.
func (l *JSONLogger) CaptureFault(env *evm.EVM, pc uint64, op evm.OpCode, gas, cost uint64, memory *evm.Memory, stack *evm.Stack, contract *evm.Contract, depth int, err error) error {
	return nil
}

// CaptureEnd is triggered at end of execution.
func (l *JSONLogger) CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) error {
	type endLog struct {
		Output  string                  `json:"output"`
		GasUsed mathutil.HexOrDecimal64 `json:"gasUsed"`
		Time    time.Duration           `json:"time"`
		Err     string                  `json:"error,omitempty"`
	}
	if err != nil {
		return l.encoder.Encode(endLog{common.Bytes2Hex(output), mathutil.HexOrDecimal64(gasUsed), t, err.Error()})
	}
	return l.encoder.Encode(endLog{common.Bytes2Hex(output), mathutil.HexOrDecimal64(gasUsed), t, ""})
}
