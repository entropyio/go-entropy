package main

import (
	"fmt"
	"github.com/entropyio/go-entropy/cmd/entropy/subcmd"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/server/node"
	"gopkg.in/urfave/cli.v1"
	"os"
	"runtime"
	"sort"
)

var log = logger.NewLogger("[entropy]")

var (
	app = utils.NewApp(utils.GitCommit, "the go-entropy command line interface")
)

func init() {
	log.Debug("do entropy init")
	app.Action = entropyAction
	app.HideVersion = true // we have a command to print the version
	app.Copyright = "Copyright 2018 The Entropy Authors"
	app.Commands = []cli.Command{
		// entropy init
		subcmd.InitCommand,

		// entropy version
		subcmd.VersionCommand,

		// entropy console
		subcmd.ConsoleCommand,
	}

	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = append(append(app.Flags, utils.NodeFlags...), utils.RpcFlags...)

	app.Before = func(context *cli.Context) error {
		log.Debug("call app.Before....")
		runtime.GOMAXPROCS(runtime.NumCPU())

		logDir := ""
		if context.GlobalBool(utils.DashboardEnabledFlag.Name) {
			logDir = (&node.Config{DataDir: utils.MakeDataDir(context)}).ResolvePath("logs")
		}
		log.Debugf("set dashboard dir: %s", logDir)

		return nil
	}

	app.After = func(context *cli.Context) error {
		log.Debug("call app.After....")

		return nil
	}

}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func entropyAction(ctx *cli.Context) error {
	log.Debug("run entropy app action ...")

	nodeObj := utils.MakeFullNode(ctx)
	subcmd.StartEntropyNode(ctx, nodeObj)
	nodeObj.Wait()

	log.Debug("entropy app action done.")
	return nil
}
