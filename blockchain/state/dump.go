package state

import (
	"encoding/json"
	"fmt"

	"github.com/entropyio/go-entropy/common"

	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/database/trie"
)

// DumpAccount represents an account in the state
type DumpAccount struct {
	Balance   string                 `json:"balance"`
	Nonce     uint64                 `json:"nonce"`
	Root      string                 `json:"root"`
	CodeHash  string                 `json:"codeHash"`
	Code      string                 `json:"code,omitempty"`
	Storage   map[common.Hash]string `json:"storage,omitempty"`
	Address   *common.Address        `json:"address,omitempty"` // Address only present in iterative (line-by-line) mode
	SecureKey hexutil.Bytes          `json:"key,omitempty"`     // If we don't have address, we can output the key

}

// Dump represents the full dump in a collected format, as one large map
type Dump struct {
	Root     string                         `json:"root"`
	Accounts map[common.Address]DumpAccount `json:"accounts"`
}

// iterativeDump is a 'collector'-implementation which dump output line-by-line iteratively
type iterativeDump json.Encoder

// Collector interface which the state trie calls during iteration
type collector interface {
	onRoot(common.Hash)
	onAccount(common.Address, DumpAccount)
}

func (dump *Dump) onRoot(root common.Hash) {
	dump.Root = fmt.Sprintf("%x", root)
}

func (dump *Dump) onAccount(addr common.Address, account DumpAccount) {
	dump.Accounts[addr] = account
}

func (dump iterativeDump) onAccount(addr common.Address, account DumpAccount) {
	dumpAccount := &DumpAccount{
		Balance:   account.Balance,
		Nonce:     account.Nonce,
		Root:      account.Root,
		CodeHash:  account.CodeHash,
		Code:      account.Code,
		Storage:   account.Storage,
		SecureKey: account.SecureKey,
		Address:   nil,
	}
	if addr != (common.Address{}) {
		dumpAccount.Address = &addr
	}
	(*json.Encoder)(&dump).Encode(dumpAccount)
}
func (dump iterativeDump) onRoot(root common.Hash) {
	(*json.Encoder)(&dump).Encode(struct {
		Root common.Hash `json:"root"`
	}{root})
}

func (stateDB *StateDB) dump(c collector, excludeCode, excludeStorage, excludeMissingPreimages bool) {
	emptyAddress := common.Address{}
	missingPreimages := 0
	c.onRoot(stateDB.trie.Hash())
	it := trie.NewIterator(stateDB.trie.NodeIterator(nil))
	for it.Next() {
		var data Account
		if err := rlputil.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}
		addr := common.BytesToAddress(stateDB.trie.GetKey(it.Key))
		obj := newObject(nil, addr, data)
		account := DumpAccount{
			Balance:  data.Balance.String(),
			Nonce:    data.Nonce,
			Root:     common.Bytes2Hex(data.Root[:]),
			CodeHash: common.Bytes2Hex(data.CodeHash),
		}
		if emptyAddress == addr {
			// Preimage missing
			missingPreimages++
			if excludeMissingPreimages {
				continue
			}
			account.SecureKey = it.Key
		}
		if !excludeCode {
			account.Code = common.Bytes2Hex(obj.Code(stateDB.db))
		}
		if !excludeStorage {
			account.Storage = make(map[common.Hash]string)
			storageIt := trie.NewIterator(obj.getTrie(stateDB.db).NodeIterator(nil))
			for storageIt.Next() {
				account.Storage[common.BytesToHash(stateDB.trie.GetKey(storageIt.Key))] = common.Bytes2Hex(storageIt.Value)
			}
		}
		c.onAccount(addr, account)
	}
	if missingPreimages > 0 {
		stateLog.Warning("Dump incomplete due to missing preimages", "missing", missingPreimages)
	}
}

// RawDump returns the entire state an a single large object
func (stateDB *StateDB) RawDump(excludeCode, excludeStorage, excludeMissingPreimages bool) Dump {
	dump := &Dump{
		Accounts: make(map[common.Address]DumpAccount),
	}
	stateDB.dump(dump, excludeCode, excludeStorage, excludeMissingPreimages)
	return *dump
}

// Dump returns a JSON string representing the entire state as a single json-object
func (stateDB *StateDB) Dump(excludeCode, excludeStorage, excludeMissingPreimages bool) []byte {
	dump := stateDB.RawDump(excludeCode, excludeStorage, excludeMissingPreimages)
	jsonObj, err := json.MarshalIndent(dump, "", "    ")
	if err != nil {
		fmt.Println("dump err", err)
	}
	return jsonObj
}

// IterativeDump dumps out accounts as json-objects, delimited by linebreaks on stdout
func (stateDB *StateDB) IterativeDump(excludeCode, excludeStorage, excludeMissingPreimages bool, output *json.Encoder) {
	stateDB.dump(iterativeDump(*output), excludeCode, excludeStorage, excludeMissingPreimages)
}
