package subcmd

import (
	"encoding/json"
	"github.com/entropyio/go-entropy/blockchain/genesis"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/entropyio/go-entropy/logger"
	"gopkg.in/urfave/cli.v1"
	"os"
)

var initLogger = logger.NewLogger("[initCmd]")

var (
	InitCommand = cli.Command{
		Action:    utils.MigrateFlags(initGenesis),
		Name:      "init",
		Usage:     "Bootstrap and initialize a new genesis block",
		ArgsUsage: "<genesisPath>",
		Flags: []cli.Flag{
			utils.DataDirFlag,
		},
		Category: "BLOCKCHAIN COMMANDS",
		Description: `
The init command initializes a new genesis block and definition for the network.
This is a destructive action and changes the network in which you will be
participating.

It expects the genesis file as argument.`,
	}
)

func initGenesis(ctx *cli.Context) error {
	// Make sure we have a valid genesis JSON
	genesisPath := ctx.Args().First()
	initLogger.Debug("init genesisPath: " + genesisPath)

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

	// Open an initialise both full and light databases
	stack := utils.MakeFullNode(ctx)
	defer stack.Close()
	for _, name := range []string{"chaindata", "lightchaindata"} {
		chaindb, err := stack.OpenDatabase(name, 0, 0, "")

		if err != nil {
			utils.Fatalf("Failed to open database: %v", err)
		}
		_, hash, err := genesis.SetupGenesisBlock(chaindb, genesisObj)
		if err != nil {
			utils.Fatalf("Failed to write genesis block: %v", err)
		}
		chaindb.Close()
		initLogger.Info("Successfully wrote genesis state", "database", name, "hash", hash)
	}

	return nil
}
