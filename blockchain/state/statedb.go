package state

import (
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state/snapshot"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/database/trie"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/metrics"
	"math/big"
	"sort"
	"time"
)

var log = logger.NewLogger("[state]")

type revision struct {
	id           int
	journalIndex int
}

var (
	// emptyRoot is the known root hash of an empty trie.
	emptyRoot = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
)

type proofList [][]byte

func (n *proofList) Put(key []byte, value []byte, from string) error {
	log.Debugf("proofList Put %s. key:%x, vSize:%d", from, key, len(value))
	*n = append(*n, value)
	return nil
}

func (n *proofList) Delete([]byte, string) error {
	panic("not supported")
}

// StateDB structs within the entropy protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	db         StateDatabase
	prefetcher *triePrefetcher
	trie       Trie
	hasher     crypto.KeccakState

	// originalRoot is the pre-state root, before any changes were made.
	// It will be updated when the Commit is called.
	originalRoot common.Hash

	snaps         *snapshot.Tree
	snap          snapshot.Snapshot
	snapDestructs map[common.Hash]struct{}
	snapAccounts  map[common.Hash][]byte
	snapStorage   map[common.Hash]map[common.Hash][]byte

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

	thash   common.Hash
	txIndex int
	logs    map[common.Hash][]*model.Log
	logSize uint

	preimages map[common.Hash][]byte

	// Per-transaction access list
	accessList *accessList

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        *journal
	validRevisions []revision
	nextRevisionId int

	// Measurements gathered during execution for debugging purposes
	AccountReads         time.Duration
	AccountHashes        time.Duration
	AccountUpdates       time.Duration
	AccountCommits       time.Duration
	StorageReads         time.Duration
	StorageHashes        time.Duration
	StorageUpdates       time.Duration
	StorageCommits       time.Duration
	SnapshotAccountReads time.Duration
	SnapshotStorageReads time.Duration
	SnapshotCommits      time.Duration

	AccountUpdated int
	StorageUpdated int
	AccountDeleted int
	StorageDeleted int
}

// New creates a new state from a given trie.
func New(root common.Hash, db StateDatabase, snaps *snapshot.Tree) (*StateDB, error) {
	tr, err := db.OpenTrie(root)
	if err != nil {
		return nil, err
	}
	sdb := &StateDB{
		db:                  db,
		trie:                tr,
		originalRoot:        root,
		snaps:               snaps,
		stateObjects:        make(map[common.Address]*stateObject),
		stateObjectsPending: make(map[common.Address]struct{}),
		stateObjectsDirty:   make(map[common.Address]struct{}),
		logs:                make(map[common.Hash][]*model.Log),
		preimages:           make(map[common.Hash][]byte),
		journal:             newJournal(),
		accessList:          newAccessList(),
		hasher:              crypto.NewKeccakState(),
	}
	if sdb.snaps != nil {
		if sdb.snap = sdb.snaps.Snapshot(root); sdb.snap != nil {
			sdb.snapDestructs = make(map[common.Hash]struct{})
			sdb.snapAccounts = make(map[common.Hash][]byte)
			sdb.snapStorage = make(map[common.Hash]map[common.Hash][]byte)
		}
	}
	return sdb, nil
}

// StartPrefetcher initializes a new trie prefetcher to pull in nodes from the
// state trie concurrently while the state is mutated so that when we reach the
// commit phase, most of the needed data is already hot.
func (stateDB *StateDB) StartPrefetcher(namespace string) {
	if stateDB.prefetcher != nil {
		stateDB.prefetcher.close()
		stateDB.prefetcher = nil
	}
	if stateDB.snap != nil {
		stateDB.prefetcher = newTriePrefetcher(stateDB.db, stateDB.originalRoot, namespace)
	}
}

// StopPrefetcher terminates a running prefetcher and reports any leftover stats
// from the gathered metrics.
func (stateDB *StateDB) StopPrefetcher() {
	if stateDB.prefetcher != nil {
		stateDB.prefetcher.close()
		stateDB.prefetcher = nil
	}
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

func (stateDB *StateDB) AddLog(log *model.Log) {
	stateDB.journal.append(addLogChange{txhash: stateDB.thash})

	log.TxHash = stateDB.thash
	log.TxIndex = uint(stateDB.txIndex)
	log.Index = stateDB.logSize
	stateDB.logs[stateDB.thash] = append(stateDB.logs[stateDB.thash], log)
	stateDB.logSize++
}

func (stateDB *StateDB) GetLogs(hash common.Hash, blockHash common.Hash) []*model.Log {
	logs := stateDB.logs[hash]
	for _, l := range logs {
		l.BlockHash = blockHash
	}
	return logs
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

// GetBalance retrieves the balance from the given address or 0 if object not found
func (stateDB *StateDB) GetBalance(addr common.Address) *big.Int {
	stateObj := stateDB.getStateObject(addr)
	if stateObj != nil {
		return stateObj.Balance()
	}
	return common.Big0
}

func (stateDB *StateDB) GetNonce(addr common.Address) uint64 {
	stateObj := stateDB.getStateObject(addr)
	if stateObj != nil {
		return stateObj.Nonce()
	}

	return 0
}

// TxIndex returns the current transaction index set by Prepare.
func (stateDB *StateDB) TxIndex() int {
	return stateDB.txIndex
}

func (stateDB *StateDB) GetCode(addr common.Address) []byte {
	stateObj := stateDB.getStateObject(addr)
	if stateObj != nil {
		return stateObj.Code(stateDB.db)
	}
	return nil
}

func (stateDB *StateDB) GetCodeSize(addr common.Address) int {
	stateObj := stateDB.getStateObject(addr)
	if stateObj != nil {
		return stateObj.CodeSize(stateDB.db)
	}
	return 0
}

func (stateDB *StateDB) GetCodeHash(addr common.Address) common.Hash {
	stateObj := stateDB.getStateObject(addr)
	if stateObj == nil {
		return common.Hash{}
	}
	return common.BytesToHash(stateObj.CodeHash())
}

// GetState retrieves a value from the given account's storage trie.
func (stateDB *StateDB) GetState(addr common.Address, bhash common.Hash) common.Hash {
	stateObj := stateDB.getStateObject(addr)
	if stateObj != nil {
		return stateObj.GetState(stateDB.db, bhash)
	}
	return common.Hash{}
}

// GetProof returns the Merkle proof for a given account.
func (stateDB *StateDB) GetProof(addr common.Address) ([][]byte, error) {
	return stateDB.GetProofByHash(crypto.Keccak256Hash(addr.Bytes()))
}

// GetProofByHash returns the Merkle proof for a given account.
func (stateDB *StateDB) GetProofByHash(addrHash common.Hash) ([][]byte, error) {
	var proof proofList
	err := stateDB.trie.Prove(addrHash[:], 0, &proof)
	return proof, err
}

// GetStorageProof returns the Merkle proof for given storage slot.
func (stateDB *StateDB) GetStorageProof(a common.Address, key common.Hash) ([][]byte, error) {
	var proof proofList
	trieObj := stateDB.StorageTrie(a)
	if trieObj == nil {
		return proof, errors.New("storage trie for requested address does not exist")
	}
	err := trieObj.Prove(crypto.Keccak256(key.Bytes()), 0, &proof)
	return proof, err
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (stateDB *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	stateObj := stateDB.getStateObject(addr)
	if stateObj != nil {
		return stateObj.GetCommittedState(stateDB.db, hash)
	}
	return common.Hash{}
}

// Database retrieves the low level database supporting the lower level trie ops.
func (stateDB *StateDB) Database() StateDatabase {
	return stateDB.db
}

// StorageTrie returns the storage trie of an account.
// The return value is a copy and is nil for non-existent accounts.
func (stateDB *StateDB) StorageTrie(addr common.Address) Trie {
	stateObj := stateDB.getStateObject(addr)
	if stateObj == nil {
		return nil
	}
	cpy := stateObj.deepCopy(stateDB)
	cpy.updateTrie(stateDB.db)
	return cpy.getTrie(stateDB.db)
}

func (stateDB *StateDB) HasSuicided(addr common.Address) bool {
	stateObj := stateDB.getStateObject(addr)
	if stateObj != nil {
		return stateObj.suicided
	}
	return false
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (stateDB *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	stateObj := stateDB.GetOrNewStateObject(addr)
	if stateObj != nil {
		stateObj.AddBalance(amount)
		log.Debugf("AddBalance: addr=%x, amount=%d, balance=%d", addr, amount, stateObj.Balance())
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (stateDB *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	stateObj := stateDB.GetOrNewStateObject(addr)
	if stateObj != nil {
		stateObj.SubBalance(amount)
		log.Debugf("SubBalance: addr=%x, amount=%d, balance=%d", addr, amount, stateObj.Balance())
	}
}

func (stateDB *StateDB) SetBalance(addr common.Address, amount *big.Int) {
	stateObj := stateDB.GetOrNewStateObject(addr)
	if stateObj != nil {
		stateObj.SetBalance(amount)
		log.Debugf("SetBalance: addr=%x, amount=%d, balance=%d", addr, amount, stateObj.Balance())
	}
}

func (stateDB *StateDB) SetNonce(addr common.Address, nonce uint64) {
	stateObj := stateDB.GetOrNewStateObject(addr)
	if stateObj != nil {
		stateObj.SetNonce(nonce)
		log.Debugf("SetNonce: addr=%x, nonce=%d, balance=%d", addr, nonce, stateObj.Balance())
	}
}

func (stateDB *StateDB) SetCode(addr common.Address, code []byte) {
	stateObj := stateDB.GetOrNewStateObject(addr)
	if stateObj != nil {
		codeHash := crypto.Keccak256Hash(code)
		stateObj.SetCode(codeHash, code)
		log.Debugf("SetCode: addr=%x, codeHash=%x, code=%+v, balance=%d", addr, codeHash, code, stateObj.Balance())
	}
}

func (stateDB *StateDB) SetState(addr common.Address, key, value common.Hash) {
	stateObj := stateDB.GetOrNewStateObject(addr)
	if stateObj != nil {
		stateObj.SetState(stateDB.db, key, value)
		log.Debugf("SetState: addr=%x, key=%x, value=%x, balance=%d", addr, key, value, stateObj.Balance())
	}
}

// SetStorage replaces the entire storage for the specified account with given
// storage. This function should only be used for debugging.
func (stateDB *StateDB) SetStorage(addr common.Address, storage map[common.Hash]common.Hash) {
	stateObj := stateDB.GetOrNewStateObject(addr)
	if stateObj != nil {
		stateObj.SetStorage(storage)
	}
}

// Suicide marks the given account as suicided.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (stateDB *StateDB) Suicide(addr common.Address) bool {
	stateObj := stateDB.getStateObject(addr)
	if stateObj == nil {
		return false
	}
	stateDB.journal.append(suicideChange{
		account:     &addr,
		prev:        stateObj.suicided,
		prevbalance: new(big.Int).Set(stateObj.Balance()),
	})
	stateObj.markSuicided()
	stateObj.data.Balance = new(big.Int)

	return true
}

//
// Setting, updating & deleting state object methods.
//

// updateStateObject writes the given object to the trie.
func (stateDB *StateDB) updateStateObject(stateObj *stateObject) {
	// Track the amount of time wasted on updating the account from the trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountUpdates += time.Since(start) }(time.Now())
	}
	// Encode the account and update the account trie
	addr := stateObj.Address()
	if err := stateDB.trie.TryUpdateAccount(addr[:], &stateObj.data); err != nil {
		stateDB.setError(fmt.Errorf("updateStateObject (%x) error: %v", addr[:], err))
	}

	// If state snapshotting is active, cache the data til commit. Note, this
	// update mechanism is not symmetric to the deletion, because whereas it is
	// enough to track account updates at commit time, deletions need tracking
	// at transaction boundary level to ensure we capture state clearing.
	if stateDB.snap != nil {
		stateDB.snapAccounts[stateObj.addrHash] = snapshot.SlimAccountRLP(stateObj.data.Nonce, stateObj.data.Balance, stateObj.data.Root, stateObj.data.CodeHash)
	}
}

// deleteStateObject removes the given object from the state trie.
func (stateDB *StateDB) deleteStateObject(stateObj *stateObject) {
	// Track the amount of time wasted on deleting the account from the trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountUpdates += time.Since(start) }(time.Now())
	}
	// Delete the account from the trie
	addr := stateObj.Address()
	if err := stateDB.trie.TryDelete(addr[:]); err != nil {
		stateDB.setError(fmt.Errorf("deleteStateObject (%x) error: %v", addr[:], err))
	}
}

// getStateObject retrieves a state object given by the address, returning nil if
// the object is not found or was deleted in this execution context. If you need
// to differentiate between non-existent/just-deleted, use getDeletedStateObject.
func (stateDB *StateDB) getStateObject(addr common.Address) *stateObject {
	if stateObj := stateDB.getDeletedStateObject(addr); stateObj != nil && !stateObj.deleted {
		return stateObj
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
	// If no live objects are available, attempt to use snapshots
	var data *model.StateAccount
	if stateDB.snap != nil {
		start := time.Now()
		acc, err := stateDB.snap.Account(crypto.HashData(stateDB.hasher, addr.Bytes()))
		if metrics.EnabledExpensive {
			stateDB.SnapshotAccountReads += time.Since(start)
		}
		if err == nil {
			if acc == nil {
				return nil
			}
			data = &model.StateAccount{
				Nonce:    acc.Nonce,
				Balance:  acc.Balance,
				CodeHash: acc.CodeHash,
				Root:     common.BytesToHash(acc.Root),
			}
			if len(data.CodeHash) == 0 {
				data.CodeHash = emptyCodeHash
			}
			if data.Root == (common.Hash{}) {
				data.Root = emptyRoot
			}
		}
	}
	// If snapshot unavailable or reading from it failed, load from the database
	if data == nil {
		start := time.Now()
		enc, err := stateDB.trie.TryGet(addr.Bytes())
		if metrics.EnabledExpensive {
			stateDB.AccountReads += time.Since(start)
		}
		if err != nil {
			stateDB.setError(fmt.Errorf("getDeleteStateObject (%x) error: %v", addr.Bytes(), err))
			return nil
		}
		if len(enc) == 0 {
			return nil
		}
		data = new(model.StateAccount)
		if err := rlp.DecodeBytes(enc, data); err != nil {
			log.Error("Failed to decode state object", "addr", addr, "err", err)
			return nil
		}
	}
	// Insert into the live set.
	stateObj := newObject(stateDB, addr, *data)
	stateDB.setStateObject(stateObj)
	return stateObj
}

func (stateDB *StateDB) setStateObject(stateObj *stateObject) {
	stateDB.stateObjects[stateObj.Address()] = stateObj
}

// GetOrNewStateObject retrieves a state object or create a new state object if nil.
func (stateDB *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	stateObj := stateDB.getStateObject(addr)
	if stateObj == nil {
		stateObj, _ = stateDB.createObject(addr)

		log.Debugf("StateDB create new stateObject. addr:%x", addr)
	}
	return stateObj
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (stateDB *StateDB) createObject(addr common.Address) (newObj, prevObj *stateObject) {
	prevObj = stateDB.getDeletedStateObject(addr) // Note, prev might have been deleted, we need that!

	var prevdestruct bool
	if stateDB.snap != nil && prevObj != nil {
		_, prevdestruct = stateDB.snapDestructs[prevObj.addrHash]
		if !prevdestruct {
			stateDB.snapDestructs[prevObj.addrHash] = struct{}{}
		}
	}
	newObj = newObject(stateDB, addr, model.StateAccount{})
	if prevObj == nil {
		stateDB.journal.append(createObjectChange{account: &addr})
	} else {
		stateDB.journal.append(resetObjectChange{prev: prevObj, prevdestruct: prevdestruct})
	}
	stateDB.setStateObject(newObj)
	if prevObj != nil && !prevObj.deleted {
		return newObj, prevObj
	}
	return newObj, nil
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
			_, content, _, err := rlp.Split(it.Value)
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
		originalRoot:        stateDB.originalRoot,
		stateObjects:        make(map[common.Address]*stateObject, len(stateDB.journal.dirties)),
		stateObjectsPending: make(map[common.Address]struct{}, len(stateDB.stateObjectsPending)),
		stateObjectsDirty:   make(map[common.Address]struct{}, len(stateDB.journal.dirties)),
		refund:              stateDB.refund,
		logs:                make(map[common.Hash][]*model.Log, len(stateDB.logs)),
		logSize:             stateDB.logSize,
		preimages:           make(map[common.Hash][]byte, len(stateDB.preimages)),
		journal:             newJournal(),
		hasher:              crypto.NewKeccakState(),
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
	// Do we need to copy the access list? In practice: No. At the start of a
	// transaction, the access list is empty. In practice, we only ever copy state
	// _between_ transactions/blocks, never in the middle of a transaction.
	// However, it doesn't cost us much to copy an empty list, so we do it anyway
	// to not blow up if we ever decide copy it in the middle of a transaction
	state.accessList = stateDB.accessList.Copy()

	// If there's a prefetcher running, make an inactive copy of it that can
	// only access data but does not actively preload (since the user will not
	// know that they need to explicitly terminate an active copy).
	if stateDB.prefetcher != nil {
		state.prefetcher = stateDB.prefetcher.copy()
	}
	if stateDB.snaps != nil {
		// In order for the miner to be able to use and make additions
		// to the snapshot tree, we need to copy that aswell.
		// Otherwise, any block mined by ourselves will cause gaps in the tree,
		// and force the miner to operate trie-backed only
		state.snaps = stateDB.snaps
		state.snap = stateDB.snap
		// deep copy needed
		state.snapDestructs = make(map[common.Hash]struct{})
		for k, v := range stateDB.snapDestructs {
			state.snapDestructs[k] = v
		}
		state.snapAccounts = make(map[common.Hash][]byte)
		for k, v := range stateDB.snapAccounts {
			state.snapAccounts[k] = v
		}
		state.snapStorage = make(map[common.Hash]map[common.Hash][]byte)
		for k, v := range stateDB.snapStorage {
			temp := make(map[common.Hash][]byte)
			for kk, vv := range v {
				temp[kk] = vv
			}
			state.snapStorage[k] = temp
		}
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
	snapshotObj := stateDB.validRevisions[idx].journalIndex

	// Replay the journal to undo changes and remove invalidated snapshots
	stateDB.journal.revert(stateDB, snapshotObj)
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
	log.Debugf("StateDB Finalise. stateObjects: %d, dirties: %d", len(stateDB.stateObjects), len(stateDB.journal.dirties))

	addressesToPrefetch := make([][]byte, 0, len(stateDB.journal.dirties))
	for addr := range stateDB.journal.dirties {
		stateObj, exist := stateDB.stateObjects[addr]
		if !exist {
			// ripeMD is 'touched' at block 1714175, in tx 0x1237f737031e40bcde4a8b7e717b2d15e3ecadfe49bb1bbc71ee9deb09c6fcf2
			// That tx goes out of gas, and although the notion of 'touched' does not exist there, the
			// touch-event will still be recorded in the journal. Since ripeMD is a special snowflake,
			// it will persist in the journal even though the journal is reverted. In this special circumstance,
			// it may exist in `s.journal.dirties` but not in `s.stateObjects`.
			// Thus, we can safely ignore it here
			continue
		}
		if stateObj.suicided || (deleteEmptyObjects && stateObj.empty()) {
			stateObj.deleted = true

			// If state snapshotting is active, also mark the destruction there.
			// Note, we can't do this only at the end of a block because multiple
			// transactions within the same block might self destruct and then
			// ressurrect an account; but the snapshotter needs both events.
			if stateDB.snap != nil {
				stateDB.snapDestructs[stateObj.addrHash] = struct{}{} // We need to maintain account deletions explicitly (will remain set indefinitely)
				delete(stateDB.snapAccounts, stateObj.addrHash)       // Clear out any previously updated account data (may be recreated via a ressurrect)
				delete(stateDB.snapStorage, stateObj.addrHash)        // Clear out any previously updated storage data (may be recreated via a ressurrect)
			}
		} else {
			stateObj.finalise(true) // Prefetch slots in the background
		}
		stateDB.stateObjectsPending[addr] = struct{}{}
		stateDB.stateObjectsDirty[addr] = struct{}{}

		// At this point, also ship the address off to the precacher. The precacher
		// will start loading tries, and when the change is eventually committed,
		// the commit-phase will be a lot faster
		addressesToPrefetch = append(addressesToPrefetch, common.CopyBytes(addr[:])) // Copy needed for closure
	}
	if stateDB.prefetcher != nil && len(addressesToPrefetch) > 0 {
		stateDB.prefetcher.prefetch(common.Hash{}, stateDB.originalRoot, addressesToPrefetch)
	}
	// Invalidate journal because reverting across transactions is not allowed.
	stateDB.clearJournalAndRefund()
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (stateDB *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	log.Debugf("StateDB IntermediateRoot start. hash:%x, stateObjects:%d, deleteEmptyObjects:%v", stateDB.originalRoot, len(stateDB.stateObjects), deleteEmptyObjects)

	// Finalise all the dirty storage states and write them into the tries
	stateDB.Finalise(deleteEmptyObjects)

	// If there was a trie prefetcher operating, it gets aborted and irrevocably
	// modified after we start retrieving tries. Remove it from the statedb after
	// this round of use.
	//
	// This is weird pre-byzantium since the first tx runs with a prefetcher and
	// the remainder without, but pre-byzantium even the initial prefetcher is
	// useless, so no sleep lost.
	prefetcher := stateDB.prefetcher
	if stateDB.prefetcher != nil {
		defer func() {
			stateDB.prefetcher.close()
			stateDB.prefetcher = nil
		}()
	}
	// Although naively it makes sense to retrieve the account trie and then do
	// the contract storage and account updates sequentially, that short circuits
	// the account prefetcher. Instead, let's process all the storage updates
	// first, giving the account prefeches just a few more milliseconds of time
	// to pull useful data from disk.
	for addr := range stateDB.stateObjectsPending {
		if stateObj := stateDB.stateObjects[addr]; !stateObj.deleted {
			stateObj.updateRoot(stateDB.db)
		}
	}
	// Now we're about to start to write changes to the trie. The trie is so far
	// _untouched_. We can check with the prefetcher, if it can give us a trie
	// which has the same root, but also has some content loaded into it.
	if prefetcher != nil {
		if trieObj := prefetcher.trie(common.Hash{}, stateDB.originalRoot); trieObj != nil {
			stateDB.trie = trieObj
		}
	}
	usedAddrs := make([][]byte, 0, len(stateDB.stateObjectsPending))
	for addr := range stateDB.stateObjectsPending {
		if stateObj := stateDB.stateObjects[addr]; stateObj.deleted {
			stateDB.deleteStateObject(stateObj)
			stateDB.AccountDeleted += 1
		} else {
			stateDB.updateStateObject(stateObj)
			stateDB.AccountUpdated += 1
		}
		usedAddrs = append(usedAddrs, common.CopyBytes(addr[:])) // Copy needed for closure
	}
	if prefetcher != nil {
		prefetcher.used(common.Hash{}, stateDB.originalRoot, usedAddrs)
	}
	if len(stateDB.stateObjectsPending) > 0 {
		stateDB.stateObjectsPending = make(map[common.Address]struct{})
	}
	// Track the amount of time wasted on hashing the account trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateDB.AccountHashes += time.Since(start) }(time.Now())
	}
	hash := stateDB.trie.Hash()

	log.Debugf("StateDB IntermediateRoot end. hash:%x, stateObjects:%d, deleteEmptyObjects:%v", hash, len(stateDB.stateObjects), deleteEmptyObjects)
	return hash
}

// Prepare sets the current transaction hash and index which are
// used when the EVM emits new state logs.
func (stateDB *StateDB) Prepare(thash common.Hash, ti int) {
	stateDB.thash = thash
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
	log.Debugf("StateDB Commit start. hashRoot:%x, stateObjects:%d, deleteEmptyObjects:%v", root, len(stateDB.stateObjects), deleteEmptyObjects)

	if stateDB.dbErr != nil {
		return common.Hash{}, fmt.Errorf("commit aborted due to earlier error: %v", stateDB.dbErr)
	}
	// Finalize any pending changes and merge everything into the tries
	stateDB.IntermediateRoot(deleteEmptyObjects)

	// Commit objects to the trie, measuring the elapsed time
	var storageCommitted int
	codeWriter := stateDB.db.TrieDB().DiskDB().NewBatch()
	for addr := range stateDB.stateObjectsDirty {
		if stateObj := stateDB.stateObjects[addr]; !stateObj.deleted {
			// Write any contract code associated with the state object
			if stateObj.code != nil && stateObj.dirtyCode {
				rawdb.WriteCode(codeWriter, common.BytesToHash(stateObj.CodeHash()), stateObj.code)
				stateObj.dirtyCode = false
			}
			// Write any storage changes in the state object to its storage trie
			committed, err := stateObj.CommitTrie(stateDB.db)
			if err != nil {
				return common.Hash{}, err
			}
			storageCommitted += committed
		}
	}
	if len(stateDB.stateObjectsDirty) > 0 {
		stateDB.stateObjectsDirty = make(map[common.Address]struct{})
	}
	if codeWriter.ValueSize() > 0 {
		if err := codeWriter.Write(); err != nil {
			log.Critical("Failed to commit dirty codes", "error", err)
		}
	}
	// Write the account trie changes, measuing the amount of wasted time
	var start time.Time
	if metrics.EnabledExpensive {
		start = time.Now()
	}
	// The onleaf func is called _serially_, so we can reuse the same account
	// for unmarshalling every time.
	var account model.StateAccount
	root, accountCommitted, err := stateDB.trie.Commit(func(_ [][]byte, _ []byte, leaf []byte, parent common.Hash) error {
		if err := rlp.DecodeBytes(leaf, &account); err != nil {
			return nil
		}
		if account.Root != emptyRoot {
			stateDB.db.TrieDB().Reference(account.Root, parent)
		}
		return nil
	})
	if err != nil {
		return common.Hash{}, err
	}
	if metrics.EnabledExpensive {
		stateDB.AccountCommits += time.Since(start)

		accountUpdatedMeter.Mark(int64(stateDB.AccountUpdated))
		storageUpdatedMeter.Mark(int64(stateDB.StorageUpdated))
		accountDeletedMeter.Mark(int64(stateDB.AccountDeleted))
		storageDeletedMeter.Mark(int64(stateDB.StorageDeleted))
		accountCommittedMeter.Mark(int64(accountCommitted))
		storageCommittedMeter.Mark(int64(storageCommitted))
		stateDB.AccountUpdated, stateDB.AccountDeleted = 0, 0
		stateDB.StorageUpdated, stateDB.StorageDeleted = 0, 0
	}
	// If snapshotting is enabled, update the snapshot tree with this new version
	if stateDB.snap != nil {
		if metrics.EnabledExpensive {
			defer func(start time.Time) { stateDB.SnapshotCommits += time.Since(start) }(time.Now())
		}
		// Only update if there's a state transition (skip empty Clique blocks)
		if parent := stateDB.snap.Root(); parent != root {
			if err := stateDB.snaps.Update(root, parent, stateDB.snapDestructs, stateDB.snapAccounts, stateDB.snapStorage); err != nil {
				log.Warning("Failed to update snapshot tree", "from", parent, "to", root, "err", err)
			}
			// Keep 128 diff layers in the memory, persistent layer is 129th.
			// - head layer is paired with HEAD state
			// - head-1 layer is paired with HEAD-1 state
			// - head-127 layer(bottom-most diff layer) is paired with HEAD-127 state
			if err := stateDB.snaps.Cap(root, 128); err != nil {
				log.Warning("Failed to cap snapshot tree", "root", root, "layers", 128, "err", err)
			}
		}
		stateDB.snap, stateDB.snapDestructs, stateDB.snapAccounts, stateDB.snapStorage = nil, nil, nil, nil
	}
	stateDB.originalRoot = root
	log.Debugf("StateDB Commit end. hashRoot:%x, stateObjects:%d, deleteEmptyObjects:%v", root, len(stateDB.stateObjects), deleteEmptyObjects)
	return root, err
}

// PrepareAccessList handles the preparatory steps for executing a state transition with
// regards to both EIP-2929 and EIP-2930:
//
// - Add sender to access list (2929)
// - Add destination to access list (2929)
// - Add precompiles to access list (2929)
// - Add the contents of the optional tx access list (2930)
//
// This method should only be called if Berlin/2929+2930 is applicable at the current number.
func (stateDB *StateDB) PrepareAccessList(sender common.Address, dst *common.Address, precompiles []common.Address, list model.AccessList) {
	// Clear out any leftover from previous executions
	stateDB.accessList = newAccessList()

	stateDB.AddAddressToAccessList(sender)
	if dst != nil {
		stateDB.AddAddressToAccessList(*dst)
		// If it's a create-tx, the destination will be added inside evm.create
	}
	for _, addr := range precompiles {
		stateDB.AddAddressToAccessList(addr)
	}
	for _, el := range list {
		stateDB.AddAddressToAccessList(el.Address)
		for _, key := range el.StorageKeys {
			stateDB.AddSlotToAccessList(el.Address, key)
		}
	}
}

// AddAddressToAccessList adds the given address to the access list
func (stateDB *StateDB) AddAddressToAccessList(addr common.Address) {
	if stateDB.accessList.AddAddress(addr) {
		stateDB.journal.append(accessListAddAccountChange{&addr})
	}
}

// AddSlotToAccessList adds the given (address, slot)-tuple to the access list
func (stateDB *StateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	addrMod, slotMod := stateDB.accessList.AddSlot(addr, slot)
	if addrMod {
		// In practice, this should not happen, since there is no way to enter the
		// scope of 'address' without having the 'address' become already added
		// to the access list (via call-variant, create, etc).
		// Better safe than sorry, though
		stateDB.journal.append(accessListAddAccountChange{&addr})
	}
	if slotMod {
		stateDB.journal.append(accessListAddSlotChange{
			address: &addr,
			slot:    &slot,
		})
	}
}

// AddressInAccessList returns true if the given address is in the access list.
func (stateDB *StateDB) AddressInAccessList(addr common.Address) bool {
	return stateDB.accessList.ContainsAddress(addr)
}

// SlotInAccessList returns true if the given (address, slot)-tuple is in the access list.
func (stateDB *StateDB) SlotInAccessList(addr common.Address, slot common.Hash) (addressPresent bool, slotPresent bool) {
	return stateDB.accessList.Contains(addr, slot)
}
