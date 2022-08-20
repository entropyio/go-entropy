package tests

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
)

// TransactionTest checks RLP decoding and sender derivation of transactions.
type TransactionTest struct {
	RLP            hexutil.Bytes `json:"rlp"`
	Byzantium      ttFork
	Constantinople ttFork
	Istanbul       ttFork
	EIP150         ttFork
	EIP158         ttFork
	Frontier       ttFork
	Homestead      ttFork
}

type ttFork struct {
	Sender common.UnprefixedAddress `json:"sender"`
	Hash   common.UnprefixedHash    `json:"hash"`
}

func (tt *TransactionTest) Run(configObj *config.ChainConfig) error {
	validateTx := func(rlpData hexutil.Bytes, signer model.Signer, isHomestead bool) (*common.Address, *common.Hash, error) {
		tx := new(model.Transaction)
		if err := rlp.DecodeBytes(rlpData, tx); err != nil {
			return nil, nil, err
		}
		sender, err := model.Sender(signer, tx)
		if err != nil {
			return nil, nil, err
		}
		// Intrinsic gas
		requiredGas, err := blockchain.IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil, isHomestead)
		if err != nil {
			return nil, nil, err
		}
		if requiredGas > tx.Gas() {
			return nil, nil, fmt.Errorf("insufficient gas ( %d < %d )", tx.Gas(), requiredGas)
		}
		h := tx.Hash()
		return &sender, &h, nil
	}

	for _, testcase := range []struct {
		name        string
		signer      model.Signer
		fork        ttFork
		isHomestead bool
	}{
		{"Frontier", model.FrontierSigner{}, tt.Frontier, false},
		{"Homestead", model.HomesteadSigner{}, tt.Homestead, true},
		{"EIP150", model.HomesteadSigner{}, tt.EIP150, true},
	} {
		sender, txhash, err := validateTx(tt.RLP, testcase.signer, testcase.isHomestead)

		if testcase.fork.Sender == (common.UnprefixedAddress{}) {
			if err == nil {
				return fmt.Errorf("expected error, got none (address %v)[%v]", sender.String(), testcase.name)
			}
			continue
		}
		// Should resolve the right address
		if err != nil {
			return fmt.Errorf("got error, expected none: %v", err)
		}
		if sender == nil {
			return fmt.Errorf("sender was nil, should be %x", common.Address(testcase.fork.Sender))
		}
		if *sender != common.Address(testcase.fork.Sender) {
			return fmt.Errorf("sender mismatch: got %x, want %x", sender, testcase.fork.Sender)
		}
		if txhash == nil {
			return fmt.Errorf("txhash was nil, should be %x", common.Hash(testcase.fork.Hash))
		}
		if *txhash != common.Hash(testcase.fork.Hash) {
			return fmt.Errorf("hash mismatch: got %x, want %x", *txhash, testcase.fork.Hash)
		}
	}
	return nil
}
