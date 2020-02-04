package state

import (
	"bytes"
	"fmt"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/rlputil"
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
	data     Account
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
func (state *stateObject) empty() bool {
	return state.data.Nonce == 0 && state.data.Balance.Sign() == 0 && bytes.Equal(state.data.CodeHash, emptyCodeHash)
}

// Account is the Entropy consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash // merkle root of the storage trie
	CodeHash []byte
}

// newObject creates a state object.
func newObject(db *StateDB, address common.Address, data Account) *stateObject {
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

// EncodeRLP implements rlputil.Encoder.
func (state *stateObject) EncodeRLP(w io.Writer) error {
	return rlputil.Encode(w, state.data)
}

// setError remembers the first non-nil error it is called with.
func (state *stateObject) setError(err error) {
	if state.dbErr == nil {
		state.dbErr = err
	}
}

func (state *stateObject) markSuicided() {
	state.suicided = true
}

func (state *stateObject) touch() {
	state.db.journal.append(touchChange{
		account: &state.address,
	})
	if state.address == ripemd {
		// Explicitly put it in the dirty-cache, which is otherwise generated from
		// flattened journals.
		state.db.journal.dirty(state.address)
	}
}

func (state *stateObject) getTrie(db StateDatabase) Trie {
	if state.trie == nil {
		var err error
		state.trie, err = db.OpenStorageTrie(state.addrHash, state.data.Root)
		if err != nil {
			state.trie, _ = db.OpenStorageTrie(state.addrHash, common.Hash{})
			state.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return state.trie
}

// GetState retrieves a value from the account storage trie.
func (state *stateObject) GetState(db StateDatabase, key common.Hash) common.Hash {
	// If the fake storage is set, only lookup the state here(in the debugging mode)
	if state.fakeStorage != nil {
		return state.fakeStorage[key]
	}
	// If we have a dirty value for this state entry, return it
	value, dirty := state.dirtyStorage[key]
	if dirty {
		return value
	}
	// Otherwise return the entry's original value
	return state.GetCommittedState(db, key)
}

// GetCommittedState retrieves a value from the committed account storage trie.
func (state *stateObject) GetCommittedState(db StateDatabase, key common.Hash) common.Hash {
	// If the fake storage is set, only lookup the state here(in the debugging mode)
	if state.fakeStorage != nil {
		return state.fakeStorage[key]
	}
	// If we have a pending write or clean cached, return that
	if value, pending := state.pendingStorage[key]; pending {
		return value
	}
	if value, cached := state.originStorage[key]; cached {
		return value
	}
	// Track the amount of time wasted on reading the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { state.db.StorageReads += time.Since(start) }(time.Now())
	}
	// Otherwise load the value from the database
	enc, err := state.getTrie(db).TryGet(key[:])
	if err != nil {
		state.setError(err)
		return common.Hash{}
	}
	var value common.Hash
	if len(enc) > 0 {
		_, content, _, err := rlputil.Split(enc)
		if err != nil {
			state.setError(err)
		}
		value.SetBytes(content)
	}
	state.originStorage[key] = value
	return value
}

// SetState updates a value in account storage.
func (state *stateObject) SetState(db StateDatabase, key, value common.Hash) {
	// If the fake storage is set, put the temporary state update here.
	if state.fakeStorage != nil {
		state.fakeStorage[key] = value
		return
	}
	// If the new value is the same as old, don't set
	prev := state.GetState(db, key)
	if prev == value {
		return
	}
	// New value is different, update and journal the change
	state.db.journal.append(storageChange{
		account:  &state.address,
		key:      key,
		prevalue: prev,
	})
	state.setState(key, value)
}

// SetStorage replaces the entire state storage with the given one.
//
// After this function is called, all original state will be ignored and state
// lookup only happens in the fake state storage.
//
// Note this function should only be used for debugging purpose.
func (state *stateObject) SetStorage(storage map[common.Hash]common.Hash) {
	// Allocate fake storage if it's nil.
	if state.fakeStorage == nil {
		state.fakeStorage = make(Storage)
	}
	for key, value := range storage {
		state.fakeStorage[key] = value
	}
	// Don't bother journal since this function should only be used for
	// debugging and the `fake` storage won't be committed to database.
}

func (state *stateObject) setState(key, value common.Hash) {
	state.dirtyStorage[key] = value
}

// finalise moves all dirty storage slots into the pending area to be hashed or
// committed later. It is invoked at the end of every transaction.
func (state *stateObject) finalise() {
	for key, value := range state.dirtyStorage {
		state.pendingStorage[key] = value
	}
	if len(state.dirtyStorage) > 0 {
		state.dirtyStorage = make(Storage)
	}
}

// updateTrie writes cached storage modifications into the object's storage trie.
func (state *stateObject) updateTrie(db StateDatabase) Trie {
	// Make sure all dirty slots are finalized into the pending storage area
	state.finalise()

	// Track the amount of time wasted on updating the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { state.db.StorageUpdates += time.Since(start) }(time.Now())
	}
	// Insert all the pending updates into the trie
	tr := state.getTrie(db)
	for key, value := range state.pendingStorage {
		// Skip noop changes, persist actual changes
		if value == state.originStorage[key] {
			continue
		}
		state.originStorage[key] = value

		if (value == common.Hash{}) {
			state.setError(tr.TryDelete(key[:]))
			continue
		}
		// Encoding []byte cannot fail, ok to ignore the error.
		v, _ := rlputil.EncodeToBytes(common.TrimLeftZeroes(value[:]))
		state.setError(tr.TryUpdate(key[:], v))
	}
	if len(state.pendingStorage) > 0 {
		state.pendingStorage = make(Storage)
	}
	return tr
}

// UpdateRoot sets the trie root to the current root hash of
func (state *stateObject) updateRoot(db StateDatabase) {
	state.updateTrie(db)

	// Track the amount of time wasted on hashing the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { state.db.StorageHashes += time.Since(start) }(time.Now())
	}
	state.data.Root = state.trie.Hash()
}

// CommitTrie the storage trie of the object to db.
// This updates the trie root.
func (state *stateObject) CommitTrie(db StateDatabase) error {
	state.updateTrie(db)
	if state.dbErr != nil {
		return state.dbErr
	}
	// Track the amount of time wasted on committing the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { state.db.StorageCommits += time.Since(start) }(time.Now())
	}
	root, err := state.trie.Commit(nil)
	if err == nil {
		state.data.Root = root
	}
	return err
}

// AddBalance removes amount from c's balance.
// It is used to add funds to the destination account of a transfer.
func (state *stateObject) AddBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if state.empty() {
			state.touch()
		}

		return
	}
	state.SetBalance(new(big.Int).Add(state.Balance(), amount))
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (state *stateObject) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	state.SetBalance(new(big.Int).Sub(state.Balance(), amount))
}

func (state *stateObject) SetBalance(amount *big.Int) {
	state.db.journal.append(balanceChange{
		account: &state.address,
		prev:    new(big.Int).Set(state.data.Balance),
	})
	state.setBalance(amount)
}

func (state *stateObject) setBalance(amount *big.Int) {
	state.data.Balance = amount
}

// Return the gas back to the origin. Used by the Virtual machine or Closures
func (state *stateObject) ReturnGas(gas *big.Int) {}

func (state *stateObject) deepCopy(stateDB *StateDB) *stateObject {
	stateObject := newObject(stateDB, state.address, state.data)
	if state.trie != nil {
		stateObject.trie = stateDB.db.CopyTrie(state.trie)
	}
	stateObject.code = state.code
	stateObject.dirtyStorage = state.dirtyStorage.Copy()
	stateObject.originStorage = state.originStorage.Copy()
	stateObject.pendingStorage = state.pendingStorage.Copy()
	stateObject.suicided = state.suicided
	stateObject.dirtyCode = state.dirtyCode
	stateObject.deleted = state.deleted
	return stateObject
}

//
// Attribute accessors
//

// Returns the address of the contract/account
func (state *stateObject) Address() common.Address {
	return state.address
}

// Code returns the contract code associated with this object, if any.
func (state *stateObject) Code(db StateDatabase) []byte {
	if state.code != nil {
		return state.code
	}
	if bytes.Equal(state.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(state.addrHash, common.BytesToHash(state.CodeHash()))
	if err != nil {
		state.setError(fmt.Errorf("can't load code hash %x: %v", state.CodeHash(), err))
	}
	state.code = code
	return code
}

func (state *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := state.Code(state.db.db)
	state.db.journal.append(codeChange{
		account:  &state.address,
		prevhash: state.CodeHash(),
		prevcode: prevcode,
	})
	state.setCode(codeHash, code)
}

func (state *stateObject) setCode(codeHash common.Hash, code []byte) {
	state.code = code
	state.data.CodeHash = codeHash[:]
	state.dirtyCode = true
}

func (state *stateObject) SetNonce(nonce uint64) {
	state.db.journal.append(nonceChange{
		account: &state.address,
		prev:    state.data.Nonce,
	})
	state.setNonce(nonce)
}

func (state *stateObject) setNonce(nonce uint64) {
	state.data.Nonce = nonce
}

func (state *stateObject) CodeHash() []byte {
	return state.data.CodeHash
}

func (state *stateObject) Balance() *big.Int {
	return state.data.Balance
}

func (state *stateObject) Nonce() uint64 {
	return state.data.Nonce
}

// Never called, but must be present to allow stateObject to be used
// as a evm.Account interface that also satisfies the evm.ContractRef
// interface. Interfaces are awesome.
func (state *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}
