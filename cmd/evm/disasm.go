package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"gopkg.in/urfave/cli.v1"
	"github.com/entropyio/go-entropy/evm/asm"
)

var disasmCommand = cli.Command{
	Action:    disasmCmd,
	Name:      "disasm",
	Usage:     "disassembles evm binary",
	ArgsUsage: "<file>",
}

func disasmCmd(ctx *cli.Context) error {
	if len(ctx.Args().First()) == 0 {
		return errors.New("filename required")
	}

	fn := ctx.Args().First()
	in, err := ioutil.ReadFile(fn)
	if err != nil {
		return err
	}

	code := strings.TrimSpace(string(in[:]))
	fmt.Printf("%v\n", code)
	return asm.PrintDisassembled(code)
}
