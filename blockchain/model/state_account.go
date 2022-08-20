package model

import (
	"github.com/entropyio/go-entropy/common"
	"math/big"
)

//go:generate go run ../../common/rlp/rlpgen -type StateAccount -out gen_account_rlp.go

// StateAccount is the Entropy consensus representation of accounts.
// These objects are stored in the main account trie.
type StateAccount struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash // merkle root of the storage trie
	CodeHash []byte
}
