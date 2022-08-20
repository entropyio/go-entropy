package state

import (
	"encoding/json"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/database/trie"
	"time"
)

// DumpConfig is a set of options to control what portions of the statewill be
// iterated and collected.
type DumpConfig struct {
	SkipCode          bool
	SkipStorage       bool
	OnlyWithAddresses bool
	Start             []byte
	Max               uint64
}

// DumpCollector interface which the state trie calls during iteration
type DumpCollector interface {
	// OnRoot is called with the state root
	OnRoot(common.Hash)
	// OnAccount is called once for each account in the trie
	OnAccount(common.Address, DumpAccount)
}

// DumpAccount represents an account in the state.
type DumpAccount struct {
	Balance   string                 `json:"balance"`
	Nonce     uint64                 `json:"nonce"`
	Root      hexutil.Bytes          `json:"root"`
	CodeHash  hexutil.Bytes          `json:"codeHash"`
	Code      hexutil.Bytes          `json:"code,omitempty"`
	Storage   map[common.Hash]string `json:"storage,omitempty"`
	Address   *common.Address        `json:"address,omitempty"` // Address only present in iterative (line-by-line) mode
	SecureKey hexutil.Bytes          `json:"key,omitempty"`     // If we don't have address, we can output the key

}

// Dump represents the full dump in a collected format, as one large map.
type Dump struct {
	Root     string                         `json:"root"`
	Accounts map[common.Address]DumpAccount `json:"accounts"`
}

// OnRoot implements DumpCollector interface
func (d *Dump) OnRoot(root common.Hash) {
	d.Root = fmt.Sprintf("%x", root)
}

// OnAccount implements DumpCollector interface
func (d *Dump) OnAccount(addr common.Address, account DumpAccount) {
	d.Accounts[addr] = account
}

// IteratorDump is an implementation for iterating over data.
type IteratorDump struct {
	Root     string                         `json:"root"`
	Accounts map[common.Address]DumpAccount `json:"accounts"`
	Next     []byte                         `json:"next,omitempty"` // nil if no more accounts
}

// OnRoot implements DumpCollector interface
func (d *IteratorDump) OnRoot(root common.Hash) {
	d.Root = fmt.Sprintf("%x", root)
}

// OnAccount implements DumpCollector interface
func (d *IteratorDump) OnAccount(addr common.Address, account DumpAccount) {
	d.Accounts[addr] = account
}

// iterativeDump is a DumpCollector-implementation which dumps output line-by-line iteratively.
type iterativeDump struct {
	*json.Encoder
}

// OnAccount implements DumpCollector interface
func (d iterativeDump) OnAccount(addr common.Address, account DumpAccount) {
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
	d.Encode(dumpAccount)
}

// OnRoot implements DumpCollector interface
func (d iterativeDump) OnRoot(root common.Hash) {
	d.Encode(struct {
		Root common.Hash `json:"root"`
	}{root})
}

// DumpToCollector iterates the state according to the given options and inserts
// the items into a collector for aggregation or serialization.
func (stateDB *StateDB) DumpToCollector(c DumpCollector, conf *DumpConfig) (nextKey []byte) {
	// Sanitize the input to allow nil configs
	if conf == nil {
		conf = new(DumpConfig)
	}
	var (
		missingPreimages int
		accounts         uint64
		start            = time.Now()
		logged           = time.Now()
	)
	log.Info("Trie dumping started", "root", stateDB.trie.Hash())
	c.OnRoot(stateDB.trie.Hash())

	it := trie.NewIterator(stateDB.trie.NodeIterator(conf.Start))
	for it.Next() {
		var data model.StateAccount
		if err := rlp.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}
		account := DumpAccount{
			Balance:   data.Balance.String(),
			Nonce:     data.Nonce,
			Root:      data.Root[:],
			CodeHash:  data.CodeHash,
			SecureKey: it.Key,
		}
		addrBytes := stateDB.trie.GetKey(it.Key)
		if addrBytes == nil {
			// Preimage missing
			missingPreimages++
			if conf.OnlyWithAddresses {
				continue
			}
			account.SecureKey = it.Key
		}
		addr := common.BytesToAddress(addrBytes)
		obj := newObject(stateDB, addr, data)
		if !conf.SkipCode {
			account.Code = obj.Code(stateDB.db)
		}
		if !conf.SkipStorage {
			account.Storage = make(map[common.Hash]string)
			storageIt := trie.NewIterator(obj.getTrie(stateDB.db).NodeIterator(nil))
			for storageIt.Next() {
				_, content, _, err := rlp.Split(storageIt.Value)
				if err != nil {
					log.Error("Failed to decode the value returned by iterator", "error", err)
					continue
				}
				account.Storage[common.BytesToHash(stateDB.trie.GetKey(storageIt.Key))] = common.Bytes2Hex(content)
			}
		}
		c.OnAccount(addr, account)
		accounts++
		if time.Since(logged) > 8*time.Second {
			log.Info("Trie dumping in progress", "at", it.Key, "accounts", accounts,
				"elapsed", common.PrettyDuration(time.Since(start)))
			logged = time.Now()
		}
		if conf.Max > 0 && accounts >= conf.Max {
			if it.Next() {
				nextKey = it.Key
			}
			break
		}
	}
	if missingPreimages > 0 {
		log.Warning("Dump incomplete due to missing preimages", "missing", missingPreimages)
	}
	log.Info("Trie dumping complete", "accounts", accounts,
		"elapsed", common.PrettyDuration(time.Since(start)))

	return nextKey
}

// RawDump returns the entire state an a single large object
func (stateDB *StateDB) RawDump(opts *DumpConfig) Dump {
	dump := &Dump{
		Accounts: make(map[common.Address]DumpAccount),
	}
	stateDB.DumpToCollector(dump, opts)
	return *dump
}

// Dump returns a JSON string representing the entire state as a single json-object
func (stateDB *StateDB) Dump(opts *DumpConfig) []byte {
	dump := stateDB.RawDump(opts)
	jsonObj, err := json.MarshalIndent(dump, "", "    ")
	if err != nil {
		fmt.Println("Dump err", err)
	}
	return jsonObj
}

// IterativeDump dumps out accounts as json-objects, delimited by linebreaks on stdout
func (stateDB *StateDB) IterativeDump(opts *DumpConfig, output *json.Encoder) {
	stateDB.DumpToCollector(iterativeDump{output}, opts)
}

// IteratorDump dumps out a batch of accounts starts with the given start key
func (stateDB *StateDB) IteratorDump(opts *DumpConfig) IteratorDump {
	iterator := &IteratorDump{
		Accounts: make(map[common.Address]DumpAccount),
	}
	iterator.Next = stateDB.DumpToCollector(iterator, opts)
	return *iterator
}
