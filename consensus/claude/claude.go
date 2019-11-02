package claude

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/entropyio/go-entropy/accounts"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/trie"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/rpc"
	"github.com/hashicorp/golang-lru"
	"golang.org/x/crypto/sha3"
)

const (
	extraVanity        = 32   // Fixed number of extra-data prefix bytes reserved for signer vanity
	extraSeal          = 65   // Fixed number of extra-data suffix bytes reserved for signer seal
	inmemorySignatures = 4096 // Number of recent block signatures to keep in memory

	blockInterval    = uint64(10)
	epochInterval    = uint64(86400)
	maxValidatorSize = 21
	safeSize         = maxValidatorSize*2/3 + 1
	consensusSize    = maxValidatorSize*2/3 + 1
)

var log = logger.NewLogger("[claude]")

var (
	frontierBlockReward  = big.NewInt(5e+18) // Block reward in wei for successfully mining a block
	byzantiumBlockReward = big.NewInt(3e+18) // Block reward in wei for successfully mining a block upward from Byzantium

	timeOfFirstBlock = uint64(0)

	confirmedBlockHead = []byte("confirmed-block-head")
)

var (
	// errUnknownBlock is returned when the list of signers is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")
	// errMissingVanity is returned if a block's extra-data section is shorter than
	// 32 bytes, which is required to store the signer vanity.
	errMissingVanity = errors.New("extra-data 32 byte vanity prefix missing")
	// errMissingSignature is returned if a block's extra-data section doesn't seem
	// to contain a 65 byte secp256k1 signature.
	errMissingSignature = errors.New("extra-data 65 byte suffix signature missing")
	// errInvalidMixDigest is returned if a block's mix digest is non-zero.
	errInvalidMixDigest = errors.New("non-zero mix digest")
	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash  = errors.New("non empty uncle hash")
	errInvalidDifficulty = errors.New("invalid difficulty")

	// ErrInvalidTimestamp is returned if the timestamp of a block is lower than
	// the previous block's timestamp + the minimum block period.
	ErrInvalidTimestamp           = errors.New("invalid timestamp")
	ErrWaitForPrevBlock           = errors.New("wait for last block arrived")
	ErrMintFutureBlock            = errors.New("mint the future block")
	ErrMismatchSignerAndValidator = errors.New("mismatch block signer and validator")
	ErrInvalidBlockValidator      = errors.New("invalid block validator")
	ErrInvalidMintBlockTime       = errors.New("invalid time to mint the block")
	ErrNilBlockHeader             = errors.New("nil block header returned")
)
var (
	uncleHash = model.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
)

type Claude struct {
	config *config.ClaudeConfig // Consensus engine configuration parameters
	db     database.Database    // Database to store and retrieve snapshot checkpoints

	signer               common.Address
	signFn               SignerFn
	signatures           *lru.ARCCache // Signatures of recent blocks to speed up mining
	confirmedBlockHeader *model.Header

	mu   sync.RWMutex
	stop chan bool
}

type SignerFn func(accounts.Account, []byte) ([]byte, error)

// Fixme: dpos共识算法修改了header，会导致区块和pow，pos不一致。需要在不改变区块的情况下支持dpos

// NOTE: sigHash was copy from clique
// sigHash returns the hash which is used as input for the proof-of-authority
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *model.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()

	rlputil.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[:len(header.Extra)-65], // Yes, this will panic if extra is too short
		header.MixDigest,
		header.Nonce,

		header.Validator,
		header.ClaudeCtxHash.Root(),
	})
	hasher.Sum(hash[:0])
	return hash
}

func New(config *config.ClaudeConfig, db database.Database) *Claude {
	signatures, _ := lru.NewARC(inmemorySignatures)
	return &Claude{
		config:     config,
		db:         db,
		signatures: signatures,
	}
}

func (claude *Claude) Author(header *model.Header) (common.Address, error) {
	return header.Validator, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (claude *Claude) VerifyHeader(chain consensus.ChainReader, header *model.Header, seal bool) error {
	return claude.verifyHeader(chain, header, nil)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers. The
// method returns a quit channel to abort the operations and a results channel to
// retrieve the async verifications (the order is that of the input slice).
func (claude *Claude) VerifyHeaders(chain consensus.ChainReader, headers []*model.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for i, header := range headers {
			err := claude.verifyHeader(chain, header, headers[:i])
			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (claude *Claude) verifyHeader(chain consensus.ChainReader, header *model.Header, parents []*model.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}
	number := header.Number.Uint64()
	// Unnecssary to verify the block from feature
	if header.Time > uint64(time.Now().Unix()) {
		return consensus.ErrFutureBlock
	}
	// Check that the extra-data contains both the vanity and signature
	if len(header.Extra) < extraVanity {
		return errMissingVanity
	}
	if len(header.Extra) < extraVanity+extraSeal {
		return errMissingSignature
	}
	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != (common.Hash{}) {
		return errInvalidMixDigest
	}
	// Difficulty always 1
	if header.Difficulty.Uint64() != 1 {
		return errInvalidDifficulty
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in DPoS
	if header.UncleHash != uncleHash {
		return errInvalidUncleHash
	}

	var parent *model.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	if parent.Time+uint64(blockInterval) > header.Time {
		return ErrInvalidTimestamp
	}
	return nil
}

// VerifyUncles implements consensus.Engine, always returning an error for any
// uncles as this consensus mechanism doesn't permit uncles.
func (claude *Claude) VerifyUncles(chain consensus.ChainReader, block *model.Block) error {
	if len(block.Uncles()) > 0 {
		return errors.New("uncles not allowed")
	}
	return nil
}

// VerifySeal implements consensus.Engine, checking whether the signature contained
// in the header satisfies the consensus protocol requirements.
func (claude *Claude) VerifySeal(chain consensus.ChainReader, header *model.Header) error {
	return claude.verifySeal(chain, header, nil)
}

// verifySeal checks whether the signature contained in the header satisfies the
// consensus protocol requirements. The method accepts an optional list of parent
// headers that aren't yet part of the local blockchain to generate the snapshots
// from.
func (claude *Claude) verifySeal(chain consensus.ChainReader, header *model.Header, parents []*model.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	var parent *model.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	triedb := *trie.NewDatabase(claude.db)
	claudeContext, err := model.NewClaudeContextFromHash(triedb, parent.ClaudeCtxHash)
	if err != nil {
		return err
	}
	epochContext := &EpochContext{ClaudeContext: claudeContext}
	validator, err := epochContext.lookupValidator(header.Time)
	if err != nil {
		return err
	}
	if err := claude.verifyBlockSigner(validator, header); err != nil {
		return err
	}
	return claude.updateConfirmedBlockHeader(chain)
}

func (claude *Claude) verifyBlockSigner(validator common.Address, header *model.Header) error {
	signer, err := ecrecover(header, claude.signatures)
	if err != nil {
		return err
	}
	if bytes.Compare(signer.Bytes(), validator.Bytes()) != 0 {
		return ErrInvalidBlockValidator
	}
	if bytes.Compare(signer.Bytes(), header.Validator.Bytes()) != 0 {
		return ErrMismatchSignerAndValidator
	}
	return nil
}

func (claude *Claude) updateConfirmedBlockHeader(chain consensus.ChainReader) error {
	if claude.confirmedBlockHeader == nil {
		header, err := claude.loadConfirmedBlockHeader(chain)
		if err != nil {
			header = chain.GetHeaderByNumber(0)
			if header == nil {
				return err
			}
		}
		claude.confirmedBlockHeader = header
	}

	curHeader := chain.CurrentHeader()
	epoch := int64(-1)
	validatorMap := make(map[common.Address]bool)
	for claude.confirmedBlockHeader.Hash() != curHeader.Hash() &&
		claude.confirmedBlockHeader.Number.Uint64() < curHeader.Number.Uint64() {
		curEpoch := (int64)(curHeader.Time / epochInterval)
		if curEpoch != epoch {
			epoch = curEpoch
			validatorMap = make(map[common.Address]bool)
		}
		// fast return
		// if block number difference less consensusSize-witnessNum
		// there is no need to check block is confirmed
		if curHeader.Number.Int64()-claude.confirmedBlockHeader.Number.Int64() < int64(consensusSize-len(validatorMap)) {
			log.Debug("Claude fast return", "current", curHeader.Number.String(), "confirmed", claude.confirmedBlockHeader.Number.String(), "witnessCount", len(validatorMap))
			return nil
		}
		validatorMap[curHeader.Validator] = true
		if len(validatorMap) >= consensusSize {
			claude.confirmedBlockHeader = curHeader
			if err := claude.storeConfirmedBlockHeader(claude.db); err != nil {
				return err
			}
			log.Debug("claude set confirmed block header success", "currentHeader", curHeader.Number.String())
			return nil
		}
		curHeader = chain.GetHeaderByHash(curHeader.ParentHash)
		if curHeader == nil {
			return ErrNilBlockHeader
		}
	}
	return nil
}

func (claude *Claude) loadConfirmedBlockHeader(chain consensus.ChainReader) (*model.Header, error) {
	key, err := claude.db.Get(confirmedBlockHead)
	if err != nil {
		return nil, err
	}
	header := chain.GetHeaderByHash(common.BytesToHash(key))
	if header == nil {
		return nil, ErrNilBlockHeader
	}
	return header, nil
}

// store inserts the snapshot into the database.
func (claude *Claude) storeConfirmedBlockHeader(db database.Database) error {
	return db.Put(confirmedBlockHead, claude.confirmedBlockHeader.Hash().Bytes())
}

func (claude *Claude) Prepare(chain consensus.ChainReader, header *model.Header) error {
	header.Nonce = model.BlockNonce{}
	number := header.Number.Uint64()
	if len(header.Extra) < extraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, extraVanity-len(header.Extra))...)
	}
	header.Extra = header.Extra[:extraVanity]
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = claude.CalcDifficulty(chain, header.Time, parent)
	header.Validator = claude.signer
	return nil
}

func AccumulateRewards(config *config.ChainConfig, state *state.StateDB, header *model.Header, uncles []*model.Header) {
	// Select the correct block reward based on chain progression
	blockReward := frontierBlockReward
	if config.IsHomestead(header.Number) {
		blockReward = byzantiumBlockReward
	}
	// Accumulate the rewards for the miner and any included uncles
	reward := new(big.Int).Set(blockReward)
	state.AddBalance(header.Coinbase, reward)

	log.Debugf("AccumulateRewards address=%x, reward=%d, uncles=%v", uncles)
}

// Finalize implements consensus.Engine, ensuring no uncles are set, nor block
// rewards given.
func (clique *Claude) Finalize(chain consensus.ChainReader, header *model.Header, state *state.StateDB, txs []*model.Transaction, uncles []*model.Header) {
	// No block rewards in PoA, so the state remains as is and uncles are dropped
	header.Root = state.IntermediateRoot(chain.Config().IsHomestead(header.Number))
	header.UncleHash = model.CalcUncleHash(nil)
}

// FinalizeAndAssemble implements consensus.Engine, ensuring no uncles are set,
// nor block rewards given, and returns the final block.
func (claude *Claude) FinalizeAndAssemble(chain consensus.ChainReader, header *model.Header, state *state.StateDB, txs []*model.Transaction, uncles []*model.Header, receipts []*model.Receipt, claudeContext *model.ClaudeContext) (*model.Block, error) {
	// Accumulate block rewards and commit the final state root
	AccumulateRewards(chain.Config(), state, header, uncles)
	header.Root = state.IntermediateRoot(chain.Config().IsHomestead(header.Number))

	parent := chain.GetHeaderByHash(header.ParentHash)
	epochContext := &EpochContext{
		statedb:       state,
		ClaudeContext: claudeContext,
		TimeStamp:     header.Time,
	}
	if timeOfFirstBlock == 0 {
		if firstBlockHeader := chain.GetHeaderByNumber(1); firstBlockHeader != nil {
			timeOfFirstBlock = firstBlockHeader.Time
		}
	}
	genesis := chain.GetHeaderByNumber(0)
	err := epochContext.tryElect(genesis, parent)
	if err != nil {
		return nil, fmt.Errorf("got error when elect next epoch, err: %s", err)
	}

	//update mint count trie
	updateMintCnt(parent.Time, header.Time, header.Validator, claudeContext)
	header.ClaudeCtxHash = claudeContext.ToHash()
	return model.NewBlock(header, txs, uncles, receipts), nil
}

func (claude *Claude) checkDeadline(lastBlock *model.Block, now uint64) error {
	prevSlot := PrevSlot(now)
	nextSlot := NextSlot(now)
	if lastBlock.Time() >= nextSlot {
		return ErrMintFutureBlock
	}
	// last block was arrived, or time's up
	if lastBlock.Time() == prevSlot || nextSlot-now <= 1 {
		return nil
	}
	return ErrWaitForPrevBlock
}

func (claude *Claude) CheckValidator(lastBlock *model.Block, now uint64) error {
	if err := claude.checkDeadline(lastBlock, now); err != nil {
		return err
	}
	triedb := *trie.NewDatabase(claude.db)
	claudeContext, err := model.NewClaudeContextFromHash(triedb, lastBlock.Header().ClaudeCtxHash)
	if err != nil {
		return err
	}
	epochContext := &EpochContext{ClaudeContext: claudeContext}
	validator, err := epochContext.lookupValidator(now)
	log.Debug("claude CheckValidator", validator)
	if err != nil {
		return err
	}
	//if (validator == common.Address{}) || bytes.Compare(validator.Bytes(), claude.signer.Bytes()) != 0 {
	//	return ErrInvalidBlockValidator
	//}
	return nil
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (claude *Claude) Seal(chain consensus.ChainReader, block *model.Block, results chan<- *model.Block, stop <-chan struct{}) error {
	header := block.Header()
	number := header.Number.Uint64()
	// Sealing the genesis block is not supported
	if number == 0 {
		return errUnknownBlock
	}
	now := (uint64)(time.Now().Unix())
	delay := NextSlot(now) - now
	if delay > 0 {
		select {
		case <-stop:
			return nil
		case <-time.After(time.Duration(delay) * time.Second):
		}
	}
	block.Header().Time = (uint64)(time.Now().Unix())

	// time's up, sign the block
	sighash, err := claude.signFn(accounts.Account{Address: claude.signer}, sigHash(header).Bytes())
	if err != nil {
		return err
	}
	copy(header.Extra[len(header.Extra)-extraSeal:], sighash)
	// Wait until sealing is terminated or delay timeout.
	log.Debug("Waiting for slot to sign and propagate", "delay", common.PrettyDuration(delay))
	go func() {
		//select {
		//case <-stop:
		//	return
		//case <-time.After(delay):
		//}

		select {
		case results <- block.WithSeal(header):
		default:
			log.Debug("Sealing result is not read by miner", "sealhash", claude.SealHash(header))
		}
	}()

	return nil
}

func (claude *Claude) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *model.Header) *big.Int {
	return big.NewInt(1)
}

// SealHash returns the hash of a block prior to it being sealed.
func (claude *Claude) SealHash(header *model.Header) common.Hash {
	return sigHash(header)
}

// Close implements consensus.Engine. It's a noop for clique as there are no background threads.
func (claude *Claude) Close() error {
	return nil
}

func (claude *Claude) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "claude",
		Version:   "1.0",
		Service:   &API{chain: chain, claude: claude},
		Public:    true,
	}}
}

func (claude *Claude) Authorize(signer common.Address, signFn SignerFn) {
	claude.mu.Lock()
	claude.signer = signer
	claude.signFn = signFn
	claude.mu.Unlock()
}

// ecrecover extracts the Entropy account address from a signed header.
func ecrecover(header *model.Header, sigcache *lru.ARCCache) (common.Address, error) {
	// If the signature's already cached, return that
	hash := header.Hash()
	if address, known := sigcache.Get(hash); known {
		return address.(common.Address), nil
	}
	// Retrieve the signature from the header extra-data
	if len(header.Extra) < extraSeal {
		return common.Address{}, errMissingSignature
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]
	// Recover the public key and the Entropy address
	pubkey, err := crypto.Ecrecover(sigHash(header).Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])
	sigcache.Add(hash, signer)
	return signer, nil
}

func PrevSlot(now uint64) uint64 {
	return uint64((now-1)/blockInterval) * blockInterval
}

func NextSlot(now uint64) uint64 {
	return uint64((now+blockInterval-1)/blockInterval) * blockInterval
}

// update counts in MintCntTrie for the miner of newBlock
func updateMintCnt(parentBlockTime, currentBlockTime uint64, validator common.Address, claudeContext *model.ClaudeContext) {
	currentMintCntTrie := claudeContext.MintCntTrie()
	currentEpoch := parentBlockTime / epochInterval
	currentEpochBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(currentEpochBytes, uint64(currentEpoch))

	cnt := int64(1)
	newEpoch := currentBlockTime / epochInterval
	// still during the currentEpochID
	if currentEpoch == newEpoch {
		iter := trie.NewIterator(currentMintCntTrie.NodeIterator(currentEpochBytes))

		// when current is not genesis, read last count from the MintCntTrie
		if iter.Next() {
			cntBytes := currentMintCntTrie.Get(append(currentEpochBytes, validator.Bytes()...))

			// not the first time to mint
			if cntBytes != nil {
				cnt = int64(binary.BigEndian.Uint64(cntBytes)) + 1
			}
		}
	}

	newCntBytes := make([]byte, 8)
	newEpochBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(newEpochBytes, uint64(newEpoch))
	binary.BigEndian.PutUint64(newCntBytes, uint64(cnt))
	claudeContext.MintCntTrie().TryUpdate(append(newEpochBytes, validator.Bytes()...), newCntBytes)
}
