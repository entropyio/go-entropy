package keystore

import (
	"math/big"

	"github.com/entropyio/go-entropy"
	"github.com/entropyio/go-entropy/account"
	"github.com/entropyio/go-entropy/blockchain/model"
)

// keystoreWallet implements the account.Wallet interface for the original
// keystore.
type keystoreWallet struct {
	account  account.Account // Single account contained in this wallet
	keystore *KeyStore       // Keystore where the account originates from
}

// URL implements account.Wallet, returning the URL of the account within.
func (w *keystoreWallet) URL() account.URL {
	return w.account.URL
}

// Status implements account.Wallet, returning whether the account held by the
// keystore wallet is unlocked or not.
func (w *keystoreWallet) Status() (string, error) {
	w.keystore.mu.RLock()
	defer w.keystore.mu.RUnlock()

	if _, ok := w.keystore.unlocked[w.account.Address]; ok {
		return "Unlocked", nil
	}
	return "Locked", nil
}

// Open implements account.Wallet, but is a noop for plain wallets since there
// is no connection or decryption step necessary to access the list of account.
func (w *keystoreWallet) Open(passphrase string) error { return nil }

// Close implements account.Wallet, but is a noop for plain wallets since is no
// meaningful open operation.
func (w *keystoreWallet) Close() error { return nil }

// Accounts implements account.Wallet, returning an account list consisting of
// a single account that the plain kestore wallet contains.
func (w *keystoreWallet) Accounts() []account.Account {
	return []account.Account{w.account}
}

// Contains implements account.Wallet, returning whether a particular account is
// or is not wrapped by this wallet instance.
func (w *keystoreWallet) Contains(accountObj account.Account) bool {
	return accountObj.Address == w.account.Address && (accountObj.URL == (account.URL{}) || accountObj.URL == w.account.URL)
}

// Derive implements account.Wallet, but is a noop for plain wallets since there
// is no notion of hierarchical account derivation for plain keystore account.
func (w *keystoreWallet) Derive(path account.DerivationPath, pin bool) (account.Account, error) {
	return account.Account{}, account.ErrNotSupported
}

// SelfDerive implements account.Wallet, but is a noop for plain wallets since
// there is no notion of hierarchical account derivation for plain keystore account.
func (w *keystoreWallet) SelfDerive(base account.DerivationPath, chain entropy.ChainStateReader) {}

// SignHash implements account.Wallet, attempting to sign the given hash with
// the given account. If the wallet does not wrap this particular account, an
// error is returned to avoid account leakage (even though in theory we may be
// able to sign via our shared keystore backend).
func (w *keystoreWallet) SignHash(accountObj account.Account, hash []byte) ([]byte, error) {
	// Make sure the requested account is contained within
	if accountObj.Address != w.account.Address {
		return nil, account.ErrUnknownAccount
	}
	if accountObj.URL != (account.URL{}) && accountObj.URL != w.account.URL {
		return nil, account.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignHash(accountObj, hash)
}

// SignTx implements account.Wallet, attempting to sign the given transaction
// with the given account. If the wallet does not wrap this particular account,
// an error is returned to avoid account leakage (even though in theory we may
// be able to sign via our shared keystore backend).
func (w *keystoreWallet) SignTx(accountObj account.Account, tx *model.Transaction, chainID *big.Int) (*model.Transaction, error) {
	// Make sure the requested account is contained within
	if accountObj.Address != w.account.Address {
		return nil, account.ErrUnknownAccount
	}
	if accountObj.URL != (account.URL{}) && accountObj.URL != w.account.URL {
		return nil, account.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignTx(accountObj, tx, chainID)
}

// SignHashWithPassphrase implements account.Wallet, attempting to sign the
// given hash with the given account using passphrase as extra authentication.
func (w *keystoreWallet) SignHashWithPassphrase(accountObj account.Account, passphrase string, hash []byte) ([]byte, error) {
	// Make sure the requested account is contained within
	if accountObj.Address != w.account.Address {
		return nil, account.ErrUnknownAccount
	}
	if accountObj.URL != (account.URL{}) && accountObj.URL != w.account.URL {
		return nil, account.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignHashWithPassphrase(accountObj, passphrase, hash)
}

// SignTxWithPassphrase implements account.Wallet, attempting to sign the given
// transaction with the given account using passphrase as extra authentication.
func (w *keystoreWallet) SignTxWithPassphrase(accountObj account.Account, passphrase string, tx *model.Transaction, chainID *big.Int) (*model.Transaction, error) {
	// Make sure the requested account is contained within
	if accountObj.Address != w.account.Address {
		return nil, account.ErrUnknownAccount
	}
	if accountObj.URL != (account.URL{}) && accountObj.URL != w.account.URL {
		return nil, account.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignTxWithPassphrase(accountObj, passphrase, tx, chainID)
}
