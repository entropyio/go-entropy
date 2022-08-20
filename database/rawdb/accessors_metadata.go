package rawdb

import (
	"encoding/json"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
	"time"
)

// ReadDatabaseVersion retrieves the version number of the database.
func ReadDatabaseVersion(db database.KeyValueReader) *uint64 {
	var version uint64

	enc, _ := db.Get(databaseVersionKey, "dbVersion")
	if len(enc) == 0 {
		return nil
	}
	if err := rlp.DecodeBytes(enc, &version); err != nil {
		return nil
	}

	return &version
}

// WriteDatabaseVersion stores the version number of the database
func WriteDatabaseVersion(db database.KeyValueWriter, version uint64) {
	enc, err := rlp.EncodeToBytes(version)
	if err != nil {
		log.Critical("Failed to encode database version", "err", err)
	}
	if err = db.Put(databaseVersionKey, enc, "dbVersion"); err != nil {
		log.Critical("Failed to store the database version", "err", err)
	}
}

// ReadChainConfig retrieves the consensus settings based on the given genesis hash.
func ReadChainConfig(db database.KeyValueReader, hash common.Hash) *config.ChainConfig {
	data, _ := db.Get(configKey(hash), "config")
	if len(data) == 0 {
		return nil
	}
	var configObj config.ChainConfig
	if err := json.Unmarshal(data, &configObj); err != nil {
		log.Error("Invalid chain config JSON", "hash", hash, "err", err)
		return nil
	}
	return &configObj
}

// WriteChainConfig writes the chain config settings to the database.
func WriteChainConfig(db database.KeyValueWriter, hash common.Hash, cfg *config.ChainConfig) {
	if cfg == nil {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Critical("Failed to JSON encode chain config", "err", err)
	}
	if err := db.Put(configKey(hash), data, "config"); err != nil {
		log.Critical("Failed to store chain config", "err", err)
	}
}

// ReadGenesisState retrieves the genesis state based on the given genesis hash.
func ReadGenesisState(db database.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(genesisKey(hash), "genesis")
	return data
}

// WriteGenesisState writes the genesis state into the disk.
func WriteGenesisState(db database.KeyValueWriter, hash common.Hash, data []byte) {
	if err := db.Put(genesisKey(hash), data, "genesis"); err != nil {
		log.Critical("Failed to store genesis state", "err", err)
	}
}

// crashList is a list of unclean-shutdown-markers, for rlp-encoding to the
// database
type crashList struct {
	Discarded uint64   // how many ucs have we deleted
	Recent    []uint64 // unix timestamps of 10 latest unclean shutdowns
}

const crashesToKeep = 10

// PushUncleanShutdownMarker appends a new unclean shutdown marker and returns
// the previous data
// - a list of timestamps
// - a count of how many old unclean-shutdowns have been discarded
func PushUncleanShutdownMarker(db database.KeyValueStore) ([]uint64, uint64, error) {
	var uncleanShutdowns crashList
	// Read old data
	if data, err := db.Get(uncleanShutdownKey, "pushUnclean"); err != nil {
		log.Warning("Error reading unclean shutdown markers", "error", err)
	} else if err := rlp.DecodeBytes(data, &uncleanShutdowns); err != nil {
		return nil, 0, err
	}
	var discarded = uncleanShutdowns.Discarded
	var previous = make([]uint64, len(uncleanShutdowns.Recent))
	copy(previous, uncleanShutdowns.Recent)
	// Add a new (but cap it)
	uncleanShutdowns.Recent = append(uncleanShutdowns.Recent, uint64(time.Now().Unix()))
	if count := len(uncleanShutdowns.Recent); count > crashesToKeep+1 {
		numDel := count - (crashesToKeep + 1)
		uncleanShutdowns.Recent = uncleanShutdowns.Recent[numDel:]
		uncleanShutdowns.Discarded += uint64(numDel)
	}
	// And save it again
	data, _ := rlp.EncodeToBytes(uncleanShutdowns)
	if err := db.Put(uncleanShutdownKey, data, "pushUnclean"); err != nil {
		log.Warning("Failed to write unclean-shutdown marker", "err", err)
		return nil, 0, err
	}
	return previous, discarded, nil
}

// PopUncleanShutdownMarker removes the last unclean shutdown marker
func PopUncleanShutdownMarker(db database.KeyValueStore) {
	var uncleanShutdowns crashList
	// Read old data
	if data, err := db.Get(uncleanShutdownKey, "popUnclean"); err != nil {
		log.Warning("Error reading unclean shutdown markers", "error", err)
	} else if err := rlp.DecodeBytes(data, &uncleanShutdowns); err != nil {
		log.Error("Error decoding unclean shutdown markers", "error", err) // Should mos def _not_ happen
	}
	if l := len(uncleanShutdowns.Recent); l > 0 {
		uncleanShutdowns.Recent = uncleanShutdowns.Recent[:l-1]
	}
	data, _ := rlp.EncodeToBytes(uncleanShutdowns)
	if err := db.Put(uncleanShutdownKey, data, "popUnclean"); err != nil {
		log.Warning("Failed to clear unclean-shutdown marker", "err", err)
	}
}

// UpdateUncleanShutdownMarker updates the last marker's timestamp to now.
func UpdateUncleanShutdownMarker(db database.KeyValueStore) {
	var uncleanShutdowns crashList
	// Read old data
	if data, err := db.Get(uncleanShutdownKey, "updateUnclean"); err != nil {
		log.Warning("Error reading unclean shutdown markers", "error", err)
	} else if err := rlp.DecodeBytes(data, &uncleanShutdowns); err != nil {
		log.Warning("Error decoding unclean shutdown markers", "error", err)
	}
	// This shouldn't happen because we push a marker on Backend instantiation
	count := len(uncleanShutdowns.Recent)
	if count == 0 {
		log.Warning("No unclean shutdown marker to update")
		return
	}
	uncleanShutdowns.Recent[count-1] = uint64(time.Now().Unix())
	data, _ := rlp.EncodeToBytes(uncleanShutdowns)
	if err := db.Put(uncleanShutdownKey, data, "updateUnclean"); err != nil {
		log.Warning("Failed to write unclean-shutdown marker", "err", err)
	}
}

// ReadTransitionStatus retrieves the eth2 transition status from the database
func ReadTransitionStatus(db database.KeyValueReader) []byte {
	data, _ := db.Get(transitionStatusKey, "transitionStatus")
	return data
}

// WriteTransitionStatus stores the eth2 transition status to the database
func WriteTransitionStatus(db database.KeyValueWriter, data []byte) {
	if err := db.Put(transitionStatusKey, data, "transitionStatus"); err != nil {
		log.Critical("Failed to store the entropy2 transition status", "err", err)
	}
}
