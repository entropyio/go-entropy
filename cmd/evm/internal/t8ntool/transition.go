package t8ntool

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/tests"
	"github.com/labstack/gommon/log"
	"github.com/urfave/cli/v2"
	"math/big"
	"os"
	"path"
	"strings"
)

const (
	ErrorEVM              = 2
	ErrorConfig           = 3
	ErrorMissingBlockhash = 4

	ErrorJson = 10
	ErrorIO   = 11
	ErrorRlp  = 12

	stdinSelector = "stdin"
)

type NumberedError struct {
	errorCode int
	err       error
}

func NewError(errorCode int, err error) *NumberedError {
	return &NumberedError{errorCode, err}
}

func (n *NumberedError) Error() string {
	return fmt.Sprintf("ERROR(%d): %v", n.errorCode, n.err.Error())
}

func (n *NumberedError) ExitCode() int {
	return n.errorCode
}

// compile-time conformance test
var (
	_ cli.ExitCoder = (*NumberedError)(nil)
)

type input struct {
	Alloc blockchain.GenesisAlloc `json:"alloc,omitempty"`
	Env   *stEnv                  `json:"env,omitempty"`
	Txs   []*txWithKey            `json:"txs,omitempty"`
	TxRlp string                  `json:"txsRlp,omitempty"`
}

func Transition(ctx *cli.Context) error {
	var (
		err    error
		tracer evm.EVMLogger
	)
	var getTracer func(txIndex int, txHash common.Hash) (evm.EVMLogger, error)

	baseDir, err := createBasedir(ctx)
	if err != nil {
		return NewError(ErrorIO, fmt.Errorf("failed creating output basedir: %v", err))
	}
	if ctx.Bool(TraceFlag.Name) {
		if ctx.IsSet(TraceDisableMemoryFlag.Name) && ctx.IsSet(TraceEnableMemoryFlag.Name) {
			return NewError(ErrorConfig, fmt.Errorf("can't use both flags --%s and --%s", TraceDisableMemoryFlag.Name, TraceEnableMemoryFlag.Name))
		}
		if ctx.IsSet(TraceDisableReturnDataFlag.Name) && ctx.IsSet(TraceEnableReturnDataFlag.Name) {
			return NewError(ErrorConfig, fmt.Errorf("can't use both flags --%s and --%s", TraceDisableReturnDataFlag.Name, TraceEnableReturnDataFlag.Name))
		}
		if ctx.IsSet(TraceDisableMemoryFlag.Name) {
			log.Warn(fmt.Sprintf("--%s has been deprecated in favour of --%s", TraceDisableMemoryFlag.Name, TraceEnableMemoryFlag.Name))
		}
		if ctx.IsSet(TraceDisableReturnDataFlag.Name) {
			log.Warn(fmt.Sprintf("--%s has been deprecated in favour of --%s", TraceDisableReturnDataFlag.Name, TraceEnableReturnDataFlag.Name))
		}

		var prevFile *os.File
		// This one closes the last file
		defer func() {
			if prevFile != nil {
				prevFile.Close()
			}
		}()
	} else {
		getTracer = func(txIndex int, txHash common.Hash) (tracer evm.EVMLogger, err error) {
			return nil, nil
		}
	}
	// We need to load three things: alloc, env and transactions. May be either in
	// stdin input or in files.
	// Check if anything needs to be read from stdin
	var (
		prestate Prestate
		txs      model.Transactions // txs to apply
		allocStr = ctx.String(InputAllocFlag.Name)

		envStr    = ctx.String(InputEnvFlag.Name)
		txStr     = ctx.String(InputTxsFlag.Name)
		inputData = &input{}
	)
	// Figure out the prestate alloc
	if allocStr == stdinSelector || envStr == stdinSelector || txStr == stdinSelector {
		decoder := json.NewDecoder(os.Stdin)
		if err := decoder.Decode(inputData); err != nil {
			return NewError(ErrorJson, fmt.Errorf("failed unmarshaling stdin: %v", err))
		}
	}
	if allocStr != stdinSelector {
		if err := readFile(allocStr, "alloc", &inputData.Alloc); err != nil {
			return err
		}
	}
	prestate.Pre = inputData.Alloc

	// Set the block environment
	if envStr != stdinSelector {
		var env stEnv
		if err := readFile(envStr, "env", &env); err != nil {
			return err
		}
		inputData.Env = &env
	}
	prestate.Env = *inputData.Env

	vmConfig := evm.Config{
		Tracer: tracer,
		Debug:  (tracer != nil),
	}
	// Construct the chainconfig
	var chainConfig *config.ChainConfig
	if cConf, extraEips, err := tests.GetChainConfig(ctx.String(ForknameFlag.Name)); err != nil {
		return NewError(ErrorConfig, fmt.Errorf("failed constructing chain configuration: %v", err))
	} else {
		chainConfig = cConf
		vmConfig.ExtraEips = extraEips
	}
	// Set the chain id
	chainConfig.ChainID = big.NewInt(ctx.Int64(ChainIDFlag.Name))

	var txsWithKeys []*txWithKey
	if txStr != stdinSelector {
		inFile, err := os.Open(txStr)
		if err != nil {
			return NewError(ErrorIO, fmt.Errorf("failed reading txs file: %v", err))
		}
		defer inFile.Close()
		decoder := json.NewDecoder(inFile)
		if strings.HasSuffix(txStr, ".rlp") {
			var body hexutil.Bytes
			if err := decoder.Decode(&body); err != nil {
				return err
			}
			var txs model.Transactions
			if err := rlp.DecodeBytes(body, &txs); err != nil {
				return err
			}
			for _, tx := range txs {
				txsWithKeys = append(txsWithKeys, &txWithKey{
					key: nil,
					tx:  tx,
				})
			}
		} else {
			if err := decoder.Decode(&txsWithKeys); err != nil {
				return NewError(ErrorJson, fmt.Errorf("failed unmarshaling txs-file: %v", err))
			}
		}
	} else {
		if len(inputData.TxRlp) > 0 {
			// Decode the body of already signed transactions
			body := common.FromHex(inputData.TxRlp)
			var txs model.Transactions
			if err := rlp.DecodeBytes(body, &txs); err != nil {
				return err
			}
			for _, tx := range txs {
				txsWithKeys = append(txsWithKeys, &txWithKey{
					key: nil,
					tx:  tx,
				})
			}
		} else {
			// JSON encoded transactions
			txsWithKeys = inputData.Txs
		}
	}
	// We may have to sign the transactions.
	signer := model.MakeSigner(chainConfig, big.NewInt(int64(prestate.Env.Number)))

	if txs, err = signUnsignedTransactions(txsWithKeys, signer); err != nil {
		return NewError(ErrorJson, fmt.Errorf("failed signing transactions: %v", err))
	}
	// Sanity check, to not `panic` in state_transition
	if chainConfig.IsLondon(big.NewInt(int64(prestate.Env.Number))) {
		if prestate.Env.BaseFee == nil {
			return NewError(ErrorConfig, errors.New("EIP-1559 config but missing 'currentBaseFee' in env section"))
		}
	}
	isMerged := chainConfig.TerminalTotalDifficulty != nil && chainConfig.TerminalTotalDifficulty.BitLen() == 0
	env := prestate.Env
	if isMerged {
		// post-merge:
		// - random must be supplied
		// - difficulty must be zero
		switch {
		case env.Random == nil:
			return NewError(ErrorConfig, errors.New("post-merge requires currentRandom to be defined in env"))
		case env.Difficulty != nil && env.Difficulty.BitLen() != 0:
			return NewError(ErrorConfig, errors.New("post-merge difficulty must be zero (or omitted) in env"))
		}
		prestate.Env.Difficulty = nil
	} else if env.Difficulty == nil {
		// pre-merge:
		// If difficulty was not provided by caller, we need to calculate it.
		switch {
		case env.ParentDifficulty == nil:
			return NewError(ErrorConfig, errors.New("currentDifficulty was not provided, and cannot be calculated due to missing parentDifficulty"))
		case env.Number == 0:
			return NewError(ErrorConfig, errors.New("currentDifficulty needs to be provided for block number 0"))
		case env.Timestamp <= env.ParentTimestamp:
			return NewError(ErrorConfig, fmt.Errorf("currentDifficulty cannot be calculated -- currentTime (%d) needs to be after parent time (%d)",
				env.Timestamp, env.ParentTimestamp))
		}
		prestate.Env.Difficulty = calcDifficulty(chainConfig, env.Number, env.Timestamp,
			env.ParentTimestamp, env.ParentDifficulty, env.ParentUncleHash)
	}
	// Run the test and aggregate the result
	s, result, err := prestate.Apply(vmConfig, chainConfig, txs, ctx.Int64(RewardFlag.Name), getTracer)
	if err != nil {
		return err
	}
	body, _ := rlp.EncodeToBytes(txs)
	// Dump the excution result
	collector := make(Alloc)
	s.DumpToCollector(collector, nil)
	return dispatchOutput(ctx, baseDir, result, collector, body)
}

// txWithKey is a helper-struct, to allow us to use the model.Transaction along with
// a `secretKey`-field, for input
type txWithKey struct {
	key       *ecdsa.PrivateKey
	tx        *model.Transaction
	protected bool
}

func (t *txWithKey) UnmarshalJSON(input []byte) error {
	// Read the metadata, if present
	type txMetadata struct {
		Key       *common.Hash `json:"secretKey"`
		Protected *bool        `json:"protected"`
	}
	var data txMetadata
	if err := json.Unmarshal(input, &data); err != nil {
		return err
	}
	if data.Key != nil {
		k := data.Key.Hex()[2:]
		if ecdsaKey, err := crypto.HexToECDSA(k); err != nil {
			return err
		} else {
			t.key = ecdsaKey
		}
	}
	if data.Protected != nil {
		t.protected = *data.Protected
	} else {
		t.protected = true
	}
	// Now, read the transaction itself
	var tx model.Transaction
	if err := json.Unmarshal(input, &tx); err != nil {
		return err
	}
	t.tx = &tx
	return nil
}

// signUnsignedTransactions converts the input txs to canonical transactions.
//
// The transactions can have two forms, either
//   1. unsigned or
//   2. signed
// For (1), r, s, v, need so be zero, and the `secretKey` needs to be set.
// If so, we sign it here and now, with the given `secretKey`
// If the condition above is not met, then it's considered a signed transaction.
//
// To manage this, we read the transactions twice, first trying to read the secretKeys,
// and secondly to read them with the standard tx json format
func signUnsignedTransactions(txs []*txWithKey, signer model.Signer) (model.Transactions, error) {
	var signedTxs []*model.Transaction
	for i, txWithKey := range txs {
		tx := txWithKey.tx
		key := txWithKey.key
		v, r, s := tx.RawSignatureValues()
		if key != nil && v.BitLen()+r.BitLen()+s.BitLen() == 0 {
			// This transaction needs to be signed
			var (
				signed *model.Transaction
				err    error
			)
			if txWithKey.protected {
				signed, err = model.SignTx(tx, signer, key)
			} else {
				signed, err = model.SignTx(tx, model.FrontierSigner{}, key)
			}
			if err != nil {
				return nil, NewError(ErrorJson, fmt.Errorf("tx %d: failed to sign tx: %v", i, err))
			}
			signedTxs = append(signedTxs, signed)
		} else {
			// Already signed
			signedTxs = append(signedTxs, tx)
		}
	}
	return signedTxs, nil
}

type Alloc map[common.Address]blockchain.GenesisAccount

func (g Alloc) OnRoot(common.Hash) {}

func (g Alloc) OnAccount(addr common.Address, dumpAccount state.DumpAccount) {
	balance, _ := new(big.Int).SetString(dumpAccount.Balance, 10)
	var storage map[common.Hash]common.Hash
	if dumpAccount.Storage != nil {
		storage = make(map[common.Hash]common.Hash)
		for k, v := range dumpAccount.Storage {
			storage[k] = common.HexToHash(v)
		}
	}
	genesisAccount := blockchain.GenesisAccount{
		Code:    dumpAccount.Code,
		Storage: storage,
		Balance: balance,
		Nonce:   dumpAccount.Nonce,
	}
	g[addr] = genesisAccount
}

// saveFile marshalls the object to the given file
func saveFile(baseDir, filename string, data interface{}) error {
	b, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return NewError(ErrorJson, fmt.Errorf("failed marshalling output: %v", err))
	}
	location := path.Join(baseDir, filename)
	if err = os.WriteFile(location, b, 0644); err != nil {
		return NewError(ErrorIO, fmt.Errorf("failed writing output: %v", err))
	}
	log.Info("Wrote file", "file", location)
	return nil
}

// dispatchOutput writes the output data to either stderr or stdout, or to the specified
// files
func dispatchOutput(ctx *cli.Context, baseDir string, result *ExecutionResult, alloc Alloc, body hexutil.Bytes) error {
	stdOutObject := make(map[string]interface{})
	stdErrObject := make(map[string]interface{})
	dispatch := func(baseDir, fName, name string, obj interface{}) error {
		switch fName {
		case "stdout":
			stdOutObject[name] = obj
		case "stderr":
			stdErrObject[name] = obj
		case "":
			// don't save
		default: // save to file
			if err := saveFile(baseDir, fName, obj); err != nil {
				return err
			}
		}
		return nil
	}
	if err := dispatch(baseDir, ctx.String(OutputAllocFlag.Name), "alloc", alloc); err != nil {
		return err
	}
	if err := dispatch(baseDir, ctx.String(OutputResultFlag.Name), "result", result); err != nil {
		return err
	}
	if err := dispatch(baseDir, ctx.String(OutputBodyFlag.Name), "body", body); err != nil {
		return err
	}
	if len(stdOutObject) > 0 {
		b, err := json.MarshalIndent(stdOutObject, "", "  ")
		if err != nil {
			return NewError(ErrorJson, fmt.Errorf("failed marshalling output: %v", err))
		}
		os.Stdout.Write(b)
		os.Stdout.WriteString("\n")
	}
	if len(stdErrObject) > 0 {
		b, err := json.MarshalIndent(stdErrObject, "", "  ")
		if err != nil {
			return NewError(ErrorJson, fmt.Errorf("failed marshalling output: %v", err))
		}
		os.Stderr.Write(b)
		os.Stderr.WriteString("\n")
	}
	return nil
}
