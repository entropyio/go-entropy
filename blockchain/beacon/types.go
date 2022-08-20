package beacon

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/database/trie"
	"math/big"
)

//go:generate go run github.com/fjl/gencodec -type PayloadAttributesV1 -field-override payloadAttributesMarshaling -out gen_blockparams.go

// PayloadAttributesV1 structure described at https://github.com/entropy/execution-apis/pull/74
type PayloadAttributesV1 struct {
	Timestamp             uint64         `json:"timestamp"     gencodec:"required"`
	Random                common.Hash    `json:"prevRandao"        gencodec:"required"`
	SuggestedFeeRecipient common.Address `json:"suggestedFeeRecipient"  gencodec:"required"`
}

// JSON type overrides for PayloadAttributesV1.
type payloadAttributesMarshaling struct {
	Timestamp hexutil.Uint64
}

//go:generate go run github.com/fjl/gencodec -type ExecutableDataV1 -field-override executableDataMarshaling -out gen_ed.go

// ExecutableDataV1 structure described at https://github.com/entropy/execution-apis/src/engine/specification.md
type ExecutableDataV1 struct {
	ParentHash    common.Hash    `json:"parentHash"    gencodec:"required"`
	FeeRecipient  common.Address `json:"feeRecipient"  gencodec:"required"`
	StateRoot     common.Hash    `json:"stateRoot"     gencodec:"required"`
	ReceiptsRoot  common.Hash    `json:"receiptsRoot"  gencodec:"required"`
	LogsBloom     []byte         `json:"logsBloom"     gencodec:"required"`
	Random        common.Hash    `json:"prevRandao"    gencodec:"required"`
	Number        uint64         `json:"blockNumber"   gencodec:"required"`
	GasLimit      uint64         `json:"gasLimit"      gencodec:"required"`
	GasUsed       uint64         `json:"gasUsed"       gencodec:"required"`
	Timestamp     uint64         `json:"timestamp"     gencodec:"required"`
	ExtraData     []byte         `json:"extraData"     gencodec:"required"`
	BaseFeePerGas *big.Int       `json:"baseFeePerGas" gencodec:"required"`
	BlockHash     common.Hash    `json:"blockHash"     gencodec:"required"`
	Transactions  [][]byte       `json:"transactions"  gencodec:"required"`
}

// JSON type overrides for executableData.
type executableDataMarshaling struct {
	Number        hexutil.Uint64
	GasLimit      hexutil.Uint64
	GasUsed       hexutil.Uint64
	Timestamp     hexutil.Uint64
	BaseFeePerGas *hexutil.Big
	ExtraData     hexutil.Bytes
	LogsBloom     hexutil.Bytes
	Transactions  []hexutil.Bytes
}

type PayloadStatusV1 struct {
	Status          string       `json:"status"`
	LatestValidHash *common.Hash `json:"latestValidHash"`
	ValidationError *string      `json:"validationError"`
}

type TransitionConfigurationV1 struct {
	TerminalTotalDifficulty *hexutil.Big   `json:"terminalTotalDifficulty"`
	TerminalBlockHash       common.Hash    `json:"terminalBlockHash"`
	TerminalBlockNumber     hexutil.Uint64 `json:"terminalBlockNumber"`
}

// PayloadID is an identifier of the payload build process
type PayloadID [8]byte

func (b PayloadID) String() string {
	return hexutil.Encode(b[:])
}

func (b PayloadID) MarshalText() ([]byte, error) {
	return hexutil.Bytes(b[:]).MarshalText()
}

func (b *PayloadID) UnmarshalText(input []byte) error {
	err := hexutil.UnmarshalFixedText("PayloadID", input, b[:])
	if err != nil {
		return fmt.Errorf("invalid payload id %q: %w", input, err)
	}
	return nil
}

type ForkChoiceResponse struct {
	PayloadStatus PayloadStatusV1 `json:"payloadStatus"`
	PayloadID     *PayloadID      `json:"payloadId"`
}

type ForkchoiceStateV1 struct {
	HeadBlockHash      common.Hash `json:"headBlockHash"`
	SafeBlockHash      common.Hash `json:"safeBlockHash"`
	FinalizedBlockHash common.Hash `json:"finalizedBlockHash"`
}

func encodeTransactions(txs []*model.Transaction) [][]byte {
	var enc = make([][]byte, len(txs))
	for i, tx := range txs {
		enc[i], _ = tx.MarshalBinary()
	}
	return enc
}

func decodeTransactions(enc [][]byte) ([]*model.Transaction, error) {
	var txs = make([]*model.Transaction, len(enc))
	for i, encTx := range enc {
		var tx model.Transaction
		if err := tx.UnmarshalBinary(encTx); err != nil {
			return nil, fmt.Errorf("invalid transaction %d: %v", i, err)
		}
		txs[i] = &tx
	}
	return txs, nil
}

// ExecutableDataToBlock constructs a block from executable data.
// It verifies that the following fields:
// 		len(extraData) <= 32
// 		uncleHash = emptyUncleHash
// 		difficulty = 0
// and that the blockhash of the constructed block matches the parameters.
func ExecutableDataToBlock(params ExecutableDataV1) (*model.Block, error) {
	txs, err := decodeTransactions(params.Transactions)
	if err != nil {
		return nil, err
	}
	if len(params.ExtraData) > 32 {
		return nil, fmt.Errorf("invalid extradata length: %v", len(params.ExtraData))
	}
	if len(params.LogsBloom) != 256 {
		return nil, fmt.Errorf("invalid logsBloom length: %v", len(params.LogsBloom))
	}
	// Check that baseFeePerGas is not negative or too big
	if params.BaseFeePerGas != nil && (params.BaseFeePerGas.Sign() == -1 || params.BaseFeePerGas.BitLen() > 256) {
		return nil, fmt.Errorf("invalid baseFeePerGas: %v", params.BaseFeePerGas)
	}
	header := &model.Header{
		ParentHash:  params.ParentHash,
		UncleHash:   model.EmptyUncleHash,
		Coinbase:    params.FeeRecipient,
		Root:        params.StateRoot,
		TxHash:      model.DeriveSha(model.Transactions(txs), trie.NewStackTrie(nil)),
		ReceiptHash: params.ReceiptsRoot,
		Bloom:       model.BytesToBloom(params.LogsBloom),
		Difficulty:  common.Big0,
		Number:      new(big.Int).SetUint64(params.Number),
		GasLimit:    params.GasLimit,
		GasUsed:     params.GasUsed,
		Time:        params.Timestamp,
		BaseFee:     params.BaseFeePerGas,
		Extra:       params.ExtraData,
		MixDigest:   params.Random,
	}
	block := model.NewBlockWithHeader(header).WithBody(txs, nil /* uncles */)
	if block.Hash() != params.BlockHash {
		return nil, fmt.Errorf("blockhash mismatch, want %x, got %x", params.BlockHash, block.Hash())
	}
	return block, nil
}

// BlockToExecutableData constructs the executableDataV1 structure by filling the
// fields from the given block. It assumes the given block is post-merge block.
func BlockToExecutableData(block *model.Block) *ExecutableDataV1 {
	return &ExecutableDataV1{
		BlockHash:     block.Hash(),
		ParentHash:    block.ParentHash(),
		FeeRecipient:  block.Coinbase(),
		StateRoot:     block.Root(),
		Number:        block.NumberU64(),
		GasLimit:      block.GasLimit(),
		GasUsed:       block.GasUsed(),
		BaseFeePerGas: block.BaseFee(),
		Timestamp:     block.Time(),
		ReceiptsRoot:  block.ReceiptHash(),
		LogsBloom:     block.Bloom().Bytes(),
		Transactions:  encodeTransactions(block.Transactions()),
		Random:        block.MixDigest(),
		ExtraData:     block.Extra(),
	}
}
