package t8ntool

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/mathutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/database/trie"
	"github.com/entropyio/go-entropy/evm"
	"golang.org/x/crypto/sha3"
	"math/big"
	"os"
)

type Prestate struct {
	Env stEnv                   `json:"env"`
	Pre blockchain.GenesisAlloc `json:"pre"`
}

// ExecutionResult contains the execution status after running a state test, any
// error that might have occurred and a dump of the final state if requested.
type ExecutionResult struct {
	StateRoot   common.Hash               `json:"stateRoot"`
	TxRoot      common.Hash               `json:"txRoot"`
	ReceiptRoot common.Hash               `json:"receiptsRoot"`
	LogsHash    common.Hash               `json:"logsHash"`
	Bloom       model.Bloom               `json:"logsBloom"        gencodec:"required"`
	Receipts    model.Receipts            `json:"receipts"`
	Rejected    []*rejectedTx             `json:"rejected,omitempty"`
	Difficulty  *mathutil.HexOrDecimal256 `json:"currentDifficulty" gencodec:"required"`
	GasUsed     mathutil.HexOrDecimal64   `json:"gasUsed"`
}

type ommer struct {
	Delta   uint64         `json:"delta"`
	Address common.Address `json:"address"`
}

//go:generate go run github.com/fjl/gencodec -type stEnv -field-override stEnvMarshaling -out gen_stenv.go
type stEnv struct {
	Coinbase         common.Address                          `json:"currentCoinbase"   gencodec:"required"`
	Difficulty       *big.Int                                `json:"currentDifficulty"`
	Random           *big.Int                                `json:"currentRandom"`
	ParentDifficulty *big.Int                                `json:"parentDifficulty"`
	GasLimit         uint64                                  `json:"currentGasLimit"   gencodec:"required"`
	Number           uint64                                  `json:"currentNumber"     gencodec:"required"`
	Timestamp        uint64                                  `json:"currentTimestamp"  gencodec:"required"`
	ParentTimestamp  uint64                                  `json:"parentTimestamp,omitempty"`
	BlockHashes      map[mathutil.HexOrDecimal64]common.Hash `json:"blockHashes,omitempty"`
	Ommers           []ommer                                 `json:"ommers,omitempty"`
	BaseFee          *big.Int                                `json:"currentBaseFee,omitempty"`
	ParentUncleHash  common.Hash                             `json:"parentUncleHash"`
}

type stEnvMarshaling struct {
	Coinbase         common.UnprefixedAddress
	Difficulty       *mathutil.HexOrDecimal256
	Random           *mathutil.HexOrDecimal256
	ParentDifficulty *mathutil.HexOrDecimal256
	GasLimit         mathutil.HexOrDecimal64
	Number           mathutil.HexOrDecimal64
	Timestamp        mathutil.HexOrDecimal64
	ParentTimestamp  mathutil.HexOrDecimal64
	BaseFee          *mathutil.HexOrDecimal256
}

type rejectedTx struct {
	Index int    `json:"index"`
	Err   string `json:"error"`
}

// Apply applies a set of transactions to a pre-state
func (pre *Prestate) Apply(vmConfig evm.Config, chainConfig *config.ChainConfig,
	txs model.Transactions, miningReward int64,
	getTracerFn func(txIndex int, txHash common.Hash) (tracer evm.EVMLogger, err error)) (*state.StateDB, *ExecutionResult, error) {

	// Capture errors for BLOCKHASH operation, if we haven't been supplied the
	// required blockhashes
	var hashError error
	getHash := func(num uint64) common.Hash {
		if pre.Env.BlockHashes == nil {
			hashError = fmt.Errorf("getHash(%d) invoked, no blockhashes provided", num)
			return common.Hash{}
		}
		h, ok := pre.Env.BlockHashes[mathutil.HexOrDecimal64(num)]
		if !ok {
			hashError = fmt.Errorf("getHash(%d) invoked, blockhash for that block not provided", num)
		}
		return h
	}
	var (
		statedb     = MakePreState(rawdb.NewMemoryDatabase(), pre.Pre)
		signer      = model.MakeSigner(chainConfig, new(big.Int).SetUint64(pre.Env.Number))
		gaspool     = new(blockchain.GasPool)
		blockHash   = common.Hash{0x13, 0x37}
		rejectedTxs []*rejectedTx
		includedTxs model.Transactions
		gasUsed     = uint64(0)
		receipts    = make(model.Receipts, 0)
		txIndex     = 0
	)
	gaspool.AddGas(pre.Env.GasLimit)
	vmContext := evm.BlockContext{
		CanTransfer: blockchain.CanTransfer,
		Transfer:    blockchain.Transfer,
		Coinbase:    pre.Env.Coinbase,
		BlockNumber: new(big.Int).SetUint64(pre.Env.Number),
		Time:        new(big.Int).SetUint64(pre.Env.Timestamp),
		Difficulty:  pre.Env.Difficulty,
		GasLimit:    pre.Env.GasLimit,
		GetHash:     getHash,
	}
	// If currentBaseFee is defined, add it to the vmContext.
	if pre.Env.BaseFee != nil {
		vmContext.BaseFee = new(big.Int).Set(pre.Env.BaseFee)
	}
	// If random is defined, add it to the vmContext.
	if pre.Env.Random != nil {
		rnd := common.BigToHash(pre.Env.Random)
		vmContext.Random = &rnd
	}

	for i, tx := range txs {
		msg, err := tx.AsMessage(signer, pre.Env.BaseFee)
		if err != nil {
			//log.Warn("rejected tx", "index", i, "hash", tx.Hash(), "error", err)
			rejectedTxs = append(rejectedTxs, &rejectedTx{i, err.Error()})
			continue
		}
		tracer, err := getTracerFn(txIndex, tx.Hash())
		if err != nil {
			return nil, nil, err
		}
		vmConfig.Tracer = tracer
		vmConfig.Debug = (tracer != nil)
		statedb.Prepare(tx.Hash(), txIndex)
		txContext := blockchain.NewEVMTxContext(msg)
		snapshot := statedb.Snapshot()
		evm := evm.NewEVM(vmContext, txContext, statedb, chainConfig, vmConfig)

		// (ret []byte, usedGas uint64, failed bool, err error)
		msgResult, err := blockchain.ApplyMessage(evm, msg, gaspool)
		if err != nil {
			statedb.RevertToSnapshot(snapshot)
			//log.Info("rejected tx", "index", i, "hash", tx.Hash(), "from", msg.From(), "error", err)
			rejectedTxs = append(rejectedTxs, &rejectedTx{i, err.Error()})
			continue
		}
		includedTxs = append(includedTxs, tx)
		if hashError != nil {
			return nil, nil, NewError(ErrorMissingBlockhash, hashError)
		}
		gasUsed += msgResult.UsedGas

		// Receipt:
		{
			root := statedb.IntermediateRoot(false).Bytes()

			// Create a new receipt for the transaction, storing the intermediate root and
			// gas used by the tx.
			receipt := &model.Receipt{Type: tx.Type(), PostState: root, CumulativeGasUsed: gasUsed}
			if msgResult.Failed() {
				receipt.Status = model.ReceiptStatusFailed
			} else {
				receipt.Status = model.ReceiptStatusSuccessful
			}
			receipt.TxHash = tx.Hash()
			receipt.GasUsed = msgResult.UsedGas

			// If the transaction created a contract, store the creation address in the receipt.
			if msg.To() == nil {
				receipt.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, tx.Nonce())
			}

			// Set the receipt logs and create the bloom filter.
			receipt.Logs = statedb.GetLogs(tx.Hash(), blockHash)
			receipt.Bloom = model.CreateBloom(model.Receipts{receipt})
			// These three are non-consensus fields:
			//receipt.BlockHash
			//receipt.BlockNumber
			receipt.TransactionIndex = uint(txIndex)
			receipts = append(receipts, receipt)
		}

		txIndex++
	}
	statedb.IntermediateRoot(false)
	// Add mining reward?
	if miningReward > 0 {
		// Add mining reward. The mining reward may be `0`, which only makes a difference in the cases
		// where
		// - the coinbase suicided, or
		// - there are only 'bad' transactions, which aren't executed. In those cases,
		//   the coinbase gets no txfee, so isn't created, and thus needs to be touched
		var (
			blockReward = big.NewInt(miningReward)
			minerReward = new(big.Int).Set(blockReward)
			perOmmer    = new(big.Int).Div(blockReward, big.NewInt(32))
		)
		for _, ommer := range pre.Env.Ommers {
			// Add 1/32th for each ommer included
			minerReward.Add(minerReward, perOmmer)
			// Add (8-delta)/8
			reward := big.NewInt(8)
			reward.Sub(reward, new(big.Int).SetUint64(ommer.Delta))
			reward.Mul(reward, blockReward)
			reward.Div(reward, big.NewInt(8))
			statedb.AddBalance(ommer.Address, reward)
		}
		statedb.AddBalance(pre.Env.Coinbase, minerReward)
	}
	// Commit block
	root, err := statedb.Commit(false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not commit state: %v", err)
		return nil, nil, NewError(ErrorEVM, fmt.Errorf("could not commit state: %v", err))
	}
	execRs := &ExecutionResult{
		StateRoot:   root,
		TxRoot:      model.DeriveSha(includedTxs, trie.NewStackTrie(nil)),
		ReceiptRoot: model.DeriveSha(receipts, trie.NewStackTrie(nil)),
		Bloom:       model.CreateBloom(receipts),
		LogsHash:    rlpHash(statedb.Logs()),
		Receipts:    receipts,
		Rejected:    rejectedTxs,
		Difficulty:  (*mathutil.HexOrDecimal256)(vmContext.Difficulty),
		GasUsed:     (mathutil.HexOrDecimal64)(gasUsed),
	}
	return statedb, execRs, nil
}

func MakePreState(db database.Database, accounts blockchain.GenesisAlloc) *state.StateDB {
	sdb := state.NewDatabase(db)
	statedb, _ := state.New(common.Hash{}, sdb, nil)
	for addr, a := range accounts {
		statedb.SetCode(addr, a.Code)
		statedb.SetNonce(addr, a.Nonce)
		statedb.SetBalance(addr, a.Balance)
		for k, v := range a.Storage {
			statedb.SetState(addr, k, v)
		}
	}
	// Commit and re-open to start with a clean state.
	root, _ := statedb.Commit(false)
	statedb, _ = state.New(root, sdb, nil)
	return statedb
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

// calcDifficulty is based on ethash.CalcDifficulty. This method is used in case
// the caller does not provide an explicit difficulty, but instead provides only
// parent timestamp + difficulty.
// Note: this method only works for ethash engine.
func calcDifficulty(config *config.ChainConfig, number, currentTime, parentTime uint64,
	parentDifficulty *big.Int, parentUncleHash common.Hash) *big.Int {
	uncleHash := parentUncleHash
	if uncleHash == (common.Hash{}) {
		uncleHash = model.EmptyUncleHash
	}
	parent := &model.Header{
		ParentHash: common.Hash{},
		UncleHash:  uncleHash,
		Difficulty: parentDifficulty,
		Number:     new(big.Int).SetUint64(number - 1),
		Time:       parentTime,
	}
	return ethash.CalcDifficulty(config, currentTime, parent)
}
