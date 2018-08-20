package model

import (
	"testing"

	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/trie"
	"github.com/stretchr/testify/assert"
)

func TestClaudeContextSnapshot(t *testing.T) {
	db := *trie.NewDatabase(database.NewMemDatabase())
	claudeContext, err := NewClaudeContext(db)
	assert.Nil(t, err)

	snapshot := claudeContext.Snapshot()
	assert.Equal(t, claudeContext.Root(), snapshot.Root())
	assert.NotEqual(t, claudeContext, snapshot)

	// change claudeContext
	assert.Nil(t, claudeContext.BecomeCandidate(common.HexToAddress("0x44d1ce0b7cb3588bca96151fe1bc05af38f91b6c")))
	assert.NotEqual(t, claudeContext.Root(), snapshot.Root())

	// revert snapshot
	claudeContext.RevertToSnapShot(snapshot)
	assert.Equal(t, claudeContext.Root(), snapshot.Root())
	assert.NotEqual(t, claudeContext, snapshot)
}

func TestClaudeContextBecomeCandidate(t *testing.T) {
	candidates := []common.Address{
		common.HexToAddress("0x44d1ce0b7cb3588bca96151fe1bc05af38f91b6e"),
		common.HexToAddress("0xa60a3886b552ff9992cfcd208ec1152079e046c2"),
		common.HexToAddress("0x4e080e49f62694554871e669aeb4ebe17c4a9670"),
	}
	db := *trie.NewDatabase(database.NewMemDatabase())
	claudeContext, err := NewClaudeContext(db)
	assert.Nil(t, err)
	for _, candidate := range candidates {
		assert.Nil(t, claudeContext.BecomeCandidate(candidate))
	}

	candidateMap := map[common.Address]bool{}
	candidateIter := trie.NewIterator(claudeContext.candidateTrie.NodeIterator(nil))
	for candidateIter.Next() {
		candidateMap[common.BytesToAddress(candidateIter.Value)] = true
	}
	assert.Equal(t, len(candidates), len(candidateMap))
	for _, candidate := range candidates {
		assert.True(t, candidateMap[candidate])
	}
}

func TestClaudeContextKickoutCandidate(t *testing.T) {
	candidates := []common.Address{
		common.HexToAddress("0x44d1ce0b7cb3588bca96151fe1bc05af38f91b6e"),
		common.HexToAddress("0xa60a3886b552ff9992cfcd208ec1152079e046c2"),
		common.HexToAddress("0x4e080e49f62694554871e669aeb4ebe17c4a9670"),
	}
	db := *trie.NewDatabase(database.NewMemDatabase())
	claudeContext, err := NewClaudeContext(db)
	assert.Nil(t, err)
	for _, candidate := range candidates {
		assert.Nil(t, claudeContext.BecomeCandidate(candidate))
		assert.Nil(t, claudeContext.Delegate(candidate, candidate))
	}

	kickIdx := 1
	assert.Nil(t, claudeContext.KickoutCandidate(candidates[kickIdx]))
	candidateMap := map[common.Address]bool{}
	candidateIter := trie.NewIterator(claudeContext.candidateTrie.NodeIterator(nil))
	for candidateIter.Next() {
		candidateMap[common.BytesToAddress(candidateIter.Value)] = true
	}
	voteIter := trie.NewIterator(claudeContext.voteTrie.NodeIterator(nil))
	voteMap := map[common.Address]bool{}
	for voteIter.Next() {
		voteMap[common.BytesToAddress(voteIter.Value)] = true
	}
	for i, candidate := range candidates {
		delegateIter := trie.NewIterator(claudeContext.delegateTrie.NodeIterator(candidate.Bytes()))
		if i == kickIdx {
			assert.False(t, delegateIter.Next())
			assert.False(t, candidateMap[candidate])
			assert.False(t, voteMap[candidate])
			continue
		}
		assert.True(t, delegateIter.Next())
		assert.True(t, candidateMap[candidate])
		assert.True(t, voteMap[candidate])
	}
}

func TestClaudeContextDelegateAndUnDelegate(t *testing.T) {
	candidate := common.HexToAddress("0x44d1ce0b7cb3588bca96151fe1bc05af38f91b6e")
	newCandidate := common.HexToAddress("0xa60a3886b552ff9992cfcd208ec1152079e046c2")
	delegator := common.HexToAddress("0x4e080e49f62694554871e669aeb4ebe17c4a9670")
	db := *trie.NewDatabase(database.NewMemDatabase())
	claudeContext, err := NewClaudeContext(db)
	assert.Nil(t, err)
	assert.Nil(t, claudeContext.BecomeCandidate(candidate))
	assert.Nil(t, claudeContext.BecomeCandidate(newCandidate))

	// delegator delegate to not exist candidate
	candidateIter := trie.NewIterator(claudeContext.candidateTrie.NodeIterator(nil))
	candidateMap := map[string]bool{}
	for candidateIter.Next() {
		candidateMap[string(candidateIter.Value)] = true
	}
	assert.NotNil(t, claudeContext.Delegate(delegator, common.HexToAddress("0xab")))

	// delegator delegate to old candidate
	assert.Nil(t, claudeContext.Delegate(delegator, candidate))
	delegateIter := trie.NewIterator(claudeContext.delegateTrie.NodeIterator(candidate.Bytes()))
	if assert.True(t, delegateIter.Next()) {
		assert.Equal(t, append(delegatePrefix, append(candidate.Bytes(), delegator.Bytes()...)...), delegateIter.Key)
		assert.Equal(t, delegator, common.BytesToAddress(delegateIter.Value))
	}
	voteIter := trie.NewIterator(claudeContext.voteTrie.NodeIterator(nil))
	if assert.True(t, voteIter.Next()) {
		assert.Equal(t, append(votePrefix, delegator.Bytes()...), voteIter.Key)
		assert.Equal(t, candidate, common.BytesToAddress(voteIter.Value))
	}

	// delegator delegate to new candidate
	assert.Nil(t, claudeContext.Delegate(delegator, newCandidate))
	delegateIter = trie.NewIterator(claudeContext.delegateTrie.NodeIterator(candidate.Bytes()))
	assert.False(t, delegateIter.Next())
	delegateIter = trie.NewIterator(claudeContext.delegateTrie.NodeIterator(newCandidate.Bytes()))
	if assert.True(t, delegateIter.Next()) {
		assert.Equal(t, append(delegatePrefix, append(newCandidate.Bytes(), delegator.Bytes()...)...), delegateIter.Key)
		assert.Equal(t, delegator, common.BytesToAddress(delegateIter.Value))
	}
	voteIter = trie.NewIterator(claudeContext.voteTrie.NodeIterator(nil))
	if assert.True(t, voteIter.Next()) {
		assert.Equal(t, append(votePrefix, delegator.Bytes()...), voteIter.Key)
		assert.Equal(t, newCandidate, common.BytesToAddress(voteIter.Value))
	}

	// delegator undelegate to not exist candidate
	assert.NotNil(t, claudeContext.UnDelegate(common.HexToAddress("0x00"), candidate))

	// delegator undelegate to old candidate
	assert.NotNil(t, claudeContext.UnDelegate(delegator, candidate))

	// delegator undelegate to new candidate
	assert.Nil(t, claudeContext.UnDelegate(delegator, newCandidate))
	delegateIter = trie.NewIterator(claudeContext.delegateTrie.NodeIterator(newCandidate.Bytes()))
	assert.False(t, delegateIter.Next())
	voteIter = trie.NewIterator(claudeContext.voteTrie.NodeIterator(nil))
	assert.False(t, voteIter.Next())
}

func TestClaudeContextValidators(t *testing.T) {
	validators := []common.Address{
		common.HexToAddress("0x44d1ce0b7cb3588bca96151fe1bc05af38f91b6e"),
		common.HexToAddress("0xa60a3886b552ff9992cfcd208ec1152079e046c2"),
		common.HexToAddress("0x4e080e49f62694554871e669aeb4ebe17c4a9670"),
	}

	db := *trie.NewDatabase(database.NewMemDatabase())
	claudeContext, err := NewClaudeContext(db)
	assert.Nil(t, err)

	assert.Nil(t, claudeContext.SetValidators(validators))

	result, err := claudeContext.GetValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), len(result))
	validatorMap := map[common.Address]bool{}
	for _, validator := range validators {
		validatorMap[validator] = true
	}
	for _, validator := range result {
		assert.True(t, validatorMap[validator])
	}
}
