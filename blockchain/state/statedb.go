package state

import (
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/database/trie"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/metrics"
	"math/big"
	"sort"
	"time"
)

var stateLog = logger.NewLogger("[state]")

type revision struct {
	id           int
	journalIndex int
}

var (
	// emptyRoot is the known root hash of an empty trie.
	emptyRoot = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// emptyCode is the known hash of the empty EVM bytecode.
	emptyCode = crypto.Keccak256Hash(nil)
)

type proofList [][]byte

func (n *proofList) Put(key []byte, value []byte) error {
	*n = append(*n, value)
	return nil
}

func (n *proofList) Delete(key []byte) error {
	panic("not supported")
}

// StateDBs within the Entropy protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	db   StateDatabase
	trie Trie

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateObjects        map[common.Address]*stateObject
	stateObjectsPending map[common.Address]struct{} // State objects finalized but not yet written to the trie
	stateObjectsDirty   map[common.Address]struct{} // State objects modified in the current execution

	// DB error.
	// State objects are used by the consensus blockchain and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// The refund counter, also used by state transitioning.
	refund uint64

	thash, bhash common.Hash
	txIndex      int
	logs         map[common.Hash][]*model.Log
	logSize      uint

	preimages map[common.Hash][]byte

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        *journal
	validRevisions []revision
	nextRevisionId int

	// Measurements gathered during execution for debugging purposes
	AccountReads   time.Duration
	AccountHashes  time.Duration
	AccountUpdates time.Duration
	AccountCommits time.Duration
	StorageReads   time.Duration
	StorageHashes  time.Duration
	StorageUpdates time.Duration
	StorageCommits time.Duration
}

// Create a new state from a given trie.
func New(root common.Hash, db StateDatabase) (*StateDB, error) {
	tr, err := db.OpenTrie(root)
	if err != nil {
		return nil, err
	}
	return &StateDB{
		db:                  db,
		trie:                tr,
		stateObjects:        make(map[common.Address]*stateObject),
		stateObjectsPending: make(map[common.Address]struct{}),
		stateObjectsDirty:   make(map[common.Address]struct{}),
		logs:                make(map[common.Hash][]*model.Log),
		preimages:           make(map[common.Hash][]byte),
		journal:             newJournal(),
	}, nil
}

// setError remembers the first non-nil error it is called with.
func (stateDB *StateDB) setError(err error) {
	if stateDB.dbErr == nil {
		stateDB.dbErr = err
	}
}

func (stateDB *StateDB) Error() error {
	return stateDB.dbErr
}

// Reset clears out all ephemeral state objects from the state db, but keeps
// the underlying state trie to avoid reloading data for the next operations.
func (stateDB *StateDB) Reset(root common.Hash) error {
	tr, err := stateDB.db.OpenTrie(root)
	if err != nil {
		return err
	}
	stateDB.trie = tr
	stateDB.stateObjects = make(map[common.Address]*stateObject)
	stateDB.stateObjectsPending = make(map[common.Address]struct{})
	stateDB.stateObjectsDirty = make(map[common.Address]struct{})
	stateDB.thash = common.Hash{}
	stateDB.bhash = common.Hash{}
	stateDB.txIndex = 0
	stateDB.logs = make(map[common.Hash][]*model.Log)
	stateDB.logSize = 0
	stateDB.preimages = make(map[common.Hash][]byte)
	stateDB.clearJournalAndRefund()
	return nil
}

func (stateDB *StateDB) AddLog(log *model.Log) {
	stateDB.journal.append(addLogChange{txhash: stateDB.thash})

	log.TxHash = stateDB.thash
	log.BlockHash = stateDB.bhash
	log.TxIndex = uint(stateDB.txIndex)
	log.Index = stateDB.logSize
	stateDB.logs[stateDB.thash] = append(stateDB.logs[stateDB.thash], log)
	stateDB.logSize++
}

func (stateDB *StateDB) GetLogs(hash common.Hash) []*model.Log {
	return stateDB.logs[hash]
}

func (stateDB *StateDB) Logs() []*model.Log {
	var logs []*model.Log
	for _, lgs := range stateDB.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (stateDB *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	if _, ok := stateDB.preimages[hash]; !ok {
		stateDB.journal.append(addPreimageChange{hash: hash})
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		stateDB.preimages[hash] = pi
	}
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (stateDB *StateDB) Preimages() map[common.Hash][]byte {
	return stateDB.preimages
}

// AddRefund adds gas to the refund counter
func (stateDB *StateDB) AddRefund(gas uint64) {
	stateDB.journal.append(refundChange{prev: stateDB.refund})
	stateDB.refund += gas
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (stateDB *StateDB) SubRefund(gas uint64) {
	stateDB.journal.append(refundChange{prev: stateDB.refund})
	if gas > stateDB.refund {
		panic(fmt.Sprintf("Refund counter below zero (gas: %d > refund: %d)", gas, stateDB.refund))
	}
	stateDB.refund -= gas
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided account.
func (stateDB *StateDB) Exist(addr common.Address) bool {
	return stateDB.getStateObject(addr) != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (stateDB *StateDB) Empty(addr common.Address) bool {
	so := stateDB.getStateObject(addr)
	return so == nil || so.empty()
}

// Retrieve the balance from the given address or 0 if object not found
func (stateDB *StateDB) GetBalance(addr common.Address) *big.Int {
	stateObject := stateDB.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Balance()
	}
	return common.Big0
}

func (stateDB *StateDB) GetNonce(addr common.Address) uint64 {
	stateObject := stateDB.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

// TxIndex returns the current transaction index set by Prepare.
func (stateDB *StateDB) TxIndex() int {
	return stateDB.txIndex
}

// BlockHash returns the current block hash set by Prepare.
func (stateDB *StateDB) BlockHash() common.Hash {
	return stateDB.bhash
}

func (stateDB *StateDB) GetCode(addr common.Address) []byte {
	stateObject := stateDB.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(stateDB.db)
	}
	return nil
}

func (stateDB *StateDB) GetCodeSize(addr common.Address) int {
	stateObject := stateDB.getStateObject(addr)
	if stateObject == nil {
		return 0
	}
	if stateObject.code != nil {
		return len(stateObject.code)
	}
	size, err := stateDB.db.ContractCodeSize(stateObject.addrHash, common.BytesToHash(stateObject.CodeHash()))
	if err != nil {
		stateDB.setError(err)
	}
	return size
}

func (stateDB *StateDB) GetCodeHash(addr common.Address) common.Hash {
	stateObject := stateDB.getStateObject(addr)
	if stateObject == nil {
		return common.Hash{}
	}
	return common.BytesToHash(stateObject.CodeHash())
}

// GetState retrieves a value from the given account's storage trie.
func (stateDB *StateDB) GetState(addr common.Address, bhash common.Hash) common.Hash {
	stateObject := stateDB.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetState(stateDB.db, bhash)
	}
	return common.Hash{}
}

// GetProof returns the MerkleProof for a given Account
func (stateDB *StateDB) GetProof(a common.Address) ([][]byte, error) {
	var proof proofList
	err := stateDB.trie.Prove(crypto.Keccak256(a.Bytes()), 0, &proof)
	return [][]byte(proof), err
}

// GetProof returns the StorageProof for given key
func (stateDB *StateDB) GetStorageProof(a common.Address, key common.Hash) ([][]byte, error) {
	var proof proofList
	trie := stateDB.StorageTrie(a)
	if trie == nil {
		return proof, errors.New("storage trie for requested address does not exist")
	}
	err := trie.Prove(crypto.Keccak256(key.Bytes()), 0, &proof)
	return [][]byte(proof), err
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (stateDB *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	stateObject := stateDB.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetCommittedState(stateDB.db, hash)
	}
	return common.Hash{}
}

// Database retrieves the low level database supporting the lower level trie ops.
func (stateDB *StateDB) Database() StateDatabase {
	return stateDB.db
}

// StorageTrie returns the storage trie of an account.
// The return value is a copy and is nil for non-existent account.
func (stateDB *StateDB) StorageTrie(addr common.Address) Trie {
	stateObject := stateDB.getStateObject(addr)
	if stateObject == nil {
		return nil
	}
	cpy := stateObject.deepCopy(stateDB)
	return cpy.updateTrie(stateDB.db)
}

func (stateDB *StateDB) HasSuicided(addr common.Address) bool {
	stateObject := stateDB.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (stateDB *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	stateObject := stateDB.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(amount)

		stateLog.Debugf("AddBalance: balance=%d, amount=%d, addr=0x%x",
			stateObject.Balance(), amount, addr)
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (stateDB *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	stateObject := stateDB.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(amount)

		stateLog.Debugf("SubBalance: balance=%d, amount=%d, addr=0x%x",
			stateObject.Balance(), amount, addr)
	}
}

func (stateDB *StateDB) SetBalance(addr common.Address, amount *big.Int) {
	stateObject := stateDB.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount)

		stateLog.Debugf("SetBalance: balance=%d, amount=%d, addr=%X",
			stateObject.Balance(), amount, addr)
	}
}

func (stateDB *StateDB) SetNonce(addr common.Address, nonce uint64) {
	stateObject := stateDB.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)

		stateLog.Debugf("SetNonce: balance=%d, nonce=%d, addr=%X",
			stateObject.Balance(), nonce, addr)
	}
}

func (stateDB *StateDB) SetCode(addr common.Address, code []byte) {
	stateObject := stateDB.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetCode(crypto.Keccak256Hash(code), code)
	}
}

func (stateDB *StateDB) SetState(addr common.Address, key, value common.Hash) {
	stateObject := stateDB.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetState(stateDB.db, key, value)
	}
}

// SetStorage replaces the entire storage for the specified account with given
// storage. This function should only be used for debugging.
func (stateDB *StateDB) SetStorage(addr common.Address, storage map[common.Hash]common.Hash) {
	stateObject := stateDB.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetStorage(storage)
	}
}

// Suicide marks the given account as suicided.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (stateDB *StateDB) Suicide(addr common.Address) bool {
	stateObject := stateDB.getStateObject(addr)
	if stateObject == nil {
		return false
	}
	stateDB.journal.append(suicideChange{
		account:     &addr,
		prev:        stateObject.suicided,
		prevbalance: new(big.Int).Set(stateObject.Balance()),
	})
	stateObject.markSuicided()
	stateObject.data.Balance = new(big.Int)

	return true
}

//
// Setting, updating & deleting state object methods.
//

// updateStateObject writes the given object to the trie.
func (stateDB *StateDB) updateStateObject(stateObject *stateObject) {
	// Track the amount of time wasted on updating the account from the trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountUpdates += time.Since(start) }(time.Now())
	}
	// Encode the account and update the account trie
	addr := stateObject.Address()
	data, err := rlputil.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}
	stateDB.setError(stateDB.trie.TryUpdate(addr[:], data))
}

// deleteStateObject removes the given object from the state trie.
func (stateDB *StateDB) deleteStateObject(stateObject *stateObject) {
	// Track the amount of time wasted on deleting the account from the trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountUpdates += time.Since(start) }(time.Now())
	}
	// Delete the account from the trie
	addr := stateObject.Address()
	stateDB.setError(stateDB.trie.TryDelete(addr[:]))
}

// getStateObject retrieves a state object given by the address, returning nil if
// the object is not found or was deleted in this execution context. If you need
// to differentiate between non-existent/just-deleted, use getDeletedStateObject.
func (stateDB *StateDB) getStateObject(addr common.Address) *stateObject {
	if obj := stateDB.getDeletedStateObject(addr); obj != nil && !obj.deleted {
		return obj
	}
	return nil
}

// getDeletedStateObject is similar to getStateObject, but instead of returning
// nil for a deleted state object, it returns the actual object with the deleted
// flag set. This is needed by the state journal to revert to the correct self-
// destructed object instead of wiping all knowledge about the state object.
func (stateDB *StateDB) getDeletedStateObject(addr common.Address) *stateObject {
	// Prefer live objects if any is available
	if obj := stateDB.stateObjects[addr]; obj != nil {
		return obj
	}
	// Track the amount of time wasted on loading the object from the database
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountReads += time.Since(start) }(time.Now())
	}
	// Load the object from the database.
	enc, err := stateDB.trie.TryGet(addr[:])
	if len(enc) == 0 {
		stateDB.setError(err)
		return nil
	}
	var data Account
	if err := rlputil.DecodeBytes(enc, &data); err != nil {
		stateLog.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newObject(stateDB, addr, data)
	stateDB.setStateObject(obj)
	return obj
}

func (stateDB *StateDB) setStateObject(object *stateObject) {
	stateDB.stateObjects[object.Address()] = object
}

// Retrieve a state object or create a new state object if nil.
func (stateDB *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	stateObject := stateDB.getStateObject(addr)
	if stateObject == nil {
		stateObject, _ = stateDB.createObject(addr)
	}
	return stateObject
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (stateDB *StateDB) createObject(addr common.Address) (newobj, prev *stateObject) {
	prev = stateDB.getDeletedStateObject(addr) // Note, prev might have been deleted, we need that!

	newobj = newObject(stateDB, addr, Account{})
	newobj.setNonce(0) // sets the object to dirty
	if prev == nil {
		stateDB.journal.append(createObjectChange{account: &addr})
	} else {
		stateDB.journal.append(resetObjectChange{prev: prev})
	}
	stateDB.setStateObject(newobj)
	return newobj, prev
}

// CreateAccount explicitly creates a state object. If a state object with the address
// already exists the balance is carried over to the new account.
//
// CreateAccount is called during the EVM CREATE operation. The situation might arise that
// a contract does the following:
//
//   1. sends funds to sha(account ++ (nonce + 1))
//   2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (stateDB *StateDB) CreateAccount(addr common.Address) {
	newDB, prev := stateDB.createObject(addr)
	if prev != nil {
		newDB.setBalance(prev.data.Balance)
	}
}

func (stateDB *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) error {
	so := stateDB.getStateObject(addr)
	if so == nil {
		return nil
	}
	it := trie.NewIterator(so.getTrie(stateDB.db).NodeIterator(nil))
	for it.Next() {
		key := common.BytesToHash(stateDB.trie.GetKey(it.Key))
		if value, dirty := so.dirtyStorage[key]; dirty {
			if !cb(key, value) {
				return nil
			}
			continue
		}
		if len(it.Value) > 0 {
			_, content, _, err := rlputil.Split(it.Value)
			if err != nil {
				return err
			}
			if !cb(key, common.BytesToHash(content)) {
				return nil
			}
		}
	}
	return nil
}

// Copy creates a deep, independent copy of the state.
// Snapshots of the copied state cannot be applied to the copy.
func (stateDB *StateDB) Copy() *StateDB {
	// Copy all the basic fields, initialize the memory ones
	state := &StateDB{
		db:                  stateDB.db,
		trie:                stateDB.db.CopyTrie(stateDB.trie),
		stateObjects:        make(map[common.Address]*stateObject, len(stateDB.journal.dirties)),
		stateObjectsPending: make(map[common.Address]struct{}, len(stateDB.stateObjectsPending)),
		stateObjectsDirty:   make(map[common.Address]struct{}, len(stateDB.journal.dirties)),
		refund:              stateDB.refund,
		logs:                make(map[common.Hash][]*model.Log, len(stateDB.logs)),
		logSize:             stateDB.logSize,
		preimages:           make(map[common.Hash][]byte, len(stateDB.preimages)),
		journal:             newJournal(),
	}
	// Copy the dirty states, logs, and preimages
	for addr := range stateDB.journal.dirties {
		// As documented [here](https://github.com/entropy/go-entropy/pull/16485#issuecomment-380438527),
		// and in the Finalise-method, there is a case where an object is in the journal but not
		// in the stateObjects: OOG after touch on ripeMD prior to Byzantium. Thus, we need to check for
		// nil
		if object, exist := stateDB.stateObjects[addr]; exist {
			// Even though the original object is dirty, we are not copying the journal,
			// so we need to make sure that anyside effect the journal would have caused
			// during a commit (or similar op) is already applied to the copy.
			state.stateObjects[addr] = object.deepCopy(state)

			state.stateObjectsDirty[addr] = struct{}{}   // Mark the copy dirty to force internal (code/state) commits
			state.stateObjectsPending[addr] = struct{}{} // Mark the copy pending to force external (account) commits
		}
	}
	// Above, we don't copy the actual journal. This means that if the copy is copied, the
	// loop above will be a no-op, since the copy's journal is empty.
	// Thus, here we iterate over stateObjects, to enable copies of copies
	for addr := range stateDB.stateObjectsPending {
		if _, exist := state.stateObjects[addr]; !exist {
			state.stateObjects[addr] = stateDB.stateObjects[addr].deepCopy(state)
		}
		state.stateObjectsPending[addr] = struct{}{}
	}
	for addr := range stateDB.stateObjectsDirty {
		if _, exist := state.stateObjects[addr]; !exist {
			state.stateObjects[addr] = stateDB.stateObjects[addr].deepCopy(state)
		}
		state.stateObjectsDirty[addr] = struct{}{}
	}
	for hash, logs := range stateDB.logs {
		cpy := make([]*model.Log, len(logs))
		for i, l := range logs {
			cpy[i] = new(model.Log)
			*cpy[i] = *l
		}
		state.logs[hash] = cpy
	}
	for hash, preimage := range stateDB.preimages {
		state.preimages[hash] = preimage
	}
	return state
}

// Snapshot returns an identifier for the current revision of the state.
func (stateDB *StateDB) Snapshot() int {
	id := stateDB.nextRevisionId
	stateDB.nextRevisionId++
	stateDB.validRevisions = append(stateDB.validRevisions, revision{id, stateDB.journal.length()})
	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (stateDB *StateDB) RevertToSnapshot(revid int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(stateDB.validRevisions), func(i int) bool {
		return stateDB.validRevisions[i].id >= revid
	})
	if idx == len(stateDB.validRevisions) || stateDB.validRevisions[idx].id != revid {
		panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	}
	snapshot := stateDB.validRevisions[idx].journalIndex

	// Replay the journal to undo changes and remove invalidated snapshots
	stateDB.journal.revert(stateDB, snapshot)
	stateDB.validRevisions = stateDB.validRevisions[:idx]
}

// GetRefund returns the current value of the refund counter.
func (stateDB *StateDB) GetRefund() uint64 {
	return stateDB.refund
}

// Finalise finalises the state by removing the self destructed objects and clears
// the journal as well as the refunds. Finalise, however, will not push any updates
// into the tries just yet. Only IntermediateRoot or Commit will do that.
func (stateDB *StateDB) Finalise(deleteEmptyObjects bool) {
	for addr := range stateDB.journal.dirties {
		stateObject, exist := stateDB.stateObjects[addr]
		if !exist {
			// ripeMD is 'touched' at block 1714175, in tx 0x1237f737031e40bcde4a8b7e717b2d15e3ecadfe49bb1bbc71ee9deb09c6fcf2
			// That tx goes out of gas, and although the notion of 'touched' does not exist there, the
			// touch-event will still be recorded in the journal. Since ripeMD is a special snowflake,
			// it will persist in the journal even though the journal is reverted. In this special circumstance,
			// it may exist in `s.journal.dirties` but not in `s.stateObjects`.
			// Thus, we can safely ignore it here
			continue
		}

		if stateObject.suicided || (deleteEmptyObjects && stateObject.empty()) {
			stateObject.deleted = true
		} else {
			stateObject.finalise()
		}
		stateDB.stateObjectsPending[addr] = struct{}{}
		stateDB.stateObjectsDirty[addr] = struct{}{}
	}
	// Invalidate journal because reverting across transactions is not allowed.
	stateDB.clearJournalAndRefund()
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (stateDB *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	// Finalise all the dirty storage states and write them into the tries
	stateDB.Finalise(deleteEmptyObjects)

	for addr := range stateDB.stateObjectsPending {
		obj := stateDB.stateObjects[addr]
		if obj.deleted {
			stateDB.deleteStateObject(obj)
		} else {
			obj.updateRoot(stateDB.db)
			stateDB.updateStateObject(obj)
		}
	}
	if len(stateDB.stateObjectsPending) > 0 {
		stateDB.stateObjectsPending = make(map[common.Address]struct{})
	}
	// Track the amount of time wasted on hashing the account trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountHashes += time.Since(start) }(time.Now())
	}
	return stateDB.trie.Hash()
}

// Prepare sets the current transaction hash and index and block hash which is
// used when the EVM emits new state logs.
func (stateDB *StateDB) Prepare(thash, bhash common.Hash, ti int) {
	stateDB.thash = thash
	stateDB.bhash = bhash
	stateDB.txIndex = ti
}

func (stateDB *StateDB) clearJournalAndRefund() {
	if len(stateDB.journal.entries) > 0 {
		stateDB.journal = newJournal()
		stateDB.refund = 0
	}
	stateDB.validRevisions = stateDB.validRevisions[:0] // Snapshots can be created without journal entires
}

// Commit writes the state to the underlying in-memory trie database.
func (stateDB *StateDB) Commit(deleteEmptyObjects bool) (root common.Hash, err error) {
	// Finalize any pending changes and merge everything into the tries
	stateDB.IntermediateRoot(deleteEmptyObjects)

	// Commit objects to the trie, measuring the elapsed time
	for addr := range stateDB.stateObjectsDirty {
		if stateObject := stateDB.stateObjects[addr]; !stateObject.deleted {
			// Write any contract code associated with the state object
			if stateObject.code != nil && stateObject.dirtyCode {
				stateDB.db.TrieDB().InsertBlob(common.BytesToHash(stateObject.CodeHash()), stateObject.code)
				stateObject.dirtyCode = false
			}
			// Write any storage changes in the state object to its storage trie.
			if err := stateObject.CommitTrie(stateDB.db); err != nil {
				return common.Hash{}, err
			}
		}
	}
	if len(stateDB.stateObjectsDirty) > 0 {
		stateDB.stateObjectsDirty = make(map[common.Address]struct{})
	}
	// Write the account trie changes, measuing the amount of wasted time
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountCommits += time.Since(start) }(time.Now())
	}
	return stateDB.trie.Commit(func(leaf []byte, parent common.Hash) error {
		var account Account
		if err := rlputil.DecodeBytes(leaf, &account); err != nil {
			return nil
		}
		if account.Root != emptyRoot {
			stateDB.db.TrieDB().Reference(account.Root, parent)
		}
		code := common.BytesToHash(account.CodeHash)
		if code != emptyCode {
			stateDB.db.TrieDB().Reference(code, parent)
		}
		return nil
	})
}
