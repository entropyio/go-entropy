package subcmd

import (
	"fmt"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/entropyio/go-entropy/logger"
	"gopkg.in/urfave/cli.v1"
	"os"
	"runtime"
	"strings"
)

var versionLogger = logger.NewLogger("[versionCmd]")

var (
	VersionCommand = cli.Command{
		Action:    utils.MigrateFlags(version),
		Name:      "version",
		Usage:     "Print version numbers",
		ArgsUsage: " ",
		Category:  "MISCELLANEOUS COMMANDS",
		Description: `
The output of this command is supposed to be machine-readable.
`,
	}
)

func version(ctx *cli.Context) error {
	versionLogger.Debug(strings.Title(utils.ClientIdentifier))
	fmt.Println("Version:", "1.1.0")
	if utils.GitCommit != "" {
		fmt.Println("Git Commit:", utils.GitCommit)
	}
	fmt.Println("Architecture:", runtime.GOARCH)
	fmt.Println("Protocol Versions:", "1.1.0")
	fmt.Println("Network Id:", "ID12345")
	fmt.Println("Go Version:", runtime.Version())
	fmt.Println("Operating System:", runtime.GOOS)
	fmt.Printf("GOPATH=%s\n", os.Getenv("GOPATH"))
	fmt.Printf("GOROOT=%s\n", runtime.GOROOT())
	return nil
}
