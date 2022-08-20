package rawdb

import (
	"encoding/binary"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/database"
)

// ReadSnapshotDisabled retrieves if the snapshot maintenance is disabled.
func ReadSnapshotDisabled(db database.KeyValueReader) bool {
	disabled, _ := db.Has(snapshotDisabledKey, "snapshotDisabled")
	return disabled
}

// WriteSnapshotDisabled stores the snapshot pause flag.
func WriteSnapshotDisabled(db database.KeyValueWriter) {
	if err := db.Put(snapshotDisabledKey, []byte("42"), "snapshotDisabled"); err != nil {
		log.Critical("Failed to store snapshot disabled flag", "err", err)
	}
}

// DeleteSnapshotDisabled deletes the flag keeping the snapshot maintenance disabled.
func DeleteSnapshotDisabled(db database.KeyValueWriter) {
	if err := db.Delete(snapshotDisabledKey, "snapshotDisabled"); err != nil {
		log.Critical("Failed to remove snapshot disabled flag", "err", err)
	}
}

// ReadSnapshotRoot retrieves the root of the block whose state is contained in
// the persisted snapshot.
func ReadSnapshotRoot(db database.KeyValueReader) common.Hash {
	data, _ := db.Get(SnapshotRootKey, "snapshotRoot")
	if len(data) != common.HashLength {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteSnapshotRoot stores the root of the block whose state is contained in
// the persisted snapshot.
func WriteSnapshotRoot(db database.KeyValueWriter, root common.Hash) {
	if err := db.Put(SnapshotRootKey, root[:], "snapshotRoot"); err != nil {
		log.Critical("Failed to store snapshot root", "err", err)
	}
}

// DeleteSnapshotRoot deletes the hash of the block whose state is contained in
// the persisted snapshot. Since snapshots are not immutable, this  method can
// be used during updates, so a crash or failure will mark the entire snapshot
// invalid.
func DeleteSnapshotRoot(db database.KeyValueWriter) {
	if err := db.Delete(SnapshotRootKey, "snapshotRoot"); err != nil {
		log.Critical("Failed to remove snapshot root", "err", err)
	}
}

// ReadAccountSnapshot retrieves the snapshot entry of an account trie leaf.
func ReadAccountSnapshot(db database.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(accountSnapshotKey(hash), "accountSnapshot")
	return data
}

// WriteAccountSnapshot stores the snapshot entry of an account trie leaf.
func WriteAccountSnapshot(db database.KeyValueWriter, hash common.Hash, entry []byte) {
	if err := db.Put(accountSnapshotKey(hash), entry, "accountSnapshot"); err != nil {
		log.Critical("Failed to store account snapshot", "err", err)
	}
}

// DeleteAccountSnapshot removes the snapshot entry of an account trie leaf.
func DeleteAccountSnapshot(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(accountSnapshotKey(hash), "accountSnapshot"); err != nil {
		log.Critical("Failed to delete account snapshot", "err", err)
	}
}

// ReadStorageSnapshot retrieves the snapshot entry of an storage trie leaf.
func ReadStorageSnapshot(db database.KeyValueReader, accountHash, storageHash common.Hash) []byte {
	data, _ := db.Get(storageSnapshotKey(accountHash, storageHash), "storageSnapshot")
	return data
}

// WriteStorageSnapshot stores the snapshot entry of an storage trie leaf.
func WriteStorageSnapshot(db database.KeyValueWriter, accountHash, storageHash common.Hash, entry []byte) {
	if err := db.Put(storageSnapshotKey(accountHash, storageHash), entry, "storageSnapshot"); err != nil {
		log.Critical("Failed to store storage snapshot", "err", err)
	}
}

// DeleteStorageSnapshot removes the snapshot entry of an storage trie leaf.
func DeleteStorageSnapshot(db database.KeyValueWriter, accountHash, storageHash common.Hash) {
	if err := db.Delete(storageSnapshotKey(accountHash, storageHash), "storageSnapshot"); err != nil {
		log.Critical("Failed to delete storage snapshot", "err", err)
	}
}

// IterateStorageSnapshots returns an iterator for walking the entire storage
// space of a specific account.
func IterateStorageSnapshots(db database.Iteratee, accountHash common.Hash) database.Iterator {
	return NewKeyLengthIterator(db.NewIterator(storageSnapshotsKey(accountHash), nil), len(SnapshotStoragePrefix)+2*common.HashLength)
}

// ReadSnapshotJournal retrieves the serialized in-memory diff layers saved at
// the last shutdown. The blob is expected to be max a few 10s of megabytes.
func ReadSnapshotJournal(db database.KeyValueReader) []byte {
	data, _ := db.Get(snapshotJournalKey, "snapshotJournal")
	return data
}

// WriteSnapshotJournal stores the serialized in-memory diff layers to save at
// shutdown. The blob is expected to be max a few 10s of megabytes.
func WriteSnapshotJournal(db database.KeyValueWriter, journal []byte) {
	if err := db.Put(snapshotJournalKey, journal, "snapshotJournal"); err != nil {
		log.Critical("Failed to store snapshot journal", "err", err)
	}
}

// DeleteSnapshotJournal deletes the serialized in-memory diff layers saved at
// the last shutdown
func DeleteSnapshotJournal(db database.KeyValueWriter) {
	if err := db.Delete(snapshotJournalKey, "snapshotJournal"); err != nil {
		log.Critical("Failed to remove snapshot journal", "err", err)
	}
}

// ReadSnapshotGenerator retrieves the serialized snapshot generator saved at
// the last shutdown.
func ReadSnapshotGenerator(db database.KeyValueReader) []byte {
	data, _ := db.Get(snapshotGeneratorKey, "snapshotGenerator")
	return data
}

// WriteSnapshotGenerator stores the serialized snapshot generator to save at
// shutdown.
func WriteSnapshotGenerator(db database.KeyValueWriter, generator []byte) {
	if err := db.Put(snapshotGeneratorKey, generator, "snapshotGenerator"); err != nil {
		log.Critical("Failed to store snapshot generator", "err", err)
	}
}

// DeleteSnapshotGenerator deletes the serialized snapshot generator saved at
// the last shutdown
func DeleteSnapshotGenerator(db database.KeyValueWriter) {
	if err := db.Delete(snapshotGeneratorKey, "snapshotGenerator"); err != nil {
		log.Critical("Failed to remove snapshot generator", "err", err)
	}
}

// ReadSnapshotRecoveryNumber retrieves the block number of the last persisted
// snapshot layer.
func ReadSnapshotRecoveryNumber(db database.KeyValueReader) *uint64 {
	data, _ := db.Get(snapshotRecoveryKey, "snapshotRecoveryNumber")
	if len(data) == 0 {
		return nil
	}
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// WriteSnapshotRecoveryNumber stores the block number of the last persisted
// snapshot layer.
func WriteSnapshotRecoveryNumber(db database.KeyValueWriter, number uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], number)
	if err := db.Put(snapshotRecoveryKey, buf[:], "snapshotRecoveryNumber"); err != nil {
		log.Critical("Failed to store snapshot recovery number", "err", err)
	}
}

// DeleteSnapshotRecoveryNumber deletes the block number of the last persisted
// snapshot layer.
func DeleteSnapshotRecoveryNumber(db database.KeyValueWriter) {
	if err := db.Delete(snapshotRecoveryKey, "snapshotRecoveryNumber"); err != nil {
		log.Critical("Failed to remove snapshot recovery number", "err", err)
	}
}

// ReadSnapshotSyncStatus retrieves the serialized sync status saved at shutdown.
func ReadSnapshotSyncStatus(db database.KeyValueReader) []byte {
	data, _ := db.Get(snapshotSyncStatusKey, "snapshotSyncStatus")
	return data
}

// WriteSnapshotSyncStatus stores the serialized sync status to save at shutdown.
func WriteSnapshotSyncStatus(db database.KeyValueWriter, status []byte) {
	if err := db.Put(snapshotSyncStatusKey, status, "snapshotSyncStatus"); err != nil {
		log.Critical("Failed to store snapshot sync status", "err", err)
	}
}
