package utils

import (
	"fmt"
	"github.com/entropyio/go-entropy/cmd/flags"
	"github.com/entropyio/go-entropy/entropy/entropyconfig"
	"github.com/urfave/cli/v2"
)

var ShowDeprecated = &cli.Command{
	Action:      showDeprecated,
	Name:        "show-deprecated-flags",
	Usage:       "Show flags that have been deprecated",
	ArgsUsage:   " ",
	Description: "Show flags that have been deprecated and will soon be removed",
}

var DeprecatedFlags = []cli.Flag{
	LegacyMinerGasTargetFlag,
	NoUSBFlag,
}

var (
	// (Deprecated May 2020, shown in aliased flags section)
	NoUSBFlag = &cli.BoolFlag{
		Name:     "nousb",
		Usage:    "Disables monitoring for and managing USB hardware wallets (deprecated)",
		Category: flags.DeprecatedCategory,
	}
	// (Deprecated July 2021, shown in aliased flags section)
	LegacyMinerGasTargetFlag = &cli.Uint64Flag{
		Name:     "miner.gastarget",
		Usage:    "Target gas floor for mined blocks (deprecated)",
		Value:    entropyconfig.Defaults.Miner.GasFloor,
		Category: flags.DeprecatedCategory,
	}
)

// showDeprecated displays deprecated flags that will be soon removed from the codebase.
func showDeprecated(*cli.Context) error {
	fmt.Println("--------------------------------------------------------------------")
	fmt.Println("The following flags are deprecated and will be removed in the future!")
	fmt.Println("--------------------------------------------------------------------")
	fmt.Println()
	for _, flag := range DeprecatedFlags {
		fmt.Println(flag.String())
	}
	fmt.Println()
	return nil
}
