package state

import (
	"encoding/json"
	"fmt"

	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/database/trie"
)

type DumpAccount struct {
	Balance  string            `json:"balance"`
	Nonce    uint64            `json:"nonce"`
	Root     string            `json:"root"`
	CodeHash string            `json:"codeHash"`
	Code     string            `json:"code"`
	Storage  map[string]string `json:"storage"`
}

type Dump struct {
	Root     string                 `json:"root"`
	Accounts map[string]DumpAccount `json:"account"`
}

func (stateDB *StateDB) RawDump() Dump {
	dump := Dump{
		Root:     fmt.Sprintf("%x", stateDB.trie.Hash()),
		Accounts: make(map[string]DumpAccount),
	}

	it := trie.NewIterator(stateDB.trie.NodeIterator(nil))
	for it.Next() {
		addr := stateDB.trie.GetKey(it.Key)
		var data Account
		if err := rlputil.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}

		obj := newObject(nil, common.BytesToAddress(addr), data)
		account := DumpAccount{
			Balance:  data.Balance.String(),
			Nonce:    data.Nonce,
			Root:     common.Bytes2Hex(data.Root[:]),
			CodeHash: common.Bytes2Hex(data.CodeHash),
			Code:     common.Bytes2Hex(obj.Code(stateDB.db)),
			Storage:  make(map[string]string),
		}
		storageIt := trie.NewIterator(obj.getTrie(stateDB.db).NodeIterator(nil))
		for storageIt.Next() {
			account.Storage[common.Bytes2Hex(stateDB.trie.GetKey(storageIt.Key))] = common.Bytes2Hex(storageIt.Value)
		}
		dump.Accounts[common.Bytes2Hex(addr)] = account
	}
	return dump
}

func (stateDB *StateDB) Dump() []byte {
	jsonObj, err := json.MarshalIndent(stateDB.RawDump(), "", "    ")
	if err != nil {
		fmt.Println("dump err", err)
	}

	return jsonObj
}
