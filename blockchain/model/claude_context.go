package model

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/database/trie"
	"golang.org/x/crypto/sha3"
)

// claude dpos context tire
type ClaudeContext struct {
	epochTrie     *trie.Trie
	delegateTrie  *trie.Trie
	voteTrie      *trie.Trie
	candidateTrie *trie.Trie
	mintCntTrie   *trie.Trie

	db trie.TrieDatabase
}

var (
	epochPrefix     = []byte("epoch-")
	delegatePrefix  = []byte("delegate-")
	votePrefix      = []byte("vote-")
	candidatePrefix = []byte("candidate-")
	mintCntPrefix   = []byte("mintCnt-")
)

func NewEpochTrie(root common.Hash, db trie.TrieDatabase) (*trie.Trie, error) {
	return trie.New(root, &db)
}

func NewDelegateTrie(root common.Hash, db trie.TrieDatabase) (*trie.Trie, error) {
	return trie.New(root, &db)
}

func NewVoteTrie(root common.Hash, db trie.TrieDatabase) (*trie.Trie, error) {
	return trie.New(root, &db)
}

func NewCandidateTrie(root common.Hash, db trie.TrieDatabase) (*trie.Trie, error) {
	return trie.New(root, &db)
}

func NewMintCntTrie(root common.Hash, db trie.TrieDatabase) (*trie.Trie, error) {
	return trie.New(root, &db)
}

func NewClaudeContext(db trie.TrieDatabase) (*ClaudeContext, error) {
	epochTrie, err := NewEpochTrie(common.Hash{}, db)
	if err != nil {
		return nil, err
	}
	delegateTrie, err := NewDelegateTrie(common.Hash{}, db)
	if err != nil {
		return nil, err
	}
	voteTrie, err := NewVoteTrie(common.Hash{}, db)
	if err != nil {
		return nil, err
	}
	candidateTrie, err := NewCandidateTrie(common.Hash{}, db)
	if err != nil {
		return nil, err
	}
	mintCntTrie, err := NewMintCntTrie(common.Hash{}, db)
	if err != nil {
		return nil, err
	}
	return &ClaudeContext{
		epochTrie:     epochTrie,
		delegateTrie:  delegateTrie,
		voteTrie:      voteTrie,
		candidateTrie: candidateTrie,
		mintCntTrie:   mintCntTrie,
		db:            db,
	}, nil
}

func NewClaudeContextFromHash(db trie.TrieDatabase, ctxHash *ClaudeContextHash) (*ClaudeContext, error) {
	epochTrie, err := NewEpochTrie(ctxHash.EpochHash, db)
	if err != nil {
		return nil, err
	}
	delegateTrie, err := NewDelegateTrie(ctxHash.DelegateHash, db)
	if err != nil {
		return nil, err
	}
	voteTrie, err := NewVoteTrie(ctxHash.VoteHash, db)
	if err != nil {
		return nil, err
	}
	candidateTrie, err := NewCandidateTrie(ctxHash.CandidateHash, db)
	if err != nil {
		return nil, err
	}
	mintCntTrie, err := NewMintCntTrie(ctxHash.MintCntHash, db)
	if err != nil {
		return nil, err
	}
	return &ClaudeContext{
		epochTrie:     epochTrie,
		delegateTrie:  delegateTrie,
		voteTrie:      voteTrie,
		candidateTrie: candidateTrie,
		mintCntTrie:   mintCntTrie,
		db:            db,
	}, nil
}

func (cc *ClaudeContext) Copy() *ClaudeContext {
	epochTrie := *cc.epochTrie
	delegateTrie := *cc.delegateTrie
	voteTrie := *cc.voteTrie
	candidateTrie := *cc.candidateTrie
	mintCntTrie := *cc.mintCntTrie
	return &ClaudeContext{
		epochTrie:     &epochTrie,
		delegateTrie:  &delegateTrie,
		voteTrie:      &voteTrie,
		candidateTrie: &candidateTrie,
		mintCntTrie:   &mintCntTrie,
	}
}

func (cc *ClaudeContext) Root() (h common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlputil.Encode(hw, cc.epochTrie.Hash())
	rlputil.Encode(hw, cc.delegateTrie.Hash())
	rlputil.Encode(hw, cc.candidateTrie.Hash())
	rlputil.Encode(hw, cc.voteTrie.Hash())
	rlputil.Encode(hw, cc.mintCntTrie.Hash())
	hw.Sum(h[:0])
	return h
}

func (cc *ClaudeContext) Snapshot() *ClaudeContext {
	return cc.Copy()
}

func (cc *ClaudeContext) RevertToSnapShot(snapshot *ClaudeContext) {
	cc.epochTrie = snapshot.epochTrie
	cc.delegateTrie = snapshot.delegateTrie
	cc.candidateTrie = snapshot.candidateTrie
	cc.voteTrie = snapshot.voteTrie
	cc.mintCntTrie = snapshot.mintCntTrie
}

func (cc *ClaudeContext) FromProto(ccp *ClaudeContextHash) error {
	var err error
	cc.epochTrie, err = NewEpochTrie(ccp.EpochHash, cc.db)
	if err != nil {
		return err
	}
	cc.delegateTrie, err = NewDelegateTrie(ccp.DelegateHash, cc.db)
	if err != nil {
		return err
	}
	cc.candidateTrie, err = NewCandidateTrie(ccp.CandidateHash, cc.db)
	if err != nil {
		return err
	}
	cc.voteTrie, err = NewVoteTrie(ccp.VoteHash, cc.db)
	if err != nil {
		return err
	}
	cc.mintCntTrie, err = NewMintCntTrie(ccp.MintCntHash, cc.db)
	return err
}

type ClaudeContextHash struct {
	EpochHash     common.Hash `json:"epochRoot"        gencodec:"required"`
	DelegateHash  common.Hash `json:"delegateRoot"     gencodec:"required"`
	CandidateHash common.Hash `json:"candidateRoot"    gencodec:"required"`
	VoteHash      common.Hash `json:"voteRoot"         gencodec:"required"`
	MintCntHash   common.Hash `json:"mintCntRoot"      gencodec:"required"`
}

func (cc *ClaudeContext) ToHash() *ClaudeContextHash {
	return &ClaudeContextHash{
		EpochHash:     cc.epochTrie.Hash(),
		DelegateHash:  cc.delegateTrie.Hash(),
		CandidateHash: cc.candidateTrie.Hash(),
		VoteHash:      cc.voteTrie.Hash(),
		MintCntHash:   cc.mintCntTrie.Hash(),
	}
}

func (ccp *ClaudeContextHash) Root() (h common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlputil.Encode(hw, ccp.EpochHash)
	rlputil.Encode(hw, ccp.DelegateHash)
	rlputil.Encode(hw, ccp.CandidateHash)
	rlputil.Encode(hw, ccp.VoteHash)
	rlputil.Encode(hw, ccp.MintCntHash)
	hw.Sum(h[:0])
	return h
}

func (cc *ClaudeContext) KickoutCandidate(candidateAddr common.Address) error {
	candidate := candidateAddr.Bytes()
	err := cc.candidateTrie.TryDelete(candidate)
	if err != nil {
		if _, ok := err.(*trie.MissingNodeError); !ok {
			return err
		}
	}
	iter := trie.NewIterator(cc.delegateTrie.NodeIterator(candidate))
	for iter.Next() {
		delegator := iter.Value
		key := append(candidate, delegator...)
		err = cc.delegateTrie.TryDelete(key)
		if err != nil {
			if _, ok := err.(*trie.MissingNodeError); !ok {
				return err
			}
		}
		v, err := cc.voteTrie.TryGet(delegator)
		if err != nil {
			if _, ok := err.(*trie.MissingNodeError); !ok {
				return err
			}
		}
		if err == nil && bytes.Equal(v, candidate) {
			err = cc.voteTrie.TryDelete(delegator)
			if err != nil {
				if _, ok := err.(*trie.MissingNodeError); !ok {
					return err
				}
			}
		}
	}
	return nil
}

func (cc *ClaudeContext) BecomeCandidate(candidateAddr common.Address) error {
	candidate := candidateAddr.Bytes()
	return cc.candidateTrie.TryUpdate(candidate, candidate)
}

func (cc *ClaudeContext) Delegate(delegatorAddr, candidateAddr common.Address) error {
	delegator, candidate := delegatorAddr.Bytes(), candidateAddr.Bytes()

	// the candidate must be candidate
	candidateInTrie, err := cc.candidateTrie.TryGet(candidate)
	if err != nil {
		return err
	}
	if candidateInTrie == nil {
		return errors.New("invalid candidate to delegate")
	}

	// delete old candidate if exists
	oldCandidate, err := cc.voteTrie.TryGet(delegator)
	if err != nil {
		if _, ok := err.(*trie.MissingNodeError); !ok {
			return err
		}
	}
	if oldCandidate != nil {
		cc.delegateTrie.Delete(append(oldCandidate, delegator...))
	}
	if err = cc.delegateTrie.TryUpdate(append(candidate, delegator...), delegator); err != nil {
		return err
	}
	return cc.voteTrie.TryUpdate(delegator, candidate)
}

func (cc *ClaudeContext) UnDelegate(delegatorAddr, candidateAddr common.Address) error {
	delegator, candidate := delegatorAddr.Bytes(), candidateAddr.Bytes()

	// the candidate must be candidate
	candidateInTrie, err := cc.candidateTrie.TryGet(candidate)
	if err != nil {
		return err
	}
	if candidateInTrie == nil {
		return errors.New("invalid candidate to undelegate")
	}

	oldCandidate, err := cc.voteTrie.TryGet(delegator)
	if err != nil {
		return err
	}
	if !bytes.Equal(candidate, oldCandidate) {
		return errors.New("mismatch candidate to undelegate")
	}

	if err = cc.delegateTrie.TryDelete(append(candidate, delegator...)); err != nil {
		return err
	}
	return cc.voteTrie.TryDelete(delegator)
}

func (cc *ClaudeContext) Commit(onleaf trie.LeafCallback) (*ClaudeContextHash, error) {
	epochRoot, err := cc.epochTrie.Commit(onleaf)
	if err != nil {
		return nil, err
	}
	delegateRoot, err := cc.delegateTrie.Commit(onleaf)
	if err != nil {
		return nil, err
	}
	voteRoot, err := cc.voteTrie.Commit(onleaf)
	if err != nil {
		return nil, err
	}
	candidateRoot, err := cc.candidateTrie.Commit(onleaf)
	if err != nil {
		return nil, err
	}
	mintCntRoot, err := cc.mintCntTrie.Commit(onleaf)
	if err != nil {
		return nil, err
	}
	return &ClaudeContextHash{
		EpochHash:     epochRoot,
		DelegateHash:  delegateRoot,
		VoteHash:      voteRoot,
		CandidateHash: candidateRoot,
		MintCntHash:   mintCntRoot,
	}, nil
}

func (cc *ClaudeContext) CandidateTrie() *trie.Trie         { return cc.candidateTrie }
func (cc *ClaudeContext) DelegateTrie() *trie.Trie          { return cc.delegateTrie }
func (cc *ClaudeContext) VoteTrie() *trie.Trie              { return cc.voteTrie }
func (cc *ClaudeContext) EpochTrie() *trie.Trie             { return cc.epochTrie }
func (cc *ClaudeContext) MintCntTrie() *trie.Trie           { return cc.mintCntTrie }
func (cc *ClaudeContext) DB() trie.TrieDatabase             { return cc.db }
func (cc *ClaudeContext) SetEpoch(epoch *trie.Trie)         { cc.epochTrie = epoch }
func (cc *ClaudeContext) SetDelegate(delegate *trie.Trie)   { cc.delegateTrie = delegate }
func (cc *ClaudeContext) SetVote(vote *trie.Trie)           { cc.voteTrie = vote }
func (cc *ClaudeContext) SetCandidate(candidate *trie.Trie) { cc.candidateTrie = candidate }
func (cc *ClaudeContext) SetMintCnt(mintCnt *trie.Trie)     { cc.mintCntTrie = mintCnt }

func (cc *ClaudeContext) GetValidators() ([]common.Address, error) {
	var validators []common.Address
	key := []byte("validator")
	validatorsRLP := cc.epochTrie.Get(key)
	if err := rlputil.DecodeBytes(validatorsRLP, &validators); err != nil {
		return nil, fmt.Errorf("failed to decode validators: %s", err)
	}
	return validators, nil
}

func (cc *ClaudeContext) SetValidators(validators []common.Address) error {
	key := []byte("validator")
	validatorsRLP, err := rlputil.EncodeToBytes(validators)
	if err != nil {
		return fmt.Errorf("failed to encode validators to rlputil bytes: %s", err)
	}
	cc.epochTrie.Update(key, validatorsRLP)
	return nil
}
