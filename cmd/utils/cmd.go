package utils

import (
	"fmt"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/server/node"
	"gopkg.in/urfave/cli.v1"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func MakeFullNode(ctx *cli.Context) *node.Node {
	stack, cfg := MakeConfigNode(ctx)

	// add entropy service
	RegisterEntropyService(stack, &cfg.Entropy)

	// Whisper must be explicitly enabled by specifying at least 1 whisper flag or in dev mode
	//shhEnabled := enableWhisper(ctx)
	//shhAutoEnabled := !ctx.GlobalIsSet(utils.WhisperEnabledFlag.Name) && ctx.GlobalIsSet(utils.DeveloperFlag.Name)
	//if shhEnabled || shhAutoEnabled {
	//	if ctx.GlobalIsSet(utils.WhisperMaxMessageSizeFlag.Name) {
	//		cfg.Shh.MaxMessageSize = uint32(ctx.Int(utils.WhisperMaxMessageSizeFlag.Name))
	//	}
	//	if ctx.GlobalIsSet(utils.WhisperMinPOWFlag.Name) {
	//		cfg.Shh.MinimumAcceptedPOW = ctx.Float64(utils.WhisperMinPOWFlag.Name)
	//	}
	//	if ctx.GlobalIsSet(utils.WhisperRestrictConnectionBetweenLightClientsFlag.Name) {
	//		cfg.Shh.RestrictConnectionBetweenLightClients = true
	//	}
	//	utils.RegisterShhService(stack, &cfg.Shh)
	//}

	// Configure GraphQL if required
	//if ctx.GlobalIsSet(GraphQLEnabledFlag.Name) {
	//	if err := graphql.RegisterGraphQLService(stack, cfg.Node.GraphQLEndpoint(), cfg.Node.GraphQLCors, cfg.Node.GraphQLVirtualHosts, cfg.Node.HTTPTimeouts); err != nil {
	//		Fatalf("Failed to register the Ethereum service: %v", err)
	//	}
	//}

	// Add statistic service
	if cfg.EntropyStats.URL != "" {
		RegisterEntropyStatsService(stack, cfg.EntropyStats.URL)
	}
	return stack
}

func MakeConfigNode(ctx *cli.Context) (*node.Node, EntropyConfig) {
	// Load defaults.
	cfg := EntropyConfig{
		Entropy: entropy.DefaultConfig,
		//Shh:       whisper.DefaultConfig,
		Node: DefaultNodeConfig(),
	}

	// Load config file.
	if file := ctx.GlobalString(ConfigFileFlag.Name); file != "" {
		if err := LoadConfig(file, &cfg); err != nil {
			Fatalf("%v", err)
		}
	}

	// Apply flags.
	SetNodeConfig(ctx, &cfg.Node)
	stack, err := node.New(&cfg.Node)
	if err != nil {
		Fatalf("Failed to create the protocol stack: %v", err)
	}
	SetEntropyConfig(ctx, stack, &cfg.Entropy)
	if ctx.GlobalIsSet(EntropyStatsURLFlag.Name) {
		cfg.EntropyStats.URL = ctx.GlobalString(EntropyStatsURLFlag.Name)
	}

	//utils.SetShhConfig(ctx, stack, &cfg.Shh)

	return stack, cfg
}

func StartNode(stack *node.Node) {
	if err := stack.Start(); err != nil {
		Fatalf("Error starting protocol stack: %v", err)
	}
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		log.Info("Got interrupt, shutting down...")
		go stack.Stop()
		for i := 10; i > 0; i-- {
			<-sigc
			if i > 1 {
				log.Warning("Already shutting down, interrupt more to panic.", "times", i-1)
			}
		}
		//debug.Exit() // ensure trace and CPU profile data is flushed.
		//debug.LoudPanic("boom")
	}()
}

// Fatalf formats a message to standard error and exits the program.
// The message is also printed to standard output if standard error
// is redirected to a different file.
func Fatalf(format string, args ...interface{}) {
	w := io.MultiWriter(os.Stdout, os.Stderr)
	if runtime.GOOS == "windows" {
		// The SameFile check below doesn't work on Windows.
		// stdout is unlikely to get redirected though, so just print there.
		w = os.Stdout
	} else {
		outf, _ := os.Stdout.Stat()
		errf, _ := os.Stderr.Stat()
		if outf != nil && errf != nil && os.SameFile(outf, errf) {
			w = os.Stderr
		}
	}
	fmt.Fprintf(w, "Fatal: "+format+"\n", args...)
	os.Exit(1)
}
