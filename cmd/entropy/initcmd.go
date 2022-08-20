package main

import (
	"encoding/json"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/urfave/cli/v2"
	"os"
)

var (
	initCommand = &cli.Command{
		Action:    initGenesis,
		Name:      "init",
		Usage:     "Bootstrap and initialize a new genesis block",
		ArgsUsage: "<genesisPath>",
		Flags:     utils.DatabasePathFlags,
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
	log.Debug("init genesisPath: " + genesisPath)

	if len(genesisPath) == 0 {
		utils.Fatalf("Must supply path to genesis JSON file")
	}
	file, err := os.Open(genesisPath)
	if err != nil {
		utils.Fatalf("Failed to read genesis file: %v", err)
	}
	defer file.Close()

	genesis := new(blockchain.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		utils.Fatalf("invalid genesis file: %v", err)
	}

	// Open an initialise both full and light databases
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()
	for _, name := range []string{"chaindata", "lightchaindata"} {
		chaindb, err := stack.OpenDatabaseWithFreezer(name, 0, 0, ctx.String(utils.AncientFlag.Name), "", false)
		if err != nil {
			utils.Fatalf("Failed to open database: %v", err)
		}
		_, hash, err := blockchain.SetupGenesisBlock(chaindb, genesis)
		if err != nil {
			utils.Fatalf("Failed to write genesis block: %v", err)
		}
		_ = chaindb.Close()
		log.Info("Successfully wrote genesis state", "database", name, "hash", hash)
	}

	return nil
}
