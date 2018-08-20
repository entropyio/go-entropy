package utils

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/dashboard"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/server/node"
	"github.com/naoina/toml"
	"gopkg.in/urfave/cli.v1"
	"os"
	"reflect"
	"unicode"
)

const (
	ClientIdentifier = "entropy"
)

var GitCommit = "gitCommit-12345678"

var (
	NodeFlags = []cli.Flag{
		// data dir
		DataDirFlag,
		LightModeFlag,
		GCModeFlag,

		// dashboard
		DashboardEnabledFlag,
		DashboardAddrFlag,
		DashboardPortFlag,
		DashboardRefreshFlag,
	}

	RpcFlags = []cli.Flag{
		RPCEnabledFlag,
		RPCListenAddrFlag,
		RPCPortFlag,
		RPCApiFlag,
		WSEnabledFlag,
		WSListenAddrFlag,
		WSPortFlag,
		WSApiFlag,
		WSAllowedOriginsFlag,
		IPCDisabledFlag,
		IPCPathFlag,
	}
)

type entropyStatsConfig struct {
	URL string `toml:",omitempty"`
}

type EntropyConfig struct {
	Entropy entropy.Config
	//Shh       whisper.Config
	Node         node.Config
	EntropyStats entropyStatsConfig
	Dashboard    dashboard.Config
}

var (
	ConfigFileFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}
)

// These settings ensure that TOML keys use the same names as Go struct fields.
var tomlSettings = toml.Config{
	NormFieldName: func(rt reflect.Type, key string) string {
		return key
	},
	FieldToKey: func(rt reflect.Type, field string) string {
		return field
	},
	MissingField: func(rt reflect.Type, field string) error {
		link := ""
		if unicode.IsUpper(rune(rt.Name()[0])) && rt.PkgPath() != "main" {
			link = fmt.Sprintf(", see https://godoc.org/%s#%s for available fields", rt.PkgPath(), rt.Name())
		}
		return fmt.Errorf("field '%s' is not defined in %s%s", field, rt.String(), link)
	},
}

func LoadConfig(file string, cfg *EntropyConfig) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tomlSettings.NewDecoder(bufio.NewReader(f)).Decode(cfg)
	// Add file name to errors that have a line number.
	if _, ok := err.(*toml.LineError); ok {
		err = errors.New(file + ", " + err.Error())
	}
	return err
}

func DefaultNodeConfig() node.Config {
	cfg := node.DefaultConfig
	cfg.Name = ClientIdentifier
	cfg.Version = config.VersionWithCommit(GitCommit)
	cfg.HTTPModules = append(cfg.HTTPModules, "entropy", "shh")
	cfg.WSModules = append(cfg.WSModules, "entropy", "shh")
	cfg.IPCPath = "entropy.ipc"
	return cfg
}
