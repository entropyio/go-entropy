package model

import (
	"encoding/binary"
	"fmt"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"io"
	"math/big"
	"reflect"
	"sync/atomic"
	"time"
)

var (
	EmptyRootHash  = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
	EmptyUncleHash = rlpHash([]*Header(nil))
)

// A BlockNonce is a 64-bit hash which proves (combined with the
// mix-hash) that a sufficient amount of computation has been carried
// out on a block.
type BlockNonce [8]byte

// EncodeNonce converts the given integer to a block nonce.
func EncodeNonce(i uint64) BlockNonce {
	var n BlockNonce
	binary.BigEndian.PutUint64(n[:], i)
	return n
}

// Uint64 returns the integer value of a block nonce.
func (nonce BlockNonce) Uint64() uint64 {
	return binary.BigEndian.Uint64(nonce[:])
}

// MarshalText encodes n as a hex string with 0x prefix.
func (nonce BlockNonce) MarshalText() ([]byte, error) {
	return hexutil.Bytes(nonce[:]).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (nonce *BlockNonce) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("BlockNonce", input, nonce[:])
}

//go:generate go run github.com/fjl/gencodec -type Header -field-override headerMarshaling -out gen_header_json.go
//go:generate go run ../../common/rlp/rlpgen -type Header -out gen_header_rlp.go

// Header represents a block header in the Entropy blockchain.
type Header struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	Coinbase    common.Address `json:"miner"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        uint64         `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    `json:"mixHash"`
	Nonce       BlockNonce     `json:"nonce"`
	BaseFee     *big.Int       `json:"baseFeePerGas" rlp:"optional"` // BaseFee was added by EIP-1559 and is ignored in legacy headers.
	// for claude dpos
	// Validator     common.Address     `json:"validator" gencodec:"required"`
	// ClaudeCtxHash *ClaudeContextHash `json:"ctxHash"  gencodec:"required"`
}

// field type overrides for gencodec
type headerMarshaling struct {
	Difficulty *hexutil.Big
	Number     *hexutil.Big
	GasLimit   hexutil.Uint64
	GasUsed    hexutil.Uint64
	Time       hexutil.Uint64
	Extra      hexutil.Bytes
	BaseFee    *hexutil.Big
	Hash       common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (header *Header) Hash() common.Hash {
	return rlpHash(header)
}

var headerSize = common.StorageSize(reflect.TypeOf(Header{}).Size())

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (header *Header) Size() common.StorageSize {
	return headerSize + common.StorageSize(len(header.Extra)+(header.Difficulty.BitLen()+header.Number.BitLen())/8)
}

// SanityCheck checks a few basic things -- these checks are way beyond what
// any 'sane' production values should hold, and can mainly be used to prevent
// that the unbounded fields are stuffed with junk data to add processing
// overhead
func (header *Header) SanityCheck() error {
	if header.Number != nil && !header.Number.IsUint64() {
		return fmt.Errorf("too large block number: bitlen %d", header.Number.BitLen())
	}
	if header.Difficulty != nil {
		if diffLen := header.Difficulty.BitLen(); diffLen > 80 {
			return fmt.Errorf("too large block difficulty: bitlen %d", diffLen)
		}
	}
	if eLen := len(header.Extra); eLen > 100*1024 {
		return fmt.Errorf("too large block extradata: size %d", eLen)
	}
	if header.BaseFee != nil {
		if bfLen := header.BaseFee.BitLen(); bfLen > 256 {
			return fmt.Errorf("too large base fee: bitlen %d", bfLen)
		}
	}
	return nil
}

// EmptyBody returns true if there is no additional 'body' to complete the header
// that is: no transactions and no uncles.
func (header *Header) EmptyBody() bool {
	return header.TxHash == EmptyRootHash && header.UncleHash == EmptyUncleHash
}

// EmptyReceipts returns true if there are no receipts for this header/block.
func (header *Header) EmptyReceipts() bool {
	return header.ReceiptHash == EmptyRootHash
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
type Body struct {
	Transactions []*Transaction
	Uncles       []*Header
}

// Block represents an entire block in the Entropy blockchain.
type Block struct {
	header       *Header
	uncles       []*Header
	transactions Transactions

	// caches
	hash atomic.Value
	size atomic.Value

	// These fields are used by package entropy to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}

	// ClaudeCtx *ClaudeContext
}

// "external" block encoding. used for eth protocol, etc.
type extblock struct {
	Header *Header
	Txs    []*Transaction
	Uncles []*Header
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewBlock(header *Header, txs []*Transaction, uncles []*Header, receipts []*Receipt, hasher TrieHasher) *Block {
	b := &Block{header: CopyHeader(header)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs), hasher)
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptHash = EmptyRootHash
	} else {
		b.header.ReceiptHash = DeriveSha(Receipts(receipts), hasher)
		b.header.Bloom = CreateBloom(receipts)
	}

	if len(uncles) == 0 {
		b.header.UncleHash = EmptyUncleHash
	} else {
		b.header.UncleHash = CalcUncleHash(uncles)
		b.uncles = make([]*Header, len(uncles))
		for i := range uncles {
			b.uncles[i] = CopyHeader(uncles[i])
		}
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {
	cpy := *h
	if cpy.Difficulty = new(big.Int); h.Difficulty != nil {
		cpy.Difficulty.Set(h.Difficulty)
	}
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if h.BaseFee != nil {
		cpy.BaseFee = new(big.Int).Set(h.BaseFee)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}

// DecodeRLP decodes the Entropy
func (block *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	block.header, block.uncles, block.transactions = eb.Header, eb.Uncles, eb.Txs
	block.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Entropy RLP block format.
func (block *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: block.header,
		Txs:    block.transactions,
		Uncles: block.uncles,
	})
}

func (block *Block) Uncles() []*Header          { return block.uncles }
func (block *Block) Transactions() Transactions { return block.transactions }
func (block *Block) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range block.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}
func (block *Block) Number() *big.Int         { return new(big.Int).Set(block.header.Number) }
func (block *Block) GasLimit() uint64         { return block.header.GasLimit }
func (block *Block) GasUsed() uint64          { return block.header.GasUsed }
func (block *Block) Difficulty() *big.Int     { return new(big.Int).Set(block.header.Difficulty) }
func (block *Block) Time() uint64             { return block.header.Time }
func (block *Block) NumberU64() uint64        { return block.header.Number.Uint64() }
func (block *Block) MixDigest() common.Hash   { return block.header.MixDigest }
func (block *Block) Nonce() uint64            { return binary.BigEndian.Uint64(block.header.Nonce[:]) }
func (block *Block) Bloom() Bloom             { return block.header.Bloom }
func (block *Block) Coinbase() common.Address { return block.header.Coinbase }
func (block *Block) Root() common.Hash        { return block.header.Root }
func (block *Block) ParentHash() common.Hash  { return block.header.ParentHash }
func (block *Block) TxHash() common.Hash      { return block.header.TxHash }
func (block *Block) ReceiptHash() common.Hash { return block.header.ReceiptHash }
func (block *Block) UncleHash() common.Hash   { return block.header.UncleHash }
func (block *Block) Extra() []byte            { return common.CopyBytes(block.header.Extra) }
func (block *Block) BaseFee() *big.Int {
	if block.header.BaseFee == nil {
		return nil
	}
	return new(big.Int).Set(block.header.BaseFee)
}

//func (b *Block) Validator() common.Address     { return b.header.Validator }
//func (b *Block) ClaudeContext() *ClaudeContext { return b.ClaudeCtx }

func (block *Block) Header() *Header { return CopyHeader(block.header) }

// Body returns the non-header content of the block.
func (block *Block) Body() *Body {
	return &Body{block.transactions, block.uncles}
}

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (block *Block) Size() common.StorageSize {
	if size := block.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	_ = rlp.Encode(&c, block)
	block.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

// SanityCheck can be used to prevent that unbounded fields are
// stuffed with junk data to add processing overhead
func (block *Block) SanityCheck() error {
	return block.header.SanityCheck()
}

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

func CalcUncleHash(uncles []*Header) common.Hash {
	if len(uncles) == 0 {
		return EmptyUncleHash
	}
	return rlpHash(uncles)
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
func (block *Block) WithSeal(header *Header) *Block {
	cpy := *header
	return &Block{
		header:       &cpy,
		transactions: block.transactions,
		uncles:       block.uncles,
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
func (block *Block) WithBody(transactions []*Transaction, uncles []*Header) *Block {
	newBlock := &Block{
		header:       CopyHeader(block.header),
		transactions: make([]*Transaction, len(transactions)),
		uncles:       make([]*Header, len(uncles)),
	}
	copy(newBlock.transactions, transactions)
	for i := range uncles {
		newBlock.uncles[i] = CopyHeader(uncles[i])
	}
	return newBlock
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (block *Block) Hash() common.Hash {
	if hash := block.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := block.header.Hash()
	block.hash.Store(v)
	return v
}

type Blocks []*Block

// HeaderParentHashFromRLP returns the parentHash of an RLP-encoded
// header. If 'header' is invalid, the zero hash is returned.
func HeaderParentHashFromRLP(header []byte) common.Hash {
	// parentHash is the first list element.
	listContent, _, err := rlp.SplitList(header)
	if err != nil {
		return common.Hash{}
	}
	parentHash, _, err := rlp.SplitString(listContent)
	if err != nil {
		return common.Hash{}
	}
	if len(parentHash) != 32 {
		return common.Hash{}
	}
	return common.BytesToHash(parentHash)
}
