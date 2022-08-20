package bind

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"github.com/entropyio/go-entropy/accounts"
	"github.com/entropyio/go-entropy/accounts/external"
	"github.com/entropyio/go-entropy/accounts/keystore"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"io"
	"math/big"
)

// ErrNoChainID is returned whenever the user failed to specify a chain id.
var ErrNoChainID = errors.New("no chain id specified")

// ErrNotAuthorized is returned when an account is not properly unlocked.
var ErrNotAuthorized = errors.New("not authorized to sign this account")

// NewTransactor is a utility method to easily create a transaction signer from
// an encrypted json key stream and the associated passphrase.
//
// Deprecated: Use NewTransactorWithChainID instead.
func NewTransactor(keyin io.Reader, passphrase string) (*TransactOpts, error) {
	log.Warning("WARNING: NewTransactor has been deprecated in favour of NewTransactorWithChainID")
	json, err := io.ReadAll(keyin)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptKey(json, passphrase)
	if err != nil {
		return nil, err
	}
	return NewKeyedTransactor(key.PrivateKey), nil
}

// NewKeyStoreTransactor is a utility method to easily create a transaction signer from
// an decrypted key from a keystore.
//
// Deprecated: Use NewKeyStoreTransactorWithChainID instead.
func NewKeyStoreTransactor(keystore *keystore.KeyStore, account accounts.Account) (*TransactOpts, error) {
	log.Warning("WARNING: NewKeyStoreTransactor has been deprecated in favour of NewTransactorWithChainID")
	signer := model.HomesteadSigner{}
	return &TransactOpts{
		From: account.Address,
		Signer: func(address common.Address, tx *model.Transaction) (*model.Transaction, error) {
			if address != account.Address {
				return nil, ErrNotAuthorized
			}
			signature, err := keystore.SignHash(account, signer.Hash(tx).Bytes())
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}, nil
}

// NewKeyedTransactor is a utility method to easily create a transaction signer
// from a single private key.
//
// Deprecated: Use NewKeyedTransactorWithChainID instead.
func NewKeyedTransactor(key *ecdsa.PrivateKey) *TransactOpts {
	log.Warning("WARNING: NewKeyedTransactor has been deprecated in favour of NewKeyedTransactorWithChainID")
	keyAddr := crypto.PubkeyToAddress(key.PublicKey)
	signer := model.HomesteadSigner{}
	return &TransactOpts{
		From: keyAddr,
		Signer: func(address common.Address, tx *model.Transaction) (*model.Transaction, error) {
			if address != keyAddr {
				return nil, ErrNotAuthorized
			}
			signature, err := crypto.Sign(signer.Hash(tx).Bytes(), key)
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}
}

// NewTransactorWithChainID is a utility method to easily create a transaction signer from
// an encrypted json key stream and the associated passphrase.
func NewTransactorWithChainID(keyin io.Reader, passphrase string, chainID *big.Int) (*TransactOpts, error) {
	json, err := io.ReadAll(keyin)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptKey(json, passphrase)
	if err != nil {
		return nil, err
	}
	return NewKeyedTransactorWithChainID(key.PrivateKey, chainID)
}

// NewKeyStoreTransactorWithChainID is a utility method to easily create a transaction signer from
// an decrypted key from a keystore.
func NewKeyStoreTransactorWithChainID(keystore *keystore.KeyStore, account accounts.Account, chainID *big.Int) (*TransactOpts, error) {
	if chainID == nil {
		return nil, ErrNoChainID
	}
	signer := model.LatestSignerForChainID(chainID)
	return &TransactOpts{
		From: account.Address,
		Signer: func(address common.Address, tx *model.Transaction) (*model.Transaction, error) {
			if address != account.Address {
				return nil, ErrNotAuthorized
			}
			signature, err := keystore.SignHash(account, signer.Hash(tx).Bytes())
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}, nil
}

// NewKeyedTransactorWithChainID is a utility method to easily create a transaction signer
// from a single private key.
func NewKeyedTransactorWithChainID(key *ecdsa.PrivateKey, chainID *big.Int) (*TransactOpts, error) {
	keyAddr := crypto.PubkeyToAddress(key.PublicKey)
	if chainID == nil {
		return nil, ErrNoChainID
	}
	signer := model.LatestSignerForChainID(chainID)
	return &TransactOpts{
		From: keyAddr,
		Signer: func(address common.Address, tx *model.Transaction) (*model.Transaction, error) {
			if address != keyAddr {
				return nil, ErrNotAuthorized
			}
			signature, err := crypto.Sign(signer.Hash(tx).Bytes(), key)
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(signer, signature)
		},
		Context: context.Background(),
	}, nil
}

// NewClefTransactor is a utility method to easily create a transaction signer
// with a clef backend.
func NewClefTransactor(clef *external.ExternalSigner, account accounts.Account) *TransactOpts {
	return &TransactOpts{
		From: account.Address,
		Signer: func(address common.Address, transaction *model.Transaction) (*model.Transaction, error) {
			if address != account.Address {
				return nil, ErrNotAuthorized
			}
			return clef.SignTx(account, transaction, nil) // Clef enforces its own chain id
		},
		Context: context.Background(),
	}
}
