package subcmd

import (
	"github.com/entropyio/go-entropy/cmd/console"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/entropyio/go-entropy/logger"
	"gopkg.in/urfave/cli.v1"
)

var consoleLogger = logger.NewLogger("[consoleCmd]")

var (
	consoleFlags = []cli.Flag{utils.JSpathFlag, utils.ExecFlag, utils.PreloadJSFlag}

	ConsoleCommand = cli.Command{
		Action:   utils.MigrateFlags(localConsole),
		Name:     "console",
		Usage:    "Start an interactive JavaScript environment",
		Flags:    append(append(utils.NodeFlags, utils.RpcFlags...), consoleFlags...),
		Category: "CONSOLE COMMANDS",
		Description: `
The Entropy console is an interactive shell for the JavaScript runtime environment
which exposes a node admin interface as well as the DAPP JavaScript API.`,
	}
)

// localConsole starts a new entropy node, attaching a JavaScript console to it at the
// same time.
func localConsole(ctx *cli.Context) error {
	consoleLogger.Info("start localConsole ...")
	// Create and start the node based on the CLI flags
	node := utils.MakeFullNode(ctx)
	StartEntropyNode(ctx, node)
	defer node.Stop()

	// Attach to the newly started node and start the JavaScript console
	client, err := node.Attach()
	if err != nil {
		utils.Fatalf("Failed to attach to the inproc entropy: %v", err)
	}
	config := console.Config{
		DataDir: utils.MakeDataDir(ctx),
		DocRoot: ctx.GlobalString(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(ctx),
	}

	console, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	// If only a short execution was requested, evaluate and return
	if script := ctx.GlobalString(utils.ExecFlag.Name); script != "" {
		console.Evaluate(script)
		return nil
	}
	// Otherwise print the welcome screen and enter interactive mode
	console.Welcome()
	console.Interactive()

	return nil
}
