package keystore

import (
	"github.com/entropyio/go-entropy"
	"github.com/entropyio/go-entropy/accounts"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common/crypto"
	"math/big"
)

// keystoreWallet implements the accounts.Wallet interface for the original
// keystore.
type keystoreWallet struct {
	account  accounts.Account // Single account contained in this wallet
	keystore *KeyStore        // Keystore where the account originates from
}

// URL implements accounts.Wallet, returning the URL of the account within.
func (w *keystoreWallet) URL() accounts.URL {
	return w.account.URL
}

// Status implements accounts.Wallet, returning whether the account held by the
// keystore wallet is unlocked or not.
func (w *keystoreWallet) Status() (string, error) {
	w.keystore.mu.RLock()
	defer w.keystore.mu.RUnlock()

	if _, ok := w.keystore.unlocked[w.account.Address]; ok {
		return "Unlocked", nil
	}
	return "Locked", nil
}

// Open implements accounts.Wallet, but is a noop for plain wallets since there
// is no connection or decryption step necessary to access the list of accounts.
func (w *keystoreWallet) Open(passphrase string) error { return nil }

// Close implements accounts.Wallet, but is a noop for plain wallets since there
// is no meaningful open operation.
func (w *keystoreWallet) Close() error { return nil }

// Accounts implements accounts.Wallet, returning an account list consisting of
// a single account that the plain keystore wallet contains.
func (w *keystoreWallet) Accounts() []accounts.Account {
	return []accounts.Account{w.account}
}

// Contains implements accounts.Wallet, returning whether a particular account is
// or is not wrapped by this wallet instance.
func (w *keystoreWallet) Contains(accountObj accounts.Account) bool {
	return accountObj.Address == w.account.Address && (accountObj.URL == (accounts.URL{}) || accountObj.URL == w.account.URL)
}

// Derive implements accounts.Wallet, but is a noop for plain wallets since there
// is no notion of hierarchical account derivation for plain keystore accounts.
func (w *keystoreWallet) Derive(path accounts.DerivationPath, pin bool) (accounts.Account, error) {
	return accounts.Account{}, accounts.ErrNotSupported
}

// SelfDerive implements accounts.Wallet, but is a noop for plain wallets since
// there is no notion of hierarchical account derivation for plain keystore accounts.
func (w *keystoreWallet) SelfDerive(bases []accounts.DerivationPath, chain entropyio.ChainStateReader) {
}

// signHash attempts to sign the given hash with
// the given account. If the wallet does not wrap this particular account, an
// error is returned to avoid account leakage (even though in theory we may be
// able to sign via our shared keystore backend).
func (w *keystoreWallet) signHash(accountObj accounts.Account, hash []byte) ([]byte, error) {
	// Make sure the requested account is contained within
	if !w.Contains(accountObj) {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignHash(accountObj, hash)
}

// SignData signs keccak256(data). The mimetype parameter describes the type of data being signed
func (w *keystoreWallet) SignData(accountObj accounts.Account, mimeType string, data []byte) ([]byte, error) {
	return w.signHash(accountObj, crypto.Keccak256(data))
}

// SignDataWithPassphrase signs keccak256(data). The mimetype parameter describes the type of data being signed
func (w *keystoreWallet) SignDataWithPassphrase(accountObj accounts.Account, passphrase, mimeType string, data []byte) ([]byte, error) {
	// Make sure the requested account is contained within
	if !w.Contains(accountObj) {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignHashWithPassphrase(accountObj, passphrase, crypto.Keccak256(data))
}

// SignText implements accounts.Wallet, attempting to sign the hash of
// the given text with the given account.
func (w *keystoreWallet) SignText(accountObj accounts.Account, text []byte) ([]byte, error) {
	return w.signHash(accountObj, accounts.TextHash(text))
}

// SignTextWithPassphrase implements accounts.Wallet, attempting to sign the
// hash of the given text with the given account using passphrase as extra authentication.
func (w *keystoreWallet) SignTextWithPassphrase(accountObj accounts.Account, passphrase string, text []byte) ([]byte, error) {
	// Make sure the requested account is contained within
	if !w.Contains(accountObj) {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignHashWithPassphrase(accountObj, passphrase, accounts.TextHash(text))
}

// SignTx implements accounts.Wallet, attempting to sign the given transaction
// with the given account. If the wallet does not wrap this particular account,
// an error is returned to avoid account leakage (even though in theory we may
// be able to sign via our shared keystore backend).
func (w *keystoreWallet) SignTx(accountObj accounts.Account, tx *model.Transaction, chainID *big.Int) (*model.Transaction, error) {
	// Make sure the requested account is contained within
	if !w.Contains(accountObj) {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignTx(accountObj, tx, chainID)
}

// SignTxWithPassphrase implements accounts.Wallet, attempting to sign the given
// transaction with the given account using passphrase as extra authentication.
func (w *keystoreWallet) SignTxWithPassphrase(accountObj accounts.Account, passphrase string, tx *model.Transaction, chainID *big.Int) (*model.Transaction, error) {
	// Make sure the requested account is contained within
	if !w.Contains(accountObj) {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	return w.keystore.SignTxWithPassphrase(accountObj, passphrase, tx, chainID)
}
