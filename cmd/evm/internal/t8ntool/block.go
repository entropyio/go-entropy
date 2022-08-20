package t8ntool

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/mathutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/consensus/clique"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/urfave/cli/v2"
	"math/big"
	"os"
)

//go:generate go run github.com/fjl/gencodec -type header -field-override headerMarshaling -out gen_header.go
type header struct {
	ParentHash  common.Hash       `json:"parentHash"`
	OmmerHash   *common.Hash      `json:"sha3Uncles"`
	Coinbase    *common.Address   `json:"miner"`
	Root        common.Hash       `json:"stateRoot"        gencodec:"required"`
	TxHash      *common.Hash      `json:"transactionsRoot"`
	ReceiptHash *common.Hash      `json:"receiptsRoot"`
	Bloom       model.Bloom       `json:"logsBloom"`
	Difficulty  *big.Int          `json:"difficulty"`
	Number      *big.Int          `json:"number"           gencodec:"required"`
	GasLimit    uint64            `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64            `json:"gasUsed"`
	Time        uint64            `json:"timestamp"        gencodec:"required"`
	Extra       []byte            `json:"extraData"`
	MixDigest   common.Hash       `json:"mixHash"`
	Nonce       *model.BlockNonce `json:"nonce"`
	BaseFee     *big.Int          `json:"baseFeePerGas" rlp:"optional"`
}

type headerMarshaling struct {
	Difficulty *mathutil.HexOrDecimal256
	Number     *mathutil.HexOrDecimal256
	GasLimit   mathutil.HexOrDecimal64
	GasUsed    mathutil.HexOrDecimal64
	Time       mathutil.HexOrDecimal64
	Extra      hexutil.Bytes
	BaseFee    *mathutil.HexOrDecimal256
}

type bbInput struct {
	Header    *header      `json:"header,omitempty"`
	OmmersRlp []string     `json:"ommers,omitempty"`
	TxRlp     string       `json:"txs,omitempty"`
	Clique    *cliqueInput `json:"clique,omitempty"`

	Ethash    bool                 `json:"-"`
	EthashDir string               `json:"-"`
	PowMode   ethash.Mode          `json:"-"`
	Txs       []*model.Transaction `json:"-"`
	Ommers    []*model.Header      `json:"-"`
}

type cliqueInput struct {
	Key       *ecdsa.PrivateKey
	Voted     *common.Address
	Authorize *bool
	Vanity    common.Hash
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (c *cliqueInput) UnmarshalJSON(input []byte) error {
	var x struct {
		Key       *common.Hash    `json:"secretKey"`
		Voted     *common.Address `json:"voted"`
		Authorize *bool           `json:"authorize"`
		Vanity    common.Hash     `json:"vanity"`
	}
	if err := json.Unmarshal(input, &x); err != nil {
		return err
	}
	if x.Key == nil {
		return errors.New("missing required field 'secretKey' for cliqueInput")
	}
	if ecdsaKey, err := crypto.ToECDSA(x.Key[:]); err != nil {
		return err
	} else {
		c.Key = ecdsaKey
	}
	c.Voted = x.Voted
	c.Authorize = x.Authorize
	c.Vanity = x.Vanity
	return nil
}

// ToBlock converts i into a *model.Block
func (i *bbInput) ToBlock() *model.Block {
	header := &model.Header{
		ParentHash:  i.Header.ParentHash,
		UncleHash:   model.EmptyUncleHash,
		Coinbase:    common.Address{},
		Root:        i.Header.Root,
		TxHash:      model.EmptyRootHash,
		ReceiptHash: model.EmptyRootHash,
		Bloom:       i.Header.Bloom,
		Difficulty:  common.Big0,
		Number:      i.Header.Number,
		GasLimit:    i.Header.GasLimit,
		GasUsed:     i.Header.GasUsed,
		Time:        i.Header.Time,
		Extra:       i.Header.Extra,
		MixDigest:   i.Header.MixDigest,
		BaseFee:     i.Header.BaseFee,
	}

	// Fill optional values.
	if i.Header.OmmerHash != nil {
		header.UncleHash = *i.Header.OmmerHash
	} else if len(i.Ommers) != 0 {
		// Calculate the ommer hash if none is provided and there are ommers to hash
		header.UncleHash = model.CalcUncleHash(i.Ommers)
	}
	if i.Header.Coinbase != nil {
		header.Coinbase = *i.Header.Coinbase
	}
	if i.Header.TxHash != nil {
		header.TxHash = *i.Header.TxHash
	}
	if i.Header.ReceiptHash != nil {
		header.ReceiptHash = *i.Header.ReceiptHash
	}
	if i.Header.Nonce != nil {
		header.Nonce = *i.Header.Nonce
	}
	if header.Difficulty != nil {
		header.Difficulty = i.Header.Difficulty
	}
	return model.NewBlockWithHeader(header).WithBody(i.Txs, i.Ommers)
}

// SealBlock seals the given block using the configured engine.
func (i *bbInput) SealBlock(block *model.Block) (*model.Block, error) {
	switch {
	case i.Ethash:
		return i.sealEthash(block)
	case i.Clique != nil:
		return i.sealClique(block)
	default:
		return block, nil
	}
}

// sealEthash seals the given block using ethash.
func (i *bbInput) sealEthash(block *model.Block) (*model.Block, error) {
	if i.Header.Nonce != nil {
		return nil, NewError(ErrorConfig, fmt.Errorf("sealing with ethash will overwrite provided nonce"))
	}
	ethashConfig := ethash.Config{
		PowMode:        i.PowMode,
		DatasetDir:     i.EthashDir,
		CacheDir:       i.EthashDir,
		DatasetsInMem:  1,
		DatasetsOnDisk: 2,
		CachesInMem:    2,
		CachesOnDisk:   3,
	}
	engine := ethash.New(ethashConfig, nil, true)
	defer engine.Close()
	// Use a buffered chan for results.
	// If the testmode is used, the sealer will return quickly, and complain
	// "Sealing result is not read by miner" if it cannot write the result.
	results := make(chan *model.Block, 1)
	if err := engine.Seal(nil, block, results, nil); err != nil {
		panic(fmt.Sprintf("failed to seal block: %v", err))
	}
	found := <-results
	return block.WithSeal(found.Header()), nil
}

// sealClique seals the given block using clique.
func (i *bbInput) sealClique(block *model.Block) (*model.Block, error) {
	// If any clique value overwrites an explicit header value, fail
	// to avoid silently building a block with unexpected values.
	if i.Header.Extra != nil {
		return nil, NewError(ErrorConfig, fmt.Errorf("sealing with clique will overwrite provided extra data"))
	}
	header := block.Header()
	if i.Clique.Voted != nil {
		if i.Header.Coinbase != nil {
			return nil, NewError(ErrorConfig, fmt.Errorf("sealing with clique and voting will overwrite provided coinbase"))
		}
		header.Coinbase = *i.Clique.Voted
	}
	if i.Clique.Authorize != nil {
		if i.Header.Nonce != nil {
			return nil, NewError(ErrorConfig, fmt.Errorf("sealing with clique and voting will overwrite provided nonce"))
		}
		if *i.Clique.Authorize {
			header.Nonce = [8]byte{}
		} else {
			header.Nonce = [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		}
	}
	// Extra is fixed 32 byte vanity and 65 byte signature
	header.Extra = make([]byte, 32+65)
	copy(header.Extra[0:32], i.Clique.Vanity.Bytes()[:])

	// Sign the seal hash and fill in the rest of the extra data
	h := clique.SealHash(header)
	sighash, err := crypto.Sign(h[:], i.Clique.Key)
	if err != nil {
		return nil, err
	}
	copy(header.Extra[32:], sighash)
	block = block.WithSeal(header)
	return block, nil
}

// BuildBlock constructs a block from the given inputs.
func BuildBlock(ctx *cli.Context) error {
	baseDir, err := createBasedir(ctx)
	if err != nil {
		return NewError(ErrorIO, fmt.Errorf("failed creating output basedir: %v", err))
	}
	inputData, err := readInput(ctx)
	if err != nil {
		return err
	}
	block := inputData.ToBlock()
	block, err = inputData.SealBlock(block)
	if err != nil {
		return err
	}
	return dispatchBlock(ctx, baseDir, block)
}

func readInput(ctx *cli.Context) (*bbInput, error) {
	var (
		headerStr  = ctx.String(InputHeaderFlag.Name)
		ommersStr  = ctx.String(InputOmmersFlag.Name)
		txsStr     = ctx.String(InputTxsRlpFlag.Name)
		cliqueStr  = ctx.String(SealCliqueFlag.Name)
		ethashOn   = ctx.Bool(SealEthashFlag.Name)
		ethashDir  = ctx.String(SealEthashDirFlag.Name)
		ethashMode = ctx.String(SealEthashModeFlag.Name)
		inputData  = &bbInput{}
	)
	if ethashOn && cliqueStr != "" {
		return nil, NewError(ErrorConfig, fmt.Errorf("both ethash and clique sealing specified, only one may be chosen"))
	}
	if ethashOn {
		inputData.Ethash = ethashOn
		inputData.EthashDir = ethashDir
		switch ethashMode {
		case "normal":
			inputData.PowMode = ethash.ModeNormal
		case "test":
			inputData.PowMode = ethash.ModeTest
		case "fake":
			inputData.PowMode = ethash.ModeFake
		default:
			return nil, NewError(ErrorConfig, fmt.Errorf("unknown pow mode: %s, supported modes: test, fake, normal", ethashMode))
		}
	}
	if headerStr == stdinSelector || ommersStr == stdinSelector || txsStr == stdinSelector || cliqueStr == stdinSelector {
		decoder := json.NewDecoder(os.Stdin)
		if err := decoder.Decode(inputData); err != nil {
			return nil, NewError(ErrorJson, fmt.Errorf("failed unmarshaling stdin: %v", err))
		}
	}
	if cliqueStr != stdinSelector && cliqueStr != "" {
		var clique cliqueInput
		if err := readFile(cliqueStr, "clique", &clique); err != nil {
			return nil, err
		}
		inputData.Clique = &clique
	}
	if headerStr != stdinSelector {
		var env header
		if err := readFile(headerStr, "header", &env); err != nil {
			return nil, err
		}
		inputData.Header = &env
	}
	if ommersStr != stdinSelector && ommersStr != "" {
		var ommers []string
		if err := readFile(ommersStr, "ommers", &ommers); err != nil {
			return nil, err
		}
		inputData.OmmersRlp = ommers
	}
	if txsStr != stdinSelector {
		var txs string
		if err := readFile(txsStr, "txs", &txs); err != nil {
			return nil, err
		}
		inputData.TxRlp = txs
	}
	// Deserialize rlp txs and ommers
	var (
		ommers = []*model.Header{}
		txs    = []*model.Transaction{}
	)
	if inputData.TxRlp != "" {
		if err := rlp.DecodeBytes(common.FromHex(inputData.TxRlp), &txs); err != nil {
			return nil, NewError(ErrorRlp, fmt.Errorf("unable to decode transaction from rlp data: %v", err))
		}
		inputData.Txs = txs
	}
	for _, str := range inputData.OmmersRlp {
		type extblock struct {
			Header *model.Header
			Txs    []*model.Transaction
			Ommers []*model.Header
		}
		var ommer *extblock
		if err := rlp.DecodeBytes(common.FromHex(str), &ommer); err != nil {
			return nil, NewError(ErrorRlp, fmt.Errorf("unable to decode ommer from rlp data: %v", err))
		}
		ommers = append(ommers, ommer.Header)
	}
	inputData.Ommers = ommers

	return inputData, nil
}

// dispatchOutput writes the output data to either stderr or stdout, or to the specified
// files
func dispatchBlock(ctx *cli.Context, baseDir string, block *model.Block) error {
	raw, _ := rlp.EncodeToBytes(block)

	type blockInfo struct {
		Rlp  hexutil.Bytes `json:"rlp"`
		Hash common.Hash   `json:"hash"`
	}
	var enc blockInfo
	enc.Rlp = raw
	enc.Hash = block.Hash()

	b, err := json.MarshalIndent(enc, "", "  ")
	if err != nil {
		return NewError(ErrorJson, fmt.Errorf("failed marshalling output: %v", err))
	}
	switch dest := ctx.String(OutputBlockFlag.Name); dest {
	case "stdout":
		os.Stdout.Write(b)
		os.Stdout.WriteString("\n")
	case "stderr":
		os.Stderr.Write(b)
		os.Stderr.WriteString("\n")
	default:
		if err := saveFile(baseDir, dest, enc); err != nil {
			return err
		}
	}
	return nil
}