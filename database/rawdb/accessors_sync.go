package rawdb

import (
	"bytes"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/database"
)

// ReadSkeletonSyncStatus retrieves the serialized sync status saved at shutdown.
func ReadSkeletonSyncStatus(db database.KeyValueReader) []byte {
	data, _ := db.Get(skeletonSyncStatusKey, "skeletonSyncStatus")
	return data
}

// WriteSkeletonSyncStatus stores the serialized sync status to save at shutdown.
func WriteSkeletonSyncStatus(db database.KeyValueWriter, status []byte) {
	if err := db.Put(skeletonSyncStatusKey, status, "skeletonSyncStatus"); err != nil {
		log.Critical("Failed to store skeleton sync status", "err", err)
	}
}

// DeleteSkeletonSyncStatus deletes the serialized sync status saved at the last
// shutdown
func DeleteSkeletonSyncStatus(db database.KeyValueWriter) {
	if err := db.Delete(skeletonSyncStatusKey, "skeletonSyncStatus"); err != nil {
		log.Critical("Failed to remove skeleton sync status", "err", err)
	}
}

// ReadSkeletonHeader retrieves a block header from the skeleton sync store,
func ReadSkeletonHeader(db database.KeyValueReader, number uint64) *model.Header {
	data, _ := db.Get(skeletonHeaderKey(number), "skeletonHeader")
	if len(data) == 0 {
		return nil
	}
	header := new(model.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid skeleton header RLP", "number", number, "err", err)
		return nil
	}
	return header
}

// WriteSkeletonHeader stores a block header into the skeleton sync store.
func WriteSkeletonHeader(db database.KeyValueWriter, header *model.Header) {
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Critical("Failed to RLP encode header", "err", err)
	}
	key := skeletonHeaderKey(header.Number.Uint64())
	if err := db.Put(key, data, "skeletonHeader"); err != nil {
		log.Critical("Failed to store skeleton header", "err", err)
	}
}

// DeleteSkeletonHeader removes all block header data associated with a hash.
func DeleteSkeletonHeader(db database.KeyValueWriter, number uint64) {
	if err := db.Delete(skeletonHeaderKey(number), "skeletonHeader"); err != nil {
		log.Critical("Failed to delete skeleton header", "err", err)
	}
}
