package main

import (
	"fmt"
	"github.com/entropyio/go-entropy/config"
	"github.com/urfave/cli/v2"
	"os"
	"runtime"
	"strings"
)

var (
	versionCommand = &cli.Command{
		Action:    version,
		Name:      "version",
		Usage:     "Print version numbers",
		ArgsUsage: " ",
		Description: `
The output of this command is supposed to be machine-readable.
`,
	}
)

func version(*cli.Context) error {
	fmt.Println(strings.Title(clientIdentifier))
	fmt.Println("Version:", config.VersionWithMeta)
	if gitCommit != "" {
		fmt.Println("Git Commit:", gitCommit)
	}
	if gitDate != "" {
		fmt.Println("Git Commit Date:", gitDate)
	}
	fmt.Println("Architecture:", runtime.GOARCH)
	fmt.Println("Go Version:", runtime.Version())
	fmt.Println("Operating System:", runtime.GOOS)
	fmt.Printf("GOPATH=%s\n", os.Getenv("GOPATH"))
	fmt.Printf("GOROOT=%s\n", runtime.GOROOT())
	return nil
}
