package mapper

import (
	"encoding/json"

	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
)

// ReadDatabaseVersion retrieves the version number of the database.
func ReadDatabaseVersion(db database.KeyValueReader) *uint64 {
	var version uint64

	enc, _ := db.Get(databaseVerisionKey)
	if len(enc) == 0 {
		return nil
	}
	if err := rlputil.DecodeBytes(enc, &version); err != nil {
		return nil
	}

	return &version
}

// WriteDatabaseVersion stores the version number of the database
func WriteDatabaseVersion(db database.KeyValueWriter, version uint64) {
	enc, err := rlputil.EncodeToBytes(version)
	if err != nil {
		mapperLog.Critical("Failed to encode database version", "err", err)
	}
	if err := db.Put(databaseVerisionKey, enc); err != nil {
		mapperLog.Critical("Failed to store the database version", "err", err)
	}
	//mapperLog.Debugf("WriteDatabaseVersion: key=%s, key=%X, version=%d", databaseVerisionKey, databaseVerisionKey, version)
}

// ReadChainConfig retrieves the consensus settings based on the given genesis hash.
func ReadChainConfig(db database.KeyValueReader, hash common.Hash) *config.ChainConfig {
	key := configKey(hash)
	data, _ := db.Get(key)
	//mapperLog.Debugf("ReadChainConfig: key=%s, key=%X, data=%X", configPrefix, key, data)

	if len(data) == 0 {
		return nil
	}
	var config config.ChainConfig
	if err := json.Unmarshal(data, &config); err != nil {
		mapperLog.Error("Invalid chain config JSON", "hash", hash, "err", err)
		return nil
	}
	return &config
}

// WriteChainConfig writes the chain config settings to the database.
func WriteChainConfig(db database.KeyValueWriter, hash common.Hash, cfg *config.ChainConfig) {
	if cfg == nil {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		mapperLog.Critical("Failed to JSON encode chain config", "err", err)
	}
	key := configKey(hash)
	if err := db.Put(key, data); err != nil {
		mapperLog.Critical("Failed to store chain config", "err", err)
	}

	//mapperLog.Debugf("WriteChainConfig: key=%s, key=%X, data=%X", configPrefix, key, data)
}

// ReadPreimage retrieves a single preimage of the provided hash.
func ReadPreimage(db database.KeyValueReader, hash common.Hash) []byte {
	key := preimageKey(hash)
	data, _ := db.Get(key)
	//mapperLog.Debugf("ReadPreimage: key=%s, key=%X, data=%X", preimagePrefix, key, data)
	return data
}

// WritePreimages writes the provided set of preimages to the database. `number` is the
// current block number, and is used for debug messages only.
func WritePreimages(db database.KeyValueWriter, preimages map[common.Hash][]byte) {
	for hash, preimage := range preimages {
		key := preimageKey(hash)
		if err := db.Put(key, preimage); err != nil {
			mapperLog.Critical("Failed to store trie preimage", "err", err)
		}
		//mapperLog.Debugf("WritePreimages: key=%s, key=%X, data=%X", preimagePrefix, key, preimage)
	}
	preimageCounter.Inc(int64(len(preimages)))
	preimageHitCounter.Inc(int64(len(preimages)))
}
