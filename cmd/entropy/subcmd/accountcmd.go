package subcmd

import (
	"fmt"
	"io/ioutil"

	"github.com/entropyio/go-entropy/account"
	"github.com/entropyio/go-entropy/account/keystore"
	"github.com/entropyio/go-entropy/cmd/console"
	"github.com/entropyio/go-entropy/cmd/utils"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/logger"
	"gopkg.in/urfave/cli.v1"
)

var accountLogger = logger.NewLogger("[accountCmd]")

var (
	walletCommand = cli.Command{
		Name:      "wallet",
		Usage:     "Manage Entropy presale wallets",
		ArgsUsage: "",
		Category:  "ACCOUNT COMMANDS",
		Description: `
    entropy wallet import /path/to/my/presale.wallet

will prompt for your password and imports your ether presale account.
It can be used non-interactively with the --password option taking a
passwordfile as argument containing the wallet password in plaintext.`,
		Subcommands: []cli.Command{
			{

				Name:      "import",
				Usage:     "Import Entropy presale wallet",
				ArgsUsage: "<keyFile>",
				Action:    utils.MigrateFlags(importWallet),
				Category:  "ACCOUNT COMMANDS",
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.PasswordFileFlag,
					utils.LightKDFFlag,
				},
				Description: `
	entropy wallet [options] /path/to/my/presale.wallet

will prompt for your password and imports your ether presale account.
It can be used non-interactively with the --password option taking a
passwordfile as argument containing the wallet password in plaintext.`,
			},
		},
	}

	accountCommand = cli.Command{
		Name:     "account",
		Usage:    "Manage account",
		Category: "ACCOUNT COMMANDS",
		Description: `

Manage account, list all existing account, import a private key into a new
account, create a new account or update an existing account.

It supports interactive mode, when you are prompted for password as well as
non-interactive mode where passwords are supplied via a given password file.
Non-interactive mode is only meant for scripted use on test networks or known
safe environments.

Make sure you remember the password you gave when creating a new account (with
either new or import). Without it you are not able to unlock your account.

Note that exporting your key in unencrypted format is NOT supported.

Keys are stored under <DATADIR>/keystore.
It is safe to transfer the entire directory or the individual keys therein
between entropy nodes by simply copying.

Make sure you backup your keys regularly.`,
		Subcommands: []cli.Command{
			{
				Name:   "list",
				Usage:  "Print summary of existing account",
				Action: utils.MigrateFlags(accountList),
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
				},
				Description: `
Print a short summary of all account`,
			},
			{
				Name:   "new",
				Usage:  "Create a new account",
				Action: utils.MigrateFlags(accountCreate),
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.PasswordFileFlag,
					utils.LightKDFFlag,
				},
				Description: `
    entropy account new

Creates a new account and prints the address.

The account is saved in encrypted format, you are prompted for a passphrase.

You must remember this passphrase to unlock your account in the future.

For non-interactive use the passphrase can be specified with the --password flag:

Note, this is meant to be used for testing only, it is a bad idea to save your
password to file or expose in any other way.
`,
			},
			{
				Name:      "update",
				Usage:     "Update an existing account",
				Action:    utils.MigrateFlags(accountUpdate),
				ArgsUsage: "<address>",
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.LightKDFFlag,
				},
				Description: `
    entropy account update <address>

Update an existing account.

The account is saved in the newest version in encrypted format, you are prompted
for a passphrase to unlock the account and another to save the updated file.

This same command can therefore be used to migrate an account of a deprecated
format to the newest format or change the password for an account.

For non-interactive use the passphrase can be specified with the --password flag:

    entropy account update [options] <address>

Since only one password can be given, only format update can be performed,
changing your password is only possible interactively.
`,
			},
			{
				Name:   "import",
				Usage:  "Import a private key into a new account",
				Action: utils.MigrateFlags(accountImport),
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.PasswordFileFlag,
					utils.LightKDFFlag,
				},
				ArgsUsage: "<keyFile>",
				Description: `
    entropy account import <keyfile>

Imports an unencrypted private key from <keyfile> and creates a new account.
Prints the address.

The keyfile is assumed to contain an unencrypted private key in hexadecimal format.

The account is saved in encrypted format, you are prompted for a passphrase.

You must remember this passphrase to unlock your account in the future.

For non-interactive use the passphrase can be specified with the -password flag:

    entropy account import [options] <keyfile>

Note:
As you can directly copy your encrypted account to another entropy instance,
this import mechanism is not needed when you transfer an account between
nodes.
`,
			},
		},
	}
)

func accountList(ctx *cli.Context) error {
	stack, _ := utils.MakeConfigNode(ctx)
	var index int
	for _, wallet := range stack.AccountManager().Wallets() {
		for _, accountObj := range wallet.Accounts() {
			fmt.Printf("Account #%d: {%x} %s\n", index, accountObj.Address, &accountObj.URL)
			index++
		}
	}
	return nil
}

// tries unlocking the specified account a few times.
func UnlockAccount(ctx *cli.Context, ks *keystore.KeyStore, address string, i int, passwords []string) (account.Account, string) {
	accountObj, err := utils.MakeAddress(ks, address)
	if err != nil {
		utils.Fatalf("Could not list account: %v", err)
	}
	for trials := 0; trials < 3; trials++ {
		prompt := fmt.Sprintf("Unlocking account %s | Attempt %d/%d", address, trials+1, 3)
		password := getPassPhrase(prompt, false, i, passwords)
		err = ks.Unlock(accountObj, password)
		if err == nil {
			accountLogger.Info("Unlocked account", "address", accountObj.Address.Hex())
			return accountObj, password
		}
		if err, ok := err.(*keystore.AmbiguousAddrError); ok {
			accountLogger.Info("Unlocked account", "address", accountObj.Address.Hex())
			return ambiguousAddrRecovery(ks, err, password), password
		}
		if err != keystore.ErrDecrypt {
			// No need to prompt again if the error is not decryption-related.
			break
		}
	}
	// All trials expended to unlock account, bail out
	utils.Fatalf("Failed to unlock account %s (%v)", address, err)

	return account.Account{}, ""
}

// getPassPhrase retrieves the password associated with an account, either fetched
// from a list of preloaded passphrases, or requested interactively from the user.
func getPassPhrase(prompt string, confirmation bool, i int, passwords []string) string {
	// If a list of passwords was supplied, retrieve from them
	if len(passwords) > 0 {
		if i < len(passwords) {
			return passwords[i]
		}
		return passwords[len(passwords)-1]
	}
	// Otherwise prompt the user for the password
	if prompt != "" {
		fmt.Println(prompt)
	}
	password, err := console.Stdin.PromptPassword("Passphrase: ")
	if err != nil {
		utils.Fatalf("Failed to read passphrase: %v", err)
	}
	if confirmation {
		confirm, err := console.Stdin.PromptPassword("Repeat passphrase: ")
		if err != nil {
			utils.Fatalf("Failed to read passphrase confirmation: %v", err)
		}
		if password != confirm {
			utils.Fatalf("Passphrases do not match")
		}
	}
	return password
}

func ambiguousAddrRecovery(ks *keystore.KeyStore, err *keystore.AmbiguousAddrError, auth string) account.Account {
	fmt.Printf("Multiple key files exist for address %x:\n", err.Addr)
	for _, a := range err.Matches {
		fmt.Println("  ", a.URL)
	}
	fmt.Println("Testing your passphrase against all of them...")
	var match *account.Account
	for _, a := range err.Matches {
		if err := ks.Unlock(a, auth); err == nil {
			match = &a
			break
		}
	}
	if match == nil {
		utils.Fatalf("None of the listed files could be unlocked.")
	}
	fmt.Printf("Your passphrase unlocked %s\n", match.URL)
	fmt.Println("In order to avoid this warning, you need to remove the following duplicate key files:")
	for _, a := range err.Matches {
		if a != *match {
			fmt.Println("  ", a.URL)
		}
	}
	return *match
}

// accountCreate creates a new account into the keystore defined by the CLI flags.
func accountCreate(ctx *cli.Context) error {
	cfg := utils.EntropyConfig{Node: utils.DefaultNodeConfig()}
	// Load config file.
	if file := ctx.GlobalString(utils.ConfigFileFlag.Name); file != "" {
		if err := utils.LoadConfig(file, &cfg); err != nil {
			utils.Fatalf("%v", err)
		}
	}
	utils.SetNodeConfig(ctx, &cfg.Node)
	scryptN, scryptP, keydir, err := cfg.Node.AccountConfig()

	if err != nil {
		utils.Fatalf("Failed to read configuration: %v", err)
	}

	password := getPassPhrase("Your new account is locked with a password. Please give a password. Do not forget this password.", true, 0, utils.MakePasswordList(ctx))

	address, err := keystore.StoreKey(keydir, password, scryptN, scryptP)

	if err != nil {
		utils.Fatalf("Failed to create account: %v", err)
	}
	fmt.Printf("Address: {%x}\n", address)
	return nil
}

// accountUpdate transitions an account from a previous format to the current
// one, also providing the possibility to change the pass-phrase.
func accountUpdate(ctx *cli.Context) error {
	if len(ctx.Args()) == 0 {
		utils.Fatalf("No account specified to update")
	}
	stack, _ := utils.MakeConfigNode(ctx)
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)

	for _, addr := range ctx.Args() {
		accountObj, oldPassword := UnlockAccount(ctx, ks, addr, 0, nil)
		newPassword := getPassPhrase("Please give a new password. Do not forget this password.", true, 0, nil)
		if err := ks.Update(accountObj, oldPassword, newPassword); err != nil {
			utils.Fatalf("Could not update the account: %v", err)
		}
	}
	return nil
}

func importWallet(ctx *cli.Context) error {
	keyFile := ctx.Args().First()
	if len(keyFile) == 0 {
		utils.Fatalf("keyFile must be given as argument")
	}
	keyJSON, err := ioutil.ReadFile(keyFile)
	if err != nil {
		utils.Fatalf("Could not read wallet file: %v", err)
	}

	stack, _ := utils.MakeConfigNode(ctx)
	passPhrase := getPassPhrase("", false, 0, utils.MakePasswordList(ctx))

	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	acct, err := ks.ImportPreSaleKey(keyJSON, passPhrase)
	if err != nil {
		utils.Fatalf("%v", err)
	}
	fmt.Printf("Address: {%x}\n", acct.Address)
	return nil
}

func accountImport(ctx *cli.Context) error {
	keyfile := ctx.Args().First()
	if len(keyfile) == 0 {
		utils.Fatalf("keyfile must be given as argument")
	}
	key, err := crypto.LoadECDSA(keyfile)
	if err != nil {
		utils.Fatalf("Failed to load the private key: %v", err)
	}
	stack, _ := utils.MakeConfigNode(ctx)
	passphrase := getPassPhrase("Your new account is locked with a password. Please give a password. Do not forget this password.", true, 0, utils.MakePasswordList(ctx))

	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	acct, err := ks.ImportECDSA(key, passphrase)
	if err != nil {
		utils.Fatalf("Could not create the account: %v", err)
	}
	fmt.Printf("Address: {%x}\n", acct.Address)
	return nil
}
