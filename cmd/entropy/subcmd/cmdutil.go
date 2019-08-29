package subcmd

import (
	"github.com/entropyio/go-entropy/account"
	"github.com/entropyio/go-entropy/account/keystore"
	"github.com/entropyio/go-entropy/client"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/server/node"
	"gopkg.in/urfave/cli.v1"
	"strings"
)

var log = logger.NewLogger("[CmdUtil]")

// StartEntropyNode boots up the system node and all registered protocols, after which
// it unlocks any requested accounts, and starts the RPC/IPC interfaces and the
// miner.
func StartEntropyNode(ctx *cli.Context, stack *node.Node) {
	//debug.Memsize.Add("node", stack)

	// Start up the node itself
	utils.StartNode(stack)

	// Unlock any account specifically requested
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)

	passwords := utils.MakePasswordList(ctx)
	unlocks := strings.Split(ctx.GlobalString(utils.UnlockedAccountFlag.Name), ",")
	for i, accountObj := range unlocks {
		if trimmed := strings.TrimSpace(accountObj); trimmed != "" {
			UnlockAccount(ctx, ks, trimmed, i, passwords)
		}
	}
	// Register wallet event handlers to open and auto-derive wallets
	events := make(chan account.WalletEvent, 16)
	stack.AccountManager().Subscribe(events)

	go func() {
		// Create a chain state reader for self-derivation
		rpcClient, err := stack.Attach()
		if err != nil {
			utils.Fatalf("Failed to attach to self: %v", err)
		}
		stateReader := client.NewClient(rpcClient)

		// Open any wallets already attached
		for _, wallet := range stack.AccountManager().Wallets() {
			if err := wallet.Open(""); err != nil {
				log.Warning("Failed to open wallet", "url", wallet.URL(), "err", err)
			}
		}
		// Listen for wallet event till termination
		for event := range events {
			switch event.Kind {
			case account.WalletArrived:
				if err := event.Wallet.Open(""); err != nil {
					log.Warning("New wallet appeared, failed to open", "url", event.Wallet.URL(), "err", err)
				}
			case account.WalletOpened:
				status, _ := event.Wallet.Status()
				log.Info("New wallet appeared", "url", event.Wallet.URL(), "status", status)

				var derivationPaths []account.DerivationPath
				if event.Wallet.URL().Scheme == "ledger" {
					derivationPaths = append(derivationPaths, account.LegacyLedgerBaseDerivationPath)
				}
				derivationPaths = append(derivationPaths, account.DefaultBaseDerivationPath)

				event.Wallet.SelfDerive(derivationPaths, stateReader)

			case account.WalletDropped:
				log.Info("Old wallet dropped", "url", event.Wallet.URL())
				event.Wallet.Close()
			}
		}
	}()
	// Start auxiliary services if enabled
	if ctx.GlobalBool(utils.MiningEnabledFlag.Name) || ctx.GlobalBool(utils.DeveloperFlag.Name) {
		// Mining only makes sense if a full Ethereum node is running
		if ctx.GlobalString(utils.SyncModeFlag.Name) == "light" {
			utils.Fatalf("Light clients do not support mining")
		}
		var entropyObj *entropy.Entropy
		if err := stack.Service(&entropyObj); err != nil {
			utils.Fatalf("Entropy service not running: %v", err)
		}
		// Set the gas price to the limits from the CLI and start mining
		gasprice := utils.GlobalBig(ctx, utils.MinerLegacyGasPriceFlag.Name)
		if ctx.IsSet(utils.MinerGasPriceFlag.Name) {
			gasprice = utils.GlobalBig(ctx, utils.MinerGasPriceFlag.Name)
		}
		entropyObj.TxPool().SetGasPrice(gasprice)

		threads := ctx.GlobalInt(utils.MinerLegacyThreadsFlag.Name)
		if ctx.GlobalIsSet(utils.MinerThreadsFlag.Name) {
			threads = ctx.GlobalInt(utils.MinerThreadsFlag.Name)
		}
		if err := entropyObj.StartMining(threads); err != nil {
			utils.Fatalf("Failed to start mining: %v", err)
		}
	}
}
