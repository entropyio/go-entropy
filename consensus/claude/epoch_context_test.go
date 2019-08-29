package claude

import (
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/entropyio/go-entropy/blockchain/mapper"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/database/trie"
	"github.com/stretchr/testify/assert"
)

func TestEpochContextCountVotes(t *testing.T) {
	voteMap := map[common.Address][]common.Address{
		common.HexToAddress("0x44d1ce0b7cb3588bca96151fe1bc05af38f91b6e"): {
			common.HexToAddress("0xb040353ec0f2c113d5639444f7253681aecda1f8"),
		},
		common.HexToAddress("0xa60a3886b552ff9992cfcd208ec1152079e046c2"): {
			common.HexToAddress("0x14432e15f21237013017fa6ee90fc99433dec82c"),
			common.HexToAddress("0x9f30d0e5c9c88cade54cd1adecf6bc2c7e0e5af6"),
		},
		common.HexToAddress("0x4e080e49f62694554871e669aeb4ebe17c4a9670"): {
			common.HexToAddress("0xd83b44a3719720ec54cdb9f54c0202de68f1ebcb"),
			common.HexToAddress("0x56cc452e450551b7b9cffe25084a069e8c1e9441"),
			common.HexToAddress("0xbcfcb3fa8250be4f2bf2b1e70e1da500c668377b"),
		},
		common.HexToAddress("0x9d9667c71bb09d6ca7c3ed12bfe5e7be24e2ffe1"): {},
	}
	balance := uint64(5)
	db := mapper.NewMemoryDatabase()
	stateDB, _ := state.New(common.Hash{}, state.NewDatabase(db))
	claudeContext, err := model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)

	epochContext := &EpochContext{
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	_, err = epochContext.countVotes()
	assert.NotNil(t, err)

	for candidate, electors := range voteMap {
		assert.Nil(t, claudeContext.BecomeCandidate(candidate))
		for _, elector := range electors {
			stateDB.SetBalance(elector, big.NewInt((int64)(balance)))
			assert.Nil(t, claudeContext.Delegate(elector, candidate))
		}
	}
	result, err := epochContext.countVotes()
	assert.Nil(t, err)
	assert.Equal(t, len(voteMap), len(result))
	for candidate, electors := range voteMap {
		voteCount, ok := result[candidate]
		assert.True(t, ok)
		assert.Equal(t, balance*uint64(len(electors)), voteCount.Uint64())
	}
}

func TestLookupValidator(t *testing.T) {
	db := *trie.NewDatabase(mapper.NewMemoryDatabase())
	claudeContext, _ := model.NewClaudeContext(db)
	mockEpochContext := &EpochContext{
		ClaudeContext: claudeContext,
	}
	validators := []common.Address{
		common.BytesToAddress([]byte("addr1")),
		common.BytesToAddress([]byte("addr2")),
		common.BytesToAddress([]byte("addr3")),
	}
	mockEpochContext.ClaudeContext.SetValidators(validators)
	for i, expected := range validators {
		got, _ := mockEpochContext.lookupValidator(uint64(i) * blockInterval)
		if got != expected {
			t.Errorf("Failed to test lookup validator, %s was expected but got %s", expected, got)
		}
	}
	_, err := mockEpochContext.lookupValidator(blockInterval - 1)
	if err != ErrInvalidMintBlockTime {
		t.Errorf("Failed to test lookup validator. err '%v' was expected but got '%v'", ErrInvalidMintBlockTime, err)
	}
}

func TestEpochContextKickoutValidator(t *testing.T) {
	db := mapper.NewMemoryDatabase()
	stateDB, _ := state.New(common.Hash{}, state.NewDatabase(db))
	claudeContext, err := model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext := &EpochContext{
		TimeStamp:     epochInterval,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	atLeastMintCnt := epochInterval / blockInterval / maxValidatorSize / 2
	testEpoch := uint64(1)

	// no validator can be kickout, because all validators mint enough block at least
	validators := []common.Address{}
	for i := 0; i < maxValidatorSize; i++ {
		validator := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		validators = append(validators, validator)
		assert.Nil(t, claudeContext.BecomeCandidate(validator))
		setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt)
	}
	assert.Nil(t, claudeContext.SetValidators(validators))
	assert.Nil(t, claudeContext.BecomeCandidate(common.BytesToAddress([]byte("addr"))))
	assert.Nil(t, epochContext.kickoutValidator(testEpoch))
	candidateMap := getCandidates(claudeContext.CandidateTrie())
	assert.Equal(t, maxValidatorSize+1, len(candidateMap))

	// atLeast a safeSize count candidate will reserve
	claudeContext, err = model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext = &EpochContext{
		TimeStamp:     epochInterval,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	validators = []common.Address{}
	for i := 0; i < maxValidatorSize; i++ {
		validator := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		validators = append(validators, validator)
		assert.Nil(t, claudeContext.BecomeCandidate(validator))
		setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt-uint64(i)-1)
	}
	assert.Nil(t, claudeContext.SetValidators(validators))
	assert.Nil(t, epochContext.kickoutValidator(testEpoch))
	candidateMap = getCandidates(claudeContext.CandidateTrie())
	assert.Equal(t, safeSize, len(candidateMap))
	for i := maxValidatorSize - 1; i >= safeSize; i-- {
		assert.False(t, candidateMap[common.BytesToAddress([]byte("addr"+strconv.Itoa(i)))])
	}

	// all validator will be kickout, because all validators didn't mint enough block at least
	claudeContext, err = model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext = &EpochContext{
		TimeStamp:     epochInterval,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	validators = []common.Address{}
	for i := 0; i < maxValidatorSize; i++ {
		validator := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		validators = append(validators, validator)
		assert.Nil(t, claudeContext.BecomeCandidate(validator))
		setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt-1)
	}
	for i := maxValidatorSize; i < maxValidatorSize*2; i++ {
		candidate := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		assert.Nil(t, claudeContext.BecomeCandidate(candidate))
	}
	assert.Nil(t, claudeContext.SetValidators(validators))
	assert.Nil(t, epochContext.kickoutValidator(testEpoch))
	candidateMap = getCandidates(claudeContext.CandidateTrie())
	assert.Equal(t, maxValidatorSize, len(candidateMap))

	// only one validator mint count is not enough
	claudeContext, err = model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext = &EpochContext{
		TimeStamp:     epochInterval,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	validators = []common.Address{}
	for i := 0; i < maxValidatorSize; i++ {
		validator := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		validators = append(validators, validator)
		assert.Nil(t, claudeContext.BecomeCandidate(validator))
		if i == 0 {
			setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt-1)
		} else {
			setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt)
		}
	}
	assert.Nil(t, claudeContext.BecomeCandidate(common.BytesToAddress([]byte("addr"))))
	assert.Nil(t, claudeContext.SetValidators(validators))
	assert.Nil(t, epochContext.kickoutValidator(testEpoch))
	candidateMap = getCandidates(claudeContext.CandidateTrie())
	assert.Equal(t, maxValidatorSize, len(candidateMap))
	assert.False(t, candidateMap[common.BytesToAddress([]byte("addr"+strconv.Itoa(0)))])

	// epochTime is not complete, all validators mint enough block at least
	claudeContext, err = model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext = &EpochContext{
		TimeStamp:     epochInterval / 2,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	validators = []common.Address{}
	for i := 0; i < maxValidatorSize; i++ {
		validator := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		validators = append(validators, validator)
		assert.Nil(t, claudeContext.BecomeCandidate(validator))
		setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt/2)
	}
	for i := maxValidatorSize; i < maxValidatorSize*2; i++ {
		candidate := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		assert.Nil(t, claudeContext.BecomeCandidate(candidate))
	}
	assert.Nil(t, claudeContext.SetValidators(validators))
	assert.Nil(t, epochContext.kickoutValidator(testEpoch))
	candidateMap = getCandidates(claudeContext.CandidateTrie())
	assert.Equal(t, maxValidatorSize*2, len(candidateMap))

	// epochTime is not complete, all validators didn't mint enough block at least
	claudeContext, err = model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext = &EpochContext{
		TimeStamp:     epochInterval / 2,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	validators = []common.Address{}
	for i := 0; i < maxValidatorSize; i++ {
		validator := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		validators = append(validators, validator)
		assert.Nil(t, claudeContext.BecomeCandidate(validator))
		setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt/2-1)
	}
	for i := maxValidatorSize; i < maxValidatorSize*2; i++ {
		candidate := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		assert.Nil(t, claudeContext.BecomeCandidate(candidate))
	}
	assert.Nil(t, claudeContext.SetValidators(validators))
	assert.Nil(t, epochContext.kickoutValidator(testEpoch))
	candidateMap = getCandidates(claudeContext.CandidateTrie())
	assert.Equal(t, maxValidatorSize, len(candidateMap))

	claudeContext, err = model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext = &EpochContext{
		TimeStamp:     epochInterval / 2,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	assert.NotNil(t, epochContext.kickoutValidator(testEpoch))
	claudeContext.SetValidators([]common.Address{})
	assert.NotNil(t, epochContext.kickoutValidator(testEpoch))
}

func setTestMintCnt(claudeContext *model.ClaudeContext, epoch uint64, validator common.Address, count uint64) {
	for i := uint64(0); i < count; i++ {
		updateMintCnt(epoch*epochInterval, epoch*epochInterval+blockInterval, validator, claudeContext)
	}
}

func getCandidates(candidateTrie *trie.Trie) map[common.Address]bool {
	candidateMap := map[common.Address]bool{}
	iter := trie.NewIterator(candidateTrie.NodeIterator(nil))
	for iter.Next() {
		candidateMap[common.BytesToAddress(iter.Value)] = true
	}
	return candidateMap
}

func TestEpochContextTryElect(t *testing.T) {
	db := mapper.NewMemoryDatabase()
	stateDB, _ := state.New(common.Hash{}, state.NewDatabase(db))
	claudeContext, err := model.NewClaudeContext(*trie.NewDatabase(db))
	assert.Nil(t, err)
	epochContext := &EpochContext{
		TimeStamp:     epochInterval,
		ClaudeContext: claudeContext,
		statedb:       stateDB,
	}
	atLeastMintCnt := epochInterval / blockInterval / maxValidatorSize / 2
	testEpoch := uint64(1)
	validators := []common.Address{}
	for i := 0; i < maxValidatorSize; i++ {
		validator := common.BytesToAddress([]byte("addr" + strconv.Itoa(i)))
		validators = append(validators, validator)
		assert.Nil(t, claudeContext.BecomeCandidate(validator))
		assert.Nil(t, claudeContext.Delegate(validator, validator))
		stateDB.SetBalance(validator, big.NewInt(1))
		setTestMintCnt(claudeContext, testEpoch, validator, atLeastMintCnt-1)
	}
	claudeContext.BecomeCandidate(common.BytesToAddress([]byte("more")))
	assert.Nil(t, claudeContext.SetValidators(validators))

	// genesisEpoch == parentEpoch do not kickout
	genesis := &model.Header{
		Time:          uint64(0),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	parent := &model.Header{
		Time:          uint64(epochInterval - blockInterval),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	oldHash := claudeContext.EpochTrie().Hash()
	assert.Nil(t, epochContext.tryElect(genesis, parent))
	result, err := claudeContext.GetValidators()
	assert.Nil(t, err)
	assert.Equal(t, maxValidatorSize, len(result))
	for _, validator := range result {
		assert.True(t, strings.Contains(string(validator[:]), "addr"))
	}
	assert.NotEqual(t, oldHash, claudeContext.EpochTrie().Hash())

	// genesisEpoch != parentEpoch and have none mintCnt do not kickout
	genesis = &model.Header{
		Time:          uint64(epochInterval),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	parent = &model.Header{
		Difficulty:    big.NewInt(1),
		Time:          uint64(epochInterval - blockInterval),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	epochContext.TimeStamp = epochInterval
	oldHash = claudeContext.EpochTrie().Hash()
	assert.Nil(t, epochContext.tryElect(genesis, parent))
	result, err = claudeContext.GetValidators()
	assert.Nil(t, err)
	assert.Equal(t, maxValidatorSize, len(result))
	for _, validator := range result {
		assert.True(t, strings.Contains(string(validator[:]), "addr"))
	}
	assert.NotEqual(t, oldHash, claudeContext.EpochTrie().Hash())

	// genesisEpoch != parentEpoch kickout
	genesis = &model.Header{
		Time:          uint64(0),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	parent = &model.Header{
		Time:          uint64(epochInterval*2 - blockInterval),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	epochContext.TimeStamp = epochInterval * 2
	oldHash = claudeContext.EpochTrie().Hash()
	assert.Nil(t, epochContext.tryElect(genesis, parent))
	result, err = claudeContext.GetValidators()
	assert.Nil(t, err)
	assert.Equal(t, safeSize, len(result))
	moreCnt := 0
	for _, validator := range result {
		if strings.Contains(string(validator[:]), "more") {
			moreCnt++
		}
	}
	assert.Equal(t, 1, moreCnt)
	assert.NotEqual(t, oldHash, claudeContext.EpochTrie().Hash())

	// parentEpoch == currentEpoch do not elect
	genesis = &model.Header{
		Time:          uint64(0),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	parent = &model.Header{
		Time:          uint64(epochInterval),
		ClaudeCtxHash: &model.ClaudeContextHash{},
	}
	epochContext.TimeStamp = epochInterval + blockInterval
	oldHash = claudeContext.EpochTrie().Hash()
	assert.Nil(t, epochContext.tryElect(genesis, parent))
	result, err = claudeContext.GetValidators()
	assert.Nil(t, err)
	assert.Equal(t, safeSize, len(result))
	assert.Equal(t, oldHash, claudeContext.EpochTrie().Hash())
}
