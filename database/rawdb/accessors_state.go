package rawdb

import (
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/database"
)

// ReadPreimage retrieves a single preimage of the provided hash.
func ReadPreimage(db database.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(preimageKey(hash), "preImage")
	log.Debugf("DB ReadPreimage. key:secure-key-......, hash:%x, vSize:%d", hash, len(data))
	return data
}

// ReadCode retrieves the contract code of the provided code hash.
func ReadCode(db database.KeyValueReader, hash common.Hash) []byte {
	// Try with the prefixed code scheme first, if not then try with legacy scheme.
	data := ReadCodeWithPrefix(db, hash)
	if len(data) != 0 {
		return data
	}

	data, _ = db.Get(hash.Bytes(), "code")
	log.Debugf("DB ReadCode. key:......, hash:%x, vSize:%d", hash, len(data))
	return data
}

// ReadCodeWithPrefix retrieves the contract code of the provided code hash.
// The main difference between this function and ReadCode is this function
// will only check the existence with latest scheme(with prefix).
func ReadCodeWithPrefix(db database.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(codeKey(hash), "code")
	log.Debugf("DB ReadCodeWithPrefix. key:c......, hash:%x, vSize:%d", hash, len(data))
	return data
}

// ReadTrieNode retrieves the trie node of the provided hash.
func ReadTrieNode(db database.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(hash.Bytes(), "trieNode")
	log.Debugf("DB ReadTrieNode. key:......, hash:%x, vSize:%d", hash, len(data))
	return data
}

// HasCode checks if the contract code corresponding to the
// provided code hash is present in the db.
func HasCode(db database.KeyValueReader, hash common.Hash) bool {
	// Try with the prefixed code scheme first, if not then try with legacy scheme.
	if ok := HasCodeWithPrefix(db, hash); ok {
		return true
	}
	ok, _ := db.Has(hash.Bytes(), "code")
	return ok
}

// HasCodeWithPrefix checks if the contract code corresponding to the
// provided code hash is present in the db. This function will only check
// presence using the prefix-scheme.
func HasCodeWithPrefix(db database.KeyValueReader, hash common.Hash) bool {
	ok, _ := db.Has(codeKey(hash), "code")
	log.Debugf("DB HasTrieNode. key:c......, hash:%x, value:%v", hash, ok)
	return ok
}

// HasTrieNode checks if the trie node with the provided hash is present in db.
func HasTrieNode(db database.KeyValueReader, hash common.Hash) bool {
	ok, _ := db.Has(hash.Bytes(), "trieNode")
	log.Debugf("DB HasTrieNode. key:......, hash:%x, value:%v", hash, ok)
	return ok
}

// WritePreimages writes the provided set of preimages to the database.
func WritePreimages(db database.KeyValueWriter, preimages map[common.Hash][]byte) {
	for hash, preimage := range preimages {
		if err := db.Put(preimageKey(hash), preimage, "preImage"); err != nil {
			log.Critical("Failed to store trie preimage", "err", err)
		}
	}
	preimageCounter.Inc(int64(len(preimages)))
	preimageHitCounter.Inc(int64(len(preimages)))
	log.Debugf("DB WritePreimages. key:secure-key-......, preimages:%d", len(preimages))
}

// WriteCode writes the provided contract code database.
func WriteCode(db database.KeyValueWriter, hash common.Hash, code []byte) {
	if err := db.Put(codeKey(hash), code, "code"); err != nil {
		log.Critical("Failed to store contract code", "err", err)
	}
	log.Debugf("DB WriteCode. key:c......, hash:%x, vSize:%d", hash, len(code))
}

// WriteTrieNode writes the provided trie node database.
func WriteTrieNode(db database.KeyValueWriter, hash common.Hash, node []byte) {
	if err := db.Put(hash.Bytes(), node, "trieNode"); err != nil {
		log.Critical("Failed to store trie node", "err", err)
	}
	log.Debugf("DB WriteTrieNode. key:......, hash:%x, vSize:%d", hash, len(node))
}

// DeleteCode deletes the specified contract code from the database.
func DeleteCode(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(codeKey(hash), "code"); err != nil {
		log.Critical("Failed to delete contract code", "err", err)
	}
	log.Debugf("DB DeleteCode. key:c......, hash:%x", hash)
}

// DeleteTrieNode deletes the specified trie node from the database.
func DeleteTrieNode(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(hash.Bytes(), "trieNode"); err != nil {
		log.Critical("Failed to delete trie node", "err", err)
	}
	log.Debugf("DB DeleteTrieNode. key:......, hash:%x", hash)
}
