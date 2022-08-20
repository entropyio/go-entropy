package light

import (
	"errors"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/database"
	"sync"
)

// NodeSet stores a set of trie nodes. It implements trie.Database and can also
// act as a cache for another trie.Database.
type NodeSet struct {
	nodes map[string][]byte
	order []string

	dataSize int
	lock     sync.RWMutex
}

// NewNodeSet creates an empty node set
func NewNodeSet() *NodeSet {
	return &NodeSet{
		nodes: make(map[string][]byte),
	}
}

// Put stores a new node in the set
func (db *NodeSet) Put(key []byte, value []byte, from string) error {
	db.lock.Lock()
	defer db.lock.Unlock()

	if _, ok := db.nodes[string(key)]; ok {
		return nil
	}

	keyStr := string(key)
	db.nodes[keyStr] = common.CopyBytes(value)
	db.order = append(db.order, keyStr)
	db.dataSize += len(value)

	log.Debugf("NodeSet Put %s. key:%s, vSize:%d", from, keyStr, len(value))
	return nil
}

// Delete removes a node from the set
func (db *NodeSet) Delete(key []byte, from string) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	delete(db.nodes, string(key))

	log.Debugf("NodeSet Delete %s. key:%s, vSize:%d", from, string(key))
	return nil
}

// Get returns a stored node
func (db *NodeSet) Get(key []byte, from string) ([]byte, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if entry, ok := db.nodes[string(key)]; ok {
		log.Debugf("NodeSet Get %s. key:%s, vSize:%d", from, string(key), len(entry))
		return entry, nil
	}
	return nil, errors.New("not found")
}

// Has returns true if the node set contains the given key
func (db *NodeSet) Has(key []byte, from string) (bool, error) {
	_, err := db.Get(key, from)
	return err == nil, nil
}

// KeyCount returns the number of nodes in the set
func (db *NodeSet) KeyCount() int {
	db.lock.RLock()
	defer db.lock.RUnlock()

	return len(db.nodes)
}

// DataSize returns the aggregated data size of nodes in the set
func (db *NodeSet) DataSize() int {
	db.lock.RLock()
	defer db.lock.RUnlock()

	return db.dataSize
}

// NodeList converts the node set to a NodeList
func (db *NodeSet) NodeList() NodeList {
	db.lock.RLock()
	defer db.lock.RUnlock()

	var values NodeList
	for _, key := range db.order {
		values = append(values, db.nodes[key])
	}
	return values
}

// Store writes the contents of the set to the given database
func (db *NodeSet) Store(target database.KeyValueWriter) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	for key, value := range db.nodes {
		_ = target.Put([]byte(key), value, "nodeSet")
	}
}

// NodeList stores an ordered list of trie nodes. It implements ethdb.KeyValueWriter.
type NodeList []rlp.RawValue

// Store writes the contents of the list to the given database
func (n NodeList) Store(db database.KeyValueWriter) {
	for _, node := range n {
		_ = db.Put(crypto.Keccak256(node), node, "nodeList")
	}
}

// NodeSet converts the node list to a NodeSet
func (n NodeList) NodeSet() *NodeSet {
	db := NewNodeSet()
	n.Store(db)
	return db
}

// Put stores a new node at the end of the list
func (n *NodeList) Put(key []byte, value []byte, from string) error {
	log.Debugf("NodeList Put %s. key:%x, vSize:%d", from, key, len(value))
	*n = append(*n, value)
	return nil
}

// Delete panics as there's no reason to remove a node from the list.
func (n *NodeList) Delete([]byte, string) error {
	panic("not supported")
}

// DataSize returns the aggregated data size of nodes in the list
func (n NodeList) DataSize() int {
	var size int
	for _, node := range n {
		size += len(node)
	}
	return size
}
