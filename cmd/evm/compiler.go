package main

import (
	"errors"
	"fmt"
	"io/ioutil"

	"gopkg.in/urfave/cli.v1"
	"github.com/entropyio/go-entropy/cmd/evm/internal/compiler"
)

var compileCommand = cli.Command{
	Action:    compileCmd,
	Name:      "compile",
	Usage:     "compiles easm source to evm binary",
	ArgsUsage: "<file>",
}

func compileCmd(ctx *cli.Context) error {
	debug := ctx.GlobalBool(DebugFlag.Name)

	if len(ctx.Args().First()) == 0 {
		return errors.New("filename required")
	}

	fn := ctx.Args().First()
	src, err := ioutil.ReadFile(fn)
	if err != nil {
		return err
	}

	bin, err := compiler.Compile(fn, src, debug)
	if err != nil {
		return err
	}
	fmt.Println(bin)
	return nil
}
