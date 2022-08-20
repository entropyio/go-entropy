package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/tests"
	"github.com/urfave/cli/v2"
	"os"
)

var stateTestCommand = &cli.Command{
	Action:    stateTestCmd,
	Name:      "statetest",
	Usage:     "executes the given state tests",
	ArgsUsage: "<file>",
}

// StatetestResult contains the execution status after running a state test, any
// error that might have occurred and a dump of the final state if requested.
type StatetestResult struct {
	Name  string      `json:"name"`
	Pass  bool        `json:"pass"`
	Fork  string      `json:"fork"`
	Error string      `json:"error,omitempty"`
	State *state.Dump `json:"state,omitempty"`
}

func stateTestCmd(ctx *cli.Context) error {
	if len(ctx.Args().First()) == 0 {
		return errors.New("path-to-test argument required")
	}

	var tracer evm.EVMLogger
	//switch {
	//case ctx.Bool(MachineFlag.Name):
	//	tracer = logger.NewJSONLogger(config, os.Stderr)
	//
	//case ctx.Bool(DebugFlag.Name):
	//	debugger = logger.NewStructLogger(config)
	//	tracer = debugger
	//
	//default:
	//	debugger = logger.NewStructLogger(config)
	//}
	// Load the test content from the input file
	src, err := os.ReadFile(ctx.Args().First())
	if err != nil {
		return err
	}
	var tests map[string]tests.StateTest
	if err = json.Unmarshal(src, &tests); err != nil {
		return err
	}
	// Iterate over all the tests, run them and aggregate the results
	cfg := evm.Config{
		Tracer: tracer,
		Debug:  ctx.Bool(DebugFlag.Name) || ctx.Bool(MachineFlag.Name),
	}
	results := make([]StatetestResult, 0, len(tests))
	for key, test := range tests {
		for _, st := range test.Subtests() {
			// Run the test and aggregate the result
			result := &StatetestResult{Name: key, Fork: st.Fork, Pass: true}
			_, s, err := test.Run(st, cfg, false)
			// print state root for evmlab tracing
			if ctx.Bool(MachineFlag.Name) && s != nil {
				fmt.Fprintf(os.Stderr, "{\"stateRoot\": \"%x\"}\n", s.IntermediateRoot(false))
			}
			if err != nil {
				// Test failed, mark as so and dump any state to aid debugging
				result.Pass, result.Error = false, err.Error()
				if ctx.Bool(DumpFlag.Name) && s != nil {
					dump := s.RawDump(nil)
					result.State = &dump
				}
			}

			results = append(results, *result)

			// Print any structured logs collected
			if ctx.Bool(DebugFlag.Name) {
				//if debugger != nil {
				//	fmt.Fprintln(os.Stderr, "#### TRACE ####")
				//	logger.WriteTrace(os.Stderr, debugger.StructLogs())
				//}
			}
		}
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(out))
	return nil
}
