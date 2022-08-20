package state

import (
	"bytes"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/metrics"
	"io"
	"math/big"
	"time"
)

var emptyCodeHash = crypto.Keccak256(nil)

// Code
type Code []byte

func (code Code) String() string {
	return string(code) //strings.Join(Disassemble(code), " ")
}

// Storage
type Storage map[common.Hash]common.Hash

func (storage Storage) String() (str string) {
	for key, value := range storage {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

func (storage Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range storage {
		cpy[key] = value
	}

	return cpy
}

// stateObject represents an Entropy account which is being modified.
//
// The usage pattern is as follows:
// First you need to obtain a state object.
// Account values can be accessed and modified through the object.
// Finally, call CommitTrie to write the modified storage trie into a database.
type stateObject struct {
	address  common.Address
	addrHash common.Hash // hash of entropy address of the account
	data     model.StateAccount
	db       *StateDB

	// DB error.
	// State objects are used by the consensus blockchain and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// Write caches.
	trie Trie // storage trie, which becomes non-nil on first access
	code Code // contract bytecode, which gets set when code is loaded

	originStorage  Storage // Storage cache of original entries to dedup rewrites, reset for every transaction
	pendingStorage Storage // Storage entries that need to be flushed to disk, at the end of an entire block
	dirtyStorage   Storage // Storage entries that have been modified in the current transaction execution
	fakeStorage    Storage // Fake storage which constructed by caller for debugging purpose.

	// Cache flags.
	// When an object is marked suicided it will be delete from the trie
	// during the "update" phase of the state transition.
	dirtyCode bool // true if the code was updated
	suicided  bool
	deleted   bool
}

// empty returns whether the account is considered empty.
func (stateObj *stateObject) empty() bool {
	return stateObj.data.Nonce == 0 && stateObj.data.Balance.Sign() == 0 && bytes.Equal(stateObj.data.CodeHash, emptyCodeHash)
}

// newObject creates a state object.
func newObject(db *StateDB, address common.Address, data model.StateAccount) *stateObject {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}
	if data.CodeHash == nil {
		data.CodeHash = emptyCodeHash
	}
	if data.Root == (common.Hash{}) {
		data.Root = emptyRoot
	}
	return &stateObject{
		db:             db,
		address:        address,
		addrHash:       crypto.Keccak256Hash(address[:]),
		data:           data,
		originStorage:  make(Storage),
		pendingStorage: make(Storage),
		dirtyStorage:   make(Storage),
	}
}

// EncodeRLP implements rlp.Encoder.
func (stateObj *stateObject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &stateObj.data)
}

// setError remembers the first non-nil error it is called with.
func (stateObj *stateObject) setError(err error) {
	if stateObj.dbErr == nil {
		stateObj.dbErr = err
	}
}

func (stateObj *stateObject) markSuicided() {
	stateObj.suicided = true
}

func (stateObj *stateObject) touch() {
	stateObj.db.journal.append(touchChange{
		account: &stateObj.address,
	})
	if stateObj.address == ripemd {
		// Explicitly put it in the dirty-cache, which is otherwise generated from
		// flattened journals.
		stateObj.db.journal.dirty(stateObj.address)
	}
}

func (stateObj *stateObject) getTrie(db StateDatabase) Trie {
	if stateObj.trie == nil {
		// Try fetching from prefetcher first
		// We don't prefetch empty tries
		if stateObj.data.Root != emptyRoot && stateObj.db.prefetcher != nil {
			// When the miner is creating the pending state, there is no
			// prefetcher
			stateObj.trie = stateObj.db.prefetcher.trie(stateObj.addrHash, stateObj.data.Root)
		}
		if stateObj.trie == nil {
			var err error
			stateObj.trie, err = db.OpenStorageTrie(stateObj.addrHash, stateObj.data.Root)
			if err != nil {
				stateObj.trie, _ = db.OpenStorageTrie(stateObj.addrHash, common.Hash{})
				stateObj.setError(fmt.Errorf("can't create storage trie: %v", err))
			}
		}
	}
	return stateObj.trie
}

// GetState retrieves a value from the account storage trie.
func (stateObj *stateObject) GetState(db StateDatabase, key common.Hash) common.Hash {
	// If the fake storage is set, only lookup the state here(in the debugging mode)
	if stateObj.fakeStorage != nil {
		return stateObj.fakeStorage[key]
	}
	// If we have a dirty value for this state entry, return it
	value, dirty := stateObj.dirtyStorage[key]
	if dirty {
		return value
	}
	// Otherwise return the entry's original value
	return stateObj.GetCommittedState(db, key)
}

// GetCommittedState retrieves a value from the committed account storage trie.
func (stateObj *stateObject) GetCommittedState(db StateDatabase, key common.Hash) common.Hash {
	// If the fake storage is set, only lookup the state here(in the debugging mode)
	if stateObj.fakeStorage != nil {
		return stateObj.fakeStorage[key]
	}
	// If we have a pending write or clean cached, return that
	if value, pending := stateObj.pendingStorage[key]; pending {
		return value
	}
	if value, cached := stateObj.originStorage[key]; cached {
		return value
	}
	// If no live objects are available, attempt to use snapshots
	var (
		enc []byte
		err error
	)
	if stateObj.db.snap != nil {
		// If the object was destructed in *this* block (and potentially resurrected),
		// the storage has been cleared out, and we should *not* consult the previous
		// snapshot about any storage values. The only possible alternatives are:
		//   1) resurrect happened, and new slot values were set -- those should
		//      have been handles via pendingStorage above.
		//   2) we don't have new values, and can deliver empty response back
		if _, destructed := stateObj.db.snapDestructs[stateObj.addrHash]; destructed {
			return common.Hash{}
		}
		start := time.Now()
		enc, err = stateObj.db.snap.Storage(stateObj.addrHash, crypto.Keccak256Hash(key.Bytes()))
		if metrics.EnabledExpensive {
			stateObj.db.SnapshotStorageReads += time.Since(start)
		}
	}
	// If the snapshot is unavailable or reading from it fails, load from the database.
	if stateObj.db.snap == nil || err != nil {
		start := time.Now()
		enc, err = stateObj.getTrie(db).TryGet(key.Bytes())
		if metrics.EnabledExpensive {
			stateObj.db.StorageReads += time.Since(start)
		}
		if err != nil {
			stateObj.setError(err)
			return common.Hash{}
		}
	}
	var value common.Hash
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			stateObj.setError(err)
		}
		value.SetBytes(content)
	}
	stateObj.originStorage[key] = value
	return value
}

// SetState updates a value in account storage.
func (stateObj *stateObject) SetState(db StateDatabase, key, value common.Hash) {
	// If the fake storage is set, put the temporary state update here.
	if stateObj.fakeStorage != nil {
		stateObj.fakeStorage[key] = value
		return
	}
	// If the new value is the same as old, don't set
	prev := stateObj.GetState(db, key)
	if prev == value {
		return
	}
	// New value is different, update and journal the change
	stateObj.db.journal.append(storageChange{
		account:  &stateObj.address,
		key:      key,
		prevalue: prev,
	})
	stateObj.setState(key, value)
}

// SetStorage replaces the entire state storage with the given one.
//
// After this function is called, all original state will be ignored and state
// lookup only happens in the fake state storage.
//
// Note this function should only be used for debugging purpose.
func (stateObj *stateObject) SetStorage(storage map[common.Hash]common.Hash) {
	// Allocate fake storage if it's nil.
	if stateObj.fakeStorage == nil {
		stateObj.fakeStorage = make(Storage)
	}
	for key, value := range storage {
		stateObj.fakeStorage[key] = value
	}
	// Don't bother journal since this function should only be used for
	// debugging and the `fake` storage won't be committed to database.
}

func (stateObj *stateObject) setState(key, value common.Hash) {
	stateObj.dirtyStorage[key] = value
}

// finalise moves all dirty storage slots into the pending area to be hashed or
// committed later. It is invoked at the end of every transaction.
func (stateObj *stateObject) finalise(prefetch bool) {
	log.Debugf("stateObject finalise. address:%x, prefetch:%v", stateObj.address, prefetch)

	slotsToPrefetch := make([][]byte, 0, len(stateObj.dirtyStorage))
	for key, value := range stateObj.dirtyStorage {
		stateObj.pendingStorage[key] = value
		if value != stateObj.originStorage[key] {
			slotsToPrefetch = append(slotsToPrefetch, common.CopyBytes(key[:])) // Copy needed for closure
		}
	}
	if stateObj.db.prefetcher != nil && prefetch && len(slotsToPrefetch) > 0 && stateObj.data.Root != emptyRoot {
		stateObj.db.prefetcher.prefetch(stateObj.addrHash, stateObj.data.Root, slotsToPrefetch)
	}
	if len(stateObj.dirtyStorage) > 0 {
		stateObj.dirtyStorage = make(Storage)
	}
}

// updateTrie writes cached storage modifications into the object's storage trie.
// It will return nil if the trie has not been loaded and no changes have been made
func (stateObj *stateObject) updateTrie(db StateDatabase) Trie {
	// Make sure all dirty slots are finalized into the pending storage area
	stateObj.finalise(false) // Don't prefetch anymore, pull directly if need be
	if len(stateObj.pendingStorage) == 0 {
		return stateObj.trie
	}
	// Track the amount of time wasted on updating the storage trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateObj.db.StorageUpdates += time.Since(start) }(time.Now())
	}
	// The snapshot storage map for the object
	var storage map[common.Hash][]byte
	// Insert all the pending updates into the trie
	tr := stateObj.getTrie(db)
	hasher := stateObj.db.hasher

	usedStorage := make([][]byte, 0, len(stateObj.pendingStorage))
	for key, value := range stateObj.pendingStorage {
		// Skip noop changes, persist actual changes
		if value == stateObj.originStorage[key] {
			continue
		}
		stateObj.originStorage[key] = value

		var v []byte
		if (value == common.Hash{}) {
			stateObj.setError(tr.TryDelete(key[:]))
			stateObj.db.StorageDeleted += 1
		} else {
			// Encoding []byte cannot fail, ok to ignore the error.
			v, _ = rlp.EncodeToBytes(common.TrimLeftZeroes(value[:]))
			stateObj.setError(tr.TryUpdate(key[:], v))
			stateObj.db.StorageUpdated += 1
		}
		// If state snapshotting is active, cache the data til commit
		if stateObj.db.snap != nil {
			if storage == nil {
				// Retrieve the old storage map, if available, create a new one otherwise
				if storage = stateObj.db.snapStorage[stateObj.addrHash]; storage == nil {
					storage = make(map[common.Hash][]byte)
					stateObj.db.snapStorage[stateObj.addrHash] = storage
				}
			}
			storage[crypto.HashData(hasher, key[:])] = v // v will be nil if it's deleted
		}
		usedStorage = append(usedStorage, common.CopyBytes(key[:])) // Copy needed for closure
	}
	if stateObj.db.prefetcher != nil {
		stateObj.db.prefetcher.used(stateObj.addrHash, stateObj.data.Root, usedStorage)
	}
	if len(stateObj.pendingStorage) > 0 {
		stateObj.pendingStorage = make(Storage)
	}
	return tr
}

// UpdateRoot sets the trie root to the current root hash of
func (stateObj *stateObject) updateRoot(db StateDatabase) {
	// If nothing changed, don't bother with hashing anything
	if stateObj.updateTrie(db) == nil {
		return
	}
	// Track the amount of time wasted on hashing the storage trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateObj.db.StorageHashes += time.Since(start) }(time.Now())
	}
	stateObj.data.Root = stateObj.trie.Hash()
}

// CommitTrie the storage trie of the object to db.
// This updates the trie root.
func (stateObj *stateObject) CommitTrie(db StateDatabase) (int, error) {
	// If nothing changed, don't bother with hashing anything
	if stateObj.updateTrie(db) == nil {
		return 0, nil
	}
	if stateObj.dbErr != nil {
		return 0, stateObj.dbErr
	}
	// Track the amount of time wasted on committing the storage trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { stateObj.db.StorageCommits += time.Since(start) }(time.Now())
	}
	root, committed, err := stateObj.trie.Commit(nil)
	if err == nil {
		stateObj.data.Root = root
	}
	return committed, err
}

// AddBalance adds amount to s's balance.
// It is used to add funds to the destination account of a transfer.
func (stateObj *stateObject) AddBalance(amount *big.Int) {
	// EIP161: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if stateObj.empty() {
			stateObj.touch()
		}

		return
	}
	stateObj.SetBalance(new(big.Int).Add(stateObj.Balance(), amount))
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (stateObj *stateObject) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	stateObj.SetBalance(new(big.Int).Sub(stateObj.Balance(), amount))
}

func (stateObj *stateObject) SetBalance(amount *big.Int) {
	stateObj.db.journal.append(balanceChange{
		account: &stateObj.address,
		prev:    new(big.Int).Set(stateObj.data.Balance),
	})
	stateObj.setBalance(amount)
}

func (stateObj *stateObject) setBalance(amount *big.Int) {
	stateObj.data.Balance = amount
}

func (stateObj *stateObject) deepCopy(stateDB *StateDB) *stateObject {
	newObj := newObject(stateDB, stateObj.address, stateObj.data)
	if stateObj.trie != nil {
		newObj.trie = stateDB.db.CopyTrie(stateObj.trie)
	}
	newObj.code = stateObj.code
	newObj.dirtyStorage = stateObj.dirtyStorage.Copy()
	newObj.originStorage = stateObj.originStorage.Copy()
	newObj.pendingStorage = stateObj.pendingStorage.Copy()
	newObj.suicided = stateObj.suicided
	newObj.dirtyCode = stateObj.dirtyCode
	newObj.deleted = stateObj.deleted
	return newObj
}

//
// Attribute accessors
//

// Returns the address of the contract/account
func (stateObj *stateObject) Address() common.Address {
	return stateObj.address
}

// Code returns the contract code associated with this object, if any.
func (stateObj *stateObject) Code(db StateDatabase) []byte {
	if stateObj.code != nil {
		return stateObj.code
	}
	if bytes.Equal(stateObj.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(stateObj.addrHash, common.BytesToHash(stateObj.CodeHash()))
	if err != nil {
		stateObj.setError(fmt.Errorf("can't load code hash %x: %v", stateObj.CodeHash(), err))
	}
	stateObj.code = code
	return code
}

// CodeSize returns the size of the contract code associated with this object,
// or zero if none. This method is an almost mirror of Code, but uses a cache
// inside the database to avoid loading codes seen recently.
func (stateObj *stateObject) CodeSize(db StateDatabase) int {
	if stateObj.code != nil {
		return len(stateObj.code)
	}
	if bytes.Equal(stateObj.CodeHash(), emptyCodeHash) {
		return 0
	}
	size, err := db.ContractCodeSize(stateObj.addrHash, common.BytesToHash(stateObj.CodeHash()))
	if err != nil {
		stateObj.setError(fmt.Errorf("can't load code size %x: %v", stateObj.CodeHash(), err))
	}
	return size
}

func (stateObj *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := stateObj.Code(stateObj.db.db)
	stateObj.db.journal.append(codeChange{
		account:  &stateObj.address,
		prevhash: stateObj.CodeHash(),
		prevcode: prevcode,
	})
	stateObj.setCode(codeHash, code)
}

func (stateObj *stateObject) setCode(codeHash common.Hash, code []byte) {
	stateObj.code = code
	stateObj.data.CodeHash = codeHash[:]
	stateObj.dirtyCode = true
}

func (stateObj *stateObject) SetNonce(nonce uint64) {
	stateObj.db.journal.append(nonceChange{
		account: &stateObj.address,
		prev:    stateObj.data.Nonce,
	})
	stateObj.setNonce(nonce)
}

func (stateObj *stateObject) setNonce(nonce uint64) {
	stateObj.data.Nonce = nonce
}

func (stateObj *stateObject) CodeHash() []byte {
	return stateObj.data.CodeHash
}

func (stateObj *stateObject) Balance() *big.Int {
	return stateObj.data.Balance
}

func (stateObj *stateObject) Nonce() uint64 {
	return stateObj.data.Nonce
}

// Never called, but must be present to allow stateObject to be used
// as a evm.Account interface that also satisfies the evm.ContractRef
// interface. Interfaces are awesome.
func (stateObj *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}
