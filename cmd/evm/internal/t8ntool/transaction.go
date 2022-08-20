package t8ntool

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/tests"
	"github.com/urfave/cli/v2"
	"math/big"
	"os"
	"strings"
)

type result struct {
	Error        error
	Address      common.Address
	Hash         common.Hash
	IntrinsicGas uint64
}

// MarshalJSON marshals as JSON with a hash.
func (r *result) MarshalJSON() ([]byte, error) {
	type xx struct {
		Error        string          `json:"error,omitempty"`
		Address      *common.Address `json:"address,omitempty"`
		Hash         *common.Hash    `json:"hash,omitempty"`
		IntrinsicGas hexutil.Uint64  `json:"intrinsicGas,omitempty"`
	}
	var out xx
	if r.Error != nil {
		out.Error = r.Error.Error()
	}
	if r.Address != (common.Address{}) {
		out.Address = &r.Address
	}
	if r.Hash != (common.Hash{}) {
		out.Hash = &r.Hash
	}
	out.IntrinsicGas = hexutil.Uint64(r.IntrinsicGas)
	return json.Marshal(out)
}

func Transaction(ctx *cli.Context) error {
	var (
		err error
	)
	// We need to load the transactions. May be either in stdin input or in files.
	// Check if anything needs to be read from stdin
	var (
		txStr       = ctx.String(InputTxsFlag.Name)
		inputData   = &input{}
		chainConfig *config.ChainConfig
	)
	// Construct the chainconfig
	if cConf, _, err := tests.GetChainConfig(ctx.String(ForknameFlag.Name)); err != nil {
		return NewError(ErrorConfig, fmt.Errorf("failed constructing chain configuration: %v", err))
	} else {
		chainConfig = cConf
	}
	// Set the chain id
	chainConfig.ChainID = big.NewInt(ctx.Int64(ChainIDFlag.Name))
	var body hexutil.Bytes
	if txStr == stdinSelector {
		decoder := json.NewDecoder(os.Stdin)
		if err := decoder.Decode(inputData); err != nil {
			return NewError(ErrorJson, fmt.Errorf("failed unmarshaling stdin: %v", err))
		}
		// Decode the body of already signed transactions
		body = common.FromHex(inputData.TxRlp)
	} else {
		// Read input from file
		inFile, err := os.Open(txStr)
		if err != nil {
			return NewError(ErrorIO, fmt.Errorf("failed reading txs file: %v", err))
		}
		defer inFile.Close()
		decoder := json.NewDecoder(inFile)
		if strings.HasSuffix(txStr, ".rlp") {
			if err := decoder.Decode(&body); err != nil {
				return err
			}
		} else {
			return NewError(ErrorIO, errors.New("only rlp supported"))
		}
	}
	signer := model.MakeSigner(chainConfig, new(big.Int))
	// We now have the transactions in 'body', which is supposed to be an
	// rlp list of transactions
	it, err := rlp.NewListIterator([]byte(body))
	if err != nil {
		return err
	}
	var results []result
	for it.Next() {
		if err := it.Err(); err != nil {
			return NewError(ErrorIO, err)
		}
		var tx model.Transaction
		err := rlp.DecodeBytes(it.Value(), &tx)
		if err != nil {
			results = append(results, result{Error: err})
			continue
		}
		r := result{Hash: tx.Hash()}
		if sender, err := model.Sender(signer, &tx); err != nil {
			r.Error = err
			results = append(results, r)
			continue
		} else {
			r.Address = sender
		}
		// Check intrinsic gas
		if gas, err := blockchain.IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil,
			chainConfig.IsHomestead(new(big.Int))); err != nil {
			r.Error = err
			results = append(results, r)
			continue
		} else {
			r.IntrinsicGas = gas
			if tx.Gas() < gas {
				r.Error = fmt.Errorf("%w: have %d, want %d", blockchain.ErrIntrinsicGas, tx.Gas(), gas)
				results = append(results, r)
				continue
			}
		}
		// Validate <256bit fields
		switch {
		case tx.Nonce()+1 < tx.Nonce():
			r.Error = errors.New("nonce exceeds 2^64-1")
		case tx.Value().BitLen() > 256:
			r.Error = errors.New("value exceeds 256 bits")
		case tx.GasPrice().BitLen() > 256:
			r.Error = errors.New("gasPrice exceeds 256 bits")
		case tx.GasTipCap().BitLen() > 256:
			r.Error = errors.New("maxPriorityFeePerGas exceeds 256 bits")
		case tx.GasFeeCap().BitLen() > 256:
			r.Error = errors.New("maxFeePerGas exceeds 256 bits")
		case tx.GasFeeCap().Cmp(tx.GasTipCap()) < 0:
			r.Error = errors.New("maxFeePerGas < maxPriorityFeePerGas")
		case new(big.Int).Mul(tx.GasPrice(), new(big.Int).SetUint64(tx.Gas())).BitLen() > 256:
			r.Error = errors.New("gas * gasPrice exceeds 256 bits")
		case new(big.Int).Mul(tx.GasFeeCap(), new(big.Int).SetUint64(tx.Gas())).BitLen() > 256:
			r.Error = errors.New("gas * maxFeePerGas exceeds 256 bits")
		}
		results = append(results, r)
	}
	out, err := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(out))
	return err
}