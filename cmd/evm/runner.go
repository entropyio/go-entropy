package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/mapper"
	"io/ioutil"
	"math/big"
	"os"
	goruntime "runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/entropyio/go-entropy/blockchain/genesis"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/cmd/evm/internal/compiler"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/evm/runtime"
	"gopkg.in/urfave/cli.v1"
)

var runCommand = cli.Command{
	Action:      runCmd,
	Name:        "run",
	Usage:       "run arbitrary evm binary",
	ArgsUsage:   "<code>",
	Description: `The run command runs arbitrary EVM code.`,
}

// readGenesis will read the given JSON format genesis file and return
// the initialized Genesis structure
func readGenesis(genesisPath string) *genesis.Genesis {
	// Make sure we have a valid genesis JSON
	//genesisPath := ctx.Args().First()
	if len(genesisPath) == 0 {
		utils.Fatalf("Must supply path to genesis JSON file")
	}
	file, err := os.Open(genesisPath)
	if err != nil {
		utils.Fatalf("Failed to read genesis file: %v", err)
	}
	defer file.Close()

	genesisObj := new(genesis.Genesis)
	if err := json.NewDecoder(file).Decode(genesisObj); err != nil {
		utils.Fatalf("invalid genesis file: %v", err)
	}
	return genesisObj
}

func timedExec(bench bool, execFunc func() ([]byte, uint64, error)) ([]byte, uint64, time.Duration, error) {
	var (
		output   []byte
		gasLeft  uint64
		execTime time.Duration
		err      error
	)

	if bench {
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				output, gasLeft, err = execFunc()
			}
		})

		// Get the average execution time from the benchmarking result.
		// There are other useful stats here that could be reported.
		execTime = time.Duration(result.NsPerOp())
	} else {
		startTime := time.Now()
		output, gasLeft, err = execFunc()
		execTime = time.Since(startTime)
	}

	return output, gasLeft, execTime, err
}

func runCmd(ctx *cli.Context) error {
	logconfig := &evm.LogConfig{
		DisableMemory: ctx.GlobalBool(DisableMemoryFlag.Name),
		DisableStack:  ctx.GlobalBool(DisableStackFlag.Name),
		Debug:         ctx.GlobalBool(DebugFlag.Name),
	}

	var (
		tracer        evm.Tracer
		debugLogger   *evm.StructLogger
		statedb       *state.StateDB
		chainConfig   *config.ChainConfig
		sender        = common.BytesToAddress([]byte("sender"))
		receiver      = common.BytesToAddress([]byte("receiver"))
		genesisConfig *genesis.Genesis
	)
	if ctx.GlobalBool(MachineFlag.Name) {
		tracer = evm.NewJSONLogger(logconfig, os.Stdout)
	} else if ctx.GlobalBool(DebugFlag.Name) {
		debugLogger = evm.NewStructLogger(logconfig)
		tracer = debugLogger
	} else {
		debugLogger = evm.NewStructLogger(logconfig)
	}
	if ctx.GlobalString(GenesisFlag.Name) != "" {
		gen := readGenesis(ctx.GlobalString(GenesisFlag.Name))
		genesisConfig = gen
		db := mapper.NewMemoryDatabase()
		genesisObj := gen.ToBlock(db)
		statedb, _ = state.New(genesisObj.Root(), state.NewDatabase(db))
		chainConfig = gen.Config
	} else {
		statedb, _ = state.New(common.Hash{}, state.NewDatabase(mapper.NewMemoryDatabase()))
		genesisConfig = new(genesis.Genesis)
	}
	if ctx.GlobalString(SenderFlag.Name) != "" {
		sender = common.HexToAddress(ctx.GlobalString(SenderFlag.Name))
	}
	statedb.CreateAccount(sender)

	if ctx.GlobalString(ReceiverFlag.Name) != "" {
		receiver = common.HexToAddress(ctx.GlobalString(ReceiverFlag.Name))
	}

	var code []byte
	codeFileFlag := ctx.GlobalString(CodeFileFlag.Name)
	codeFlag := ctx.GlobalString(CodeFlag.Name)

	// The '--code' or '--codefile' flag overrides code in state
	if codeFileFlag != "" || codeFlag != "" {
		var hexcode []byte
		if codeFileFlag != "" {
			var err error
			// If - is specified, it means that code comes from stdin
			if codeFileFlag == "-" {
				//Try reading from stdin
				if hexcode, err = ioutil.ReadAll(os.Stdin); err != nil {
					fmt.Printf("Could not load code from stdin: %v\n", err)
					os.Exit(1)
				}
			} else {
				// Codefile with hex assembly
				if hexcode, err = ioutil.ReadFile(codeFileFlag); err != nil {
					fmt.Printf("Could not load code from file: %v\n", err)
					os.Exit(1)
				}
			}
		} else {
			hexcode = []byte(codeFlag)
		}
		hexcode = bytes.TrimSpace(hexcode)
		if len(hexcode)%2 != 0 {
			fmt.Printf("Invalid input length for hex data (%d)\n", len(hexcode))
			os.Exit(1)
		}
		code = common.FromHex(string(hexcode))
	} else if fn := ctx.Args().First(); len(fn) > 0 {
		// EASM-file to compile
		src, err := ioutil.ReadFile(fn)
		if err != nil {
			return err
		}
		bin, err := compiler.Compile(fn, src, false)
		if err != nil {
			return err
		}
		code = common.Hex2Bytes(bin)
	}

	initialGas := ctx.GlobalUint64(GasFlag.Name)
	if genesisConfig.GasLimit != 0 {
		initialGas = genesisConfig.GasLimit
	}
	runtimeConfig := runtime.Config{
		Origin:      sender,
		State:       statedb,
		GasLimit:    initialGas,
		GasPrice:    utils.GlobalBig(ctx, PriceFlag.Name),
		Value:       utils.GlobalBig(ctx, ValueFlag.Name),
		Difficulty:  genesisConfig.Difficulty,
		Time:        new(big.Int).SetUint64(genesisConfig.Timestamp),
		Coinbase:    genesisConfig.Coinbase,
		BlockNumber: new(big.Int).SetUint64(genesisConfig.Number),
		EVMConfig: evm.Config{
			Tracer:         tracer,
			Debug:          ctx.GlobalBool(DebugFlag.Name) || ctx.GlobalBool(MachineFlag.Name),
			EVMInterpreter: ctx.GlobalString(EVMInterpreterFlag.Name),
		},
	}

	if cpuProfilePath := ctx.GlobalString(CPUProfileFlag.Name); cpuProfilePath != "" {
		f, err := os.Create(cpuProfilePath)
		if err != nil {
			fmt.Println("could not create CPU profile: ", err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Println("could not start CPU profile: ", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	if chainConfig != nil {
		runtimeConfig.ChainConfig = chainConfig
	} else {
		runtimeConfig.ChainConfig = config.DefaultChainConfig
	}

	var hexInput []byte
	if inputFileFlag := ctx.GlobalString(InputFileFlag.Name); inputFileFlag != "" {
		var err error
		if hexInput, err = ioutil.ReadFile(inputFileFlag); err != nil {
			fmt.Printf("could not load input from file: %v\n", err)
			os.Exit(1)
		}
	} else {
		hexInput = []byte(ctx.GlobalString(InputFlag.Name))
	}
	input := common.FromHex(string(bytes.TrimSpace(hexInput)))

	var execFunc func() ([]byte, uint64, error)
	if ctx.GlobalBool(CreateFlag.Name) {
		input = append(code, input...)
		execFunc = func() ([]byte, uint64, error) {
			output, _, gasLeft, err := runtime.Create(input, &runtimeConfig)
			return output, gasLeft, err
		}
	} else {
		if len(code) > 0 {
			statedb.SetCode(receiver, code)
		}
		execFunc = func() ([]byte, uint64, error) {
			return runtime.Call(receiver, input, &runtimeConfig)
		}
	}

	output, leftOverGas, execTime, err := timedExec(ctx.GlobalBool(BenchFlag.Name), execFunc)

	if ctx.GlobalBool(DumpFlag.Name) {
		statedb.Commit(true)
		statedb.IntermediateRoot(true)
		fmt.Println(string(statedb.Dump(false, false, true)))
	}

	if memProfilePath := ctx.GlobalString(MemProfileFlag.Name); memProfilePath != "" {
		f, err := os.Create(memProfilePath)
		if err != nil {
			fmt.Println("could not create memory profile: ", err)
			os.Exit(1)
		}
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Println("could not write memory profile: ", err)
			os.Exit(1)
		}
		f.Close()
	}

	if ctx.GlobalBool(DebugFlag.Name) {
		if debugLogger != nil {
			fmt.Fprintln(os.Stderr, "#### TRACE ####")
			evm.WriteTrace(os.Stderr, debugLogger.StructLogs())
		}
		fmt.Fprintln(os.Stderr, "#### LOGS ####")
		evm.WriteLogs(os.Stderr, statedb.Logs())
	}

	if ctx.GlobalBool(StatDumpFlag.Name) {
		var mem goruntime.MemStats
		goruntime.ReadMemStats(&mem)
		fmt.Fprintf(os.Stderr, `evm execution time: %v
heap objects:       %d
allocations:        %d
total allocations:  %d
GC calls:           %d
Gas used:           %d

`, execTime, mem.HeapObjects, mem.Alloc, mem.TotalAlloc, mem.NumGC, initialGas-leftOverGas)
	}
	if tracer == nil {
		fmt.Printf("0x%x\n", output)
		if err != nil {
			fmt.Printf(" error: %v\n", err)
		}
	}

	return nil
}
