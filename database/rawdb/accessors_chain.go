package rawdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
	"math/big"
	"sort"
)

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadCanonicalHash(db database.Reader, number uint64) common.Hash {
	var data []byte
	_ = db.ReadAncients(func(reader database.AncientReaderOp) error {
		data, _ = reader.Ancient(freezerHashTable, number)
		if len(data) == 0 {
			data, _ = db.Get(headerHashKey(number), "canonicalHash")
		}
		return nil
	})
	hash := common.BytesToHash(data)

	log.Debugf("DB ReadCanonicalHash. number:%d, hash:%x", number, hash)
	return hash
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteCanonicalHash(db database.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Put(headerHashKey(number), hash.Bytes(), "canonicalHash"); err != nil {
		log.Critical("Failed to store number to hash mapping", "err", err)
	}

	log.Debugf("DB WriteCanonicalHash. key: h......n, number:%d, hash:%x", number, hash)
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHash(db database.KeyValueWriter, number uint64) {
	if err := db.Delete(headerHashKey(number), "canonicalHash"); err != nil {
		log.Critical("Failed to delete number to hash mapping", "err", err)
	}

	log.Debugf("DB DeleteCanonicalHash. key: h......n, number:%d", number)
}

// ReadAllHashes retrieves all the hashes assigned to blocks at a certain heights,
// both canonical and reorged forks included.
func ReadAllHashes(db database.Iteratee, number uint64) []common.Hash {
	prefix := headerKeyPrefix(number)
	hashes := make([]common.Hash, 0, 1)
	it := db.NewIterator(prefix, nil)
	defer it.Release()

	for it.Next() {
		if key := it.Key(); len(key) == len(prefix)+32 {
			hashes = append(hashes, common.BytesToHash(key[len(key)-32:]))
		}
	}

	log.Debugf("DB ReadAllHashes. number:%d, hashes:%d", number, len(hashes))
	return hashes
}

type NumberHash struct {
	Number uint64
	Hash   common.Hash
}

// ReadAllHashesInRange retrieves all the hashes assigned to blocks at certain
// heights, both canonical and reorged forks included.
// This method considers both limits to be _inclusive_.
func ReadAllHashesInRange(db database.Iteratee, first, last uint64) []*NumberHash {
	var (
		start     = encodeBlockNumber(first)
		keyLength = len(headerPrefix) + 8 + 32
		hashes    = make([]*NumberHash, 0, 1+last-first)
		it        = db.NewIterator(headerPrefix, start)
	)
	defer it.Release()
	for it.Next() {
		key := it.Key()
		if len(key) != keyLength {
			continue
		}
		num := binary.BigEndian.Uint64(key[len(headerPrefix) : len(headerPrefix)+8])
		if num > last {
			break
		}
		hash := common.BytesToHash(key[len(key)-32:])
		hashes = append(hashes, &NumberHash{num, hash})
	}

	log.Debugf("DB ReadAllHashesInRange. number start:%d, end:%d, hashes:%d", first, last, len(hashes))
	return hashes
}

// ReadAllCanonicalHashes retrieves all canonical number and hash mappings at the
// certain chain range. If the accumulated entries reaches the given threshold,
// abort the iteration and return the semi-finish result.
func ReadAllCanonicalHashes(db database.Iteratee, from uint64, to uint64, limit int) ([]uint64, []common.Hash) {
	// Short circuit if the limit is 0.
	if limit == 0 {
		return nil, nil
	}
	var (
		numbers []uint64
		hashes  []common.Hash
	)
	// Construct the key prefix of start point.
	start, end := headerHashKey(from), headerHashKey(to)
	it := db.NewIterator(nil, start)
	defer it.Release()

	for it.Next() {
		if bytes.Compare(it.Key(), end) >= 0 {
			break
		}
		if key := it.Key(); len(key) == len(headerPrefix)+8+1 && bytes.Equal(key[len(key)-1:], headerHashSuffix) {
			numbers = append(numbers, binary.BigEndian.Uint64(key[len(headerPrefix):len(headerPrefix)+8]))
			hashes = append(hashes, common.BytesToHash(it.Value()))
			// If the accumulated entries reaches the limit threshold, return.
			if len(numbers) >= limit {
				break
			}
		}
	}

	log.Debugf("DB ReadAllCanonicalHashes. start:%d, end:%d, limit: %d, numbers:%d, hashes:%d", from, to, limit, len(numbers), len(hashes))
	return numbers, hashes
}

// ReadHeaderNumber returns the header number assigned to a hash.
func ReadHeaderNumber(db database.KeyValueReader, hash common.Hash) *uint64 {
	data, _ := db.Get(headerNumberKey(hash), "headerNumber")
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)

	log.Debugf("DB ReadHeaderNumber. hash:%x, number:%d", hash, number)
	return &number
}

// WriteHeaderNumber stores the hash->number mapping.
func WriteHeaderNumber(db database.KeyValueWriter, hash common.Hash, number uint64) {
	key := headerNumberKey(hash)
	enc := encodeBlockNumber(number)
	if err := db.Put(key, enc, "headerNumber"); err != nil {
		log.Critical("Failed to store hash to number mapping", "err", err)
	}

	log.Debugf("DB WriteHeaderNumber. key:H......, hash:%x, number:%d", hash, number)
}

// DeleteHeaderNumber removes hash->number mapping.
func DeleteHeaderNumber(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(headerNumberKey(hash), "headerNumber"); err != nil {
		log.Critical("Failed to delete hash to number mapping", "err", err)
	}

	log.Debugf("DB DeleteHeaderNumber. key:H......, hash:%x", hash)
}

// ReadHeadHeaderHash retrieves the hash of the current canonical head header.
func ReadHeadHeaderHash(db database.KeyValueReader) common.Hash {
	data, _ := db.Get(headHeaderKey, "headerKey")
	if len(data) == 0 {
		return common.Hash{}
	}
	hash := common.BytesToHash(data)

	log.Debugf("DB ReadHeadHeaderHash. key:LastHeader, hash:%x", hash)
	return hash
}

// WriteHeadHeaderHash stores the hash of the current canonical head header.
func WriteHeadHeaderHash(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Put(headHeaderKey, hash.Bytes(), "headerKey"); err != nil {
		log.Critical("Failed to store last header's hash", "err", err)
	}

	log.Debugf("DB WriteHeadHeaderHash. key:LastHeader, hash:%x", hash)
}

// ReadHeadBlockHash retrieves the hash of the current canonical head block.
func ReadHeadBlockHash(db database.KeyValueReader) common.Hash {
	data, _ := db.Get(headBlockKey, "lastBlockHash")
	if len(data) == 0 {
		return common.Hash{}
	}
	hash := common.BytesToHash(data)

	log.Debugf("DB ReadHeadBlockHash. key:LastBlock, hash:%x", hash)
	return hash
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHash(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Put(headBlockKey, hash.Bytes(), "lastBlockHash"); err != nil {
		log.Critical("Failed to store last block's hash", "err", err)
	}

	log.Debugf("DB WriteHeadBlockHash. key:LastBlock, hash:%x", hash)
}

// ReadHeadFastBlockHash retrieves the hash of the current fast-sync head block.
func ReadHeadFastBlockHash(db database.KeyValueReader) common.Hash {
	data, _ := db.Get(headFastBlockKey, "lastFastHash")
	if len(data) == 0 {
		return common.Hash{}
	}
	hash := common.BytesToHash(data)

	log.Debugf("DB ReadHeadFastBlockHash. key:LastFast, hash:%x", hash)
	return hash
}

// WriteHeadFastBlockHash stores the hash of the current fast-sync head block.
func WriteHeadFastBlockHash(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Put(headFastBlockKey, hash.Bytes(), "lastFastHash"); err != nil {
		log.Critical("Failed to store last fast block's hash", "err", err)
	}

	log.Debugf("DB WriteHeadFastBlockHash. key:LastFast, hash:%x", hash)
}

// ReadFinalizedBlockHash retrieves the hash of the finalized block.
func ReadFinalizedBlockHash(db database.KeyValueReader) common.Hash {
	data, _ := db.Get(headFinalizedBlockKey, "lastFinalizedHash")
	if len(data) == 0 {
		return common.Hash{}
	}
	hash := common.BytesToHash(data)

	log.Debugf("DB ReadFinalizedBlockHash. key:LastFinalized, hash:%x", hash)
	return hash
}

// WriteFinalizedBlockHash stores the hash of the finalized block.
func WriteFinalizedBlockHash(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Put(headFinalizedBlockKey, hash.Bytes(), "lastFinalizedHash"); err != nil {
		log.Critical("Failed to store last finalized block's hash", "err", err)
	}

	log.Debugf("DB WriteFinalizedBlockHash. key:LastFinalized, hash:%x", hash)
}

// ReadLastPivotNumber retrieves the number of the last pivot block. If the node
// full synced, the last pivot will always be nil.
func ReadLastPivotNumber(db database.KeyValueReader) *uint64 {
	data, _ := db.Get(lastPivotKey, "lastPivot")
	if len(data) == 0 {
		return nil
	}
	var pivot uint64
	if err := rlp.DecodeBytes(data, &pivot); err != nil {
		log.Error("Invalid pivot block number in database", "err", err)
		return nil
	}

	log.Debugf("DB ReadLastPivotNumber. key:LastPivot, pivot:%d", pivot)
	return &pivot
}

// WriteLastPivotNumber stores the number of the last pivot block.
func WriteLastPivotNumber(db database.KeyValueWriter, pivot uint64) {
	enc, err := rlp.EncodeToBytes(pivot)
	if err != nil {
		log.Critical("Failed to encode pivot block number", "err", err)
	}
	if err := db.Put(lastPivotKey, enc, "lastPivot"); err != nil {
		log.Critical("Failed to store pivot block number", "err", err)
	}

	log.Debugf("DB WriteLastPivotNumber. key:LastPivot, pivot:%d", pivot)
}

// ReadTxIndexTail retrieves the number of oldest indexed block
// whose transaction indices has been indexed. If the corresponding entry
// is non-existent in database it means the indexing has been finished.
func ReadTxIndexTail(db database.KeyValueReader) *uint64 {
	data, _ := db.Get(txIndexTailKey, "transactionIndexTail")
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)

	log.Debugf("DB ReadTxIndexTail. key: TransactionIndexTail, number:%d", number)
	return &number
}

// WriteTxIndexTail stores the number of oldest indexed block
// into database.
func WriteTxIndexTail(db database.KeyValueWriter, number uint64) {
	if err := db.Put(txIndexTailKey, encodeBlockNumber(number), "transactionIndexTail"); err != nil {
		log.Critical("Failed to store the transaction index tail", "err", err)
	}

	log.Debugf("DB WriteTxIndexTail. key:TransactionIndexTail, number:%d", number)
}

// ReadFastTxLookupLimit retrieves the tx lookup limit used in fast sync.
func ReadFastTxLookupLimit(db database.KeyValueReader) *uint64 {
	data, _ := db.Get(fastTxLookupLimitKey, "fastTransactionLookupLimit")
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)

	log.Debugf("DB ReadFastTxLookupLimit. key:FastTransactionLookupLimit, number:%d", number)
	return &number
}

// WriteFastTxLookupLimit stores the txlookup limit used in fast sync into database.
func WriteFastTxLookupLimit(db database.KeyValueWriter, number uint64) {
	if err := db.Put(fastTxLookupLimitKey, encodeBlockNumber(number), "fastTransactionLookupLimit"); err != nil {
		log.Critical("Failed to store transaction lookup limit for fast sync", "err", err)
	}

	log.Debugf("DB WriteFastTxLookupLimit. key:FastTransactionLookupLimit, number:%d", number)
}

// ReadHeaderRange returns the rlp-encoded headers, starting at 'number', and going
// backwards towards genesis. This method assumes that the caller already has
// placed a cap on count, to prevent DoS issues.
// Since this method operates in head-towards-genesis mode, it will return an empty
// slice in case the head ('number') is missing. Hence, the caller must ensure that
// the head ('number') argument is actually an existing header.
//
// N.B: Since the input is a number, as opposed to a hash, it's implicit that
// this method only operates on canon headers.
func ReadHeaderRange(db database.Reader, number uint64, count uint64) []rlp.RawValue {
	var rlpHeaders []rlp.RawValue
	if count == 0 {
		return rlpHeaders
	}
	i := number
	if count-1 > number {
		// It's ok to request block 0, 1 item
		count = number + 1
	}
	limit, _ := db.Ancients()
	// First read live blocks
	if i >= limit {
		// If we need to read live blocks, we need to figure out the hash first
		hash := ReadCanonicalHash(db, number)
		for ; i >= limit && count > 0; i-- {
			if data, _ := db.Get(headerKey(i, hash), fmt.Sprintf("headerRange_%d", i)); len(data) > 0 {
				rlpHeaders = append(rlpHeaders, data)
				// Get the parent hash for next query
				hash = model.HeaderParentHashFromRLP(data)
			} else {
				break // Maybe got moved to ancients
			}
			count--
		}
	}
	if count == 0 {
		return rlpHeaders
	}
	// read remaining from ancients
	max := count * 700
	data, err := db.AncientRange(freezerHeaderTable, i+1-count, count, max)
	if err == nil && uint64(len(data)) == count {
		// the data is on the order [h, h+1, .., n] -- reordering needed
		for i := range data {
			rlpHeaders = append(rlpHeaders, data[len(data)-1-i])
		}
	}

	log.Debugf("DB ReadHeaderRange. number:%d, count:%d, headers:%d", number, count, len(rlpHeaders))
	return rlpHeaders
}

// ReadHeaderRLP retrieves a block header in its raw RLP database encoding.
func ReadHeaderRLP(db database.Reader, hash common.Hash, number uint64) rlp.RawValue {
	var data []byte
	_ = db.ReadAncients(func(reader database.AncientReaderOp) error {
		// First try to look up the data in ancient database. Extra hash
		// comparison is necessary since ancient database only maintains
		// the canonical data.
		data, _ = reader.Ancient(freezerHeaderTable, number)
		if len(data) > 0 && crypto.Keccak256Hash(data) == hash {
			return nil
		}
		// If not, try reading from leveldb
		data, _ = db.Get(headerKey(number, hash), "headerRLP")
		return nil
	})

	log.Debugf("DB ReadHeaderRLP. number:%d, hash:%x, vSize:%d", number, hash, len(data))
	return data
}

// HasHeader verifies the existence of a block header corresponding to the hash.
func HasHeader(db database.Reader, hash common.Hash, number uint64) bool {
	if isCanon(db, number, hash) {
		return true
	}

	res := true
	if has, err := db.Has(headerKey(number, hash), "header"); !has || err != nil {
		res = false
	}

	log.Debugf("DB HasHeader. number:%d, hash:%x, result:%v", number, hash, res)
	return res
}

// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeader(db database.Reader, hash common.Hash, number uint64) *model.Header {
	data := ReadHeaderRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	header := new(model.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "hash", hash, "err", err)
		return nil
	}

	log.Debugf("DB ReadHeader. key:h......, number:%d, hash:%x, vSize:%d", number, hash, len(data))
	return header
}

// WriteHeader stores a block header into the database and also stores the hash-
// to-number mapping.
func WriteHeader(db database.KeyValueWriter, header *model.Header) {
	var (
		hash   = header.Hash()
		number = header.Number.Uint64()
	)
	// Write the hash -> number mapping
	WriteHeaderNumber(db, hash, number)

	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Critical("Failed to RLP encode header", "err", err)
	}
	key := headerKey(number, hash)
	if err := db.Put(key, data, "header"); err != nil {
		log.Critical("Failed to store header", "err", err)
	}

	log.Debugf("DB WriteHeader. key:h......, number:%d, hash:%x, vSize:%d", number, hash, len(data))
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeader(db database.KeyValueWriter, hash common.Hash, number uint64) {
	deleteHeaderWithoutNumber(db, hash, number)
	if err := db.Delete(headerNumberKey(hash), "header"); err != nil {
		log.Critical("Failed to delete hash to number mapping", "err", err)
	}

	log.Debugf("DB DeleteHeader. key:h......, number:%d, hash:%x", number, hash)
}

// deleteHeaderWithoutNumber removes only the block header but does not remove
// the hash to number mapping.
func deleteHeaderWithoutNumber(db database.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(headerKey(number, hash), "headerWithoutNumber"); err != nil {
		log.Critical("Failed to delete header", "err", err)
	}
}

// isCanon is an internal utility method, to check whether the given number/hash
// is part of the ancient (canon) set.
func isCanon(reader database.AncientReaderOp, number uint64, hash common.Hash) bool {
	h, err := reader.Ancient(freezerHashTable, number)
	if err != nil {
		return false
	}
	return bytes.Equal(h, hash[:])
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLP(db database.Reader, hash common.Hash, number uint64) rlp.RawValue {
	//log.Debugf("DB ReadBodyRLP start. key: b......, number:%d, hash:%x", number, hash)
	// First try to look up the data in ancient database. Extra hash
	// comparison is necessary since ancient database only maintains
	// the canonical data.
	var data []byte
	_ = db.ReadAncients(func(reader database.AncientReaderOp) error {
		// Check if the data is in ancients
		if isCanon(reader, number, hash) {
			data, _ = reader.Ancient(freezerBodiesTable, number)
			return nil
		}
		// If not, try reading from leveldb
		data, _ = db.Get(blockBodyKey(number, hash), "bodyRLP")
		return nil
	})

	log.Debugf("DB ReadBodyRLP. key:b......, number:%d, hash:%x, vSize:%d", number, hash, len(data))
	return data
}

// ReadCanonicalBodyRLP retrieves the block body (transactions and uncles) for the canonical
// block at number, in RLP encoding.
func ReadCanonicalBodyRLP(db database.Reader, number uint64) rlp.RawValue {
	var data []byte
	var hash []byte
	_ = db.ReadAncients(func(reader database.AncientReaderOp) error {
		data, _ = reader.Ancient(freezerBodiesTable, number)
		if len(data) > 0 {
			return nil
		}
		// Block is not in ancients, read from leveldb by hash and number.
		// Note: ReadCanonicalHash cannot be used here because it also
		// calls ReadAncients internally.
		hash, _ = db.Get(headerHashKey(number), "headerHash")
		data, _ = db.Get(blockBodyKey(number, common.BytesToHash(hash)), "bodyRLP")
		return nil
	})

	log.Debugf("DB ReadCanonicalBodyRLP. key:b......, number:%d, hash:%x, vSize:%d", number, hash, len(data))
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLP(db database.KeyValueWriter, hash common.Hash, number uint64, value rlp.RawValue) {
	if err := db.Put(blockBodyKey(number, hash), value, "bodyRLP"); err != nil {
		log.Critical("Failed to store block body", "err", err)
	}

	log.Debugf("DB WriteBodyRLP. key:b......, number:%d, hash:%x, vSize:%d", number, hash, len(value))
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBody(db database.Reader, hash common.Hash, number uint64) bool {
	if isCanon(db, number, hash) {
		return true
	}

	res := true
	if has, err := db.Has(blockBodyKey(number, hash), "bodyRLP"); !has || err != nil {
		res = false
	}
	log.Debugf("DB HasBody. number:%d, hash:%x, result:%v", number, hash, res)
	return res
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBody(db database.Reader, hash common.Hash, number uint64) *model.Body {
	data := ReadBodyRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	body := new(model.Body)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}

	log.Debugf("DB ReadBody. key:b......, number:%d, hash:%x, txs:%d, uncles:%d", number, hash, len(body.Transactions), len(body.Uncles))
	return body
}

// WriteBody stores a block body into the database.
func WriteBody(db database.KeyValueWriter, hash common.Hash, number uint64, body *model.Body) {
	data, err := rlp.EncodeToBytes(body)
	if err != nil {
		log.Critical("Failed to RLP encode body", "err", err)
	}
	WriteBodyRLP(db, hash, number, data)

	log.Debugf("DB WriteBody. key:b......, number:%d, hash:%x, txs:%d, uncles:%d", number, hash, len(body.Transactions), len(body.Uncles))
}

// DeleteBody removes all block body data associated with a hash.
func DeleteBody(db database.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(blockBodyKey(number, hash), "bodyRLP"); err != nil {
		log.Critical("Failed to delete block body", "err", err)
	}

	log.Debugf("DB DeleteBody. key:b......, number:%d, hash:%x", number, hash)
}

// ReadTdRLP retrieves a block's total difficulty corresponding to the hash in RLP encoding.
func ReadTdRLP(db database.Reader, hash common.Hash, number uint64) rlp.RawValue {
	var data []byte
	_ = db.ReadAncients(func(reader database.AncientReaderOp) error {
		// Check if the data is in ancients
		if isCanon(reader, number, hash) {
			data, _ = reader.Ancient(freezerDifficultyTable, number)
			return nil
		}
		// If not, try reading from leveldb
		data, _ = db.Get(headerTDKey(number, hash), "headerTD")
		return nil
	})

	log.Debugf("DB ReadTdRLP. key:h......t, number:%d, hash:%x, vSize:%d", number, hash, len(data))
	return data
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTd(db database.Reader, hash common.Hash, number uint64) *big.Int {
	data := ReadTdRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	td := new(big.Int)
	if err := rlp.Decode(bytes.NewReader(data), td); err != nil {
		log.Error("Invalid block total difficulty RLP", "hash", hash, "err", err)
		return nil
	}

	log.Debugf("DB ReadTd. key:h......t, number:%d, hash:%x, td:%d", number, hash, td)
	return td
}

// WriteTd stores the total difficulty of a block into the database.
func WriteTd(db database.KeyValueWriter, hash common.Hash, number uint64, td *big.Int) {
	data, err := rlp.EncodeToBytes(td)
	if err != nil {
		log.Critical("Failed to RLP encode block total difficulty", "err", err)
	}
	if err := db.Put(headerTDKey(number, hash), data, "headerTD"); err != nil {
		log.Critical("Failed to store block total difficulty", "err", err)
	}

	log.Debugf("DB WriteTd. key:h......t, number:%d, hash:%x, td:%d", number, hash, td)
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTd(db database.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(headerTDKey(number, hash), "headTD"); err != nil {
		log.Critical("Failed to delete block total difficulty", "err", err)
	}

	log.Debugf("DB DeleteTd. key:h......t, number:%d, hash:%x", number, hash)
}

// HasReceipts verifies the existence of all the transaction receipts belonging
// to a block.
func HasReceipts(db database.Reader, hash common.Hash, number uint64) bool {
	if isCanon(db, number, hash) {
		return true
	}

	res := true
	if has, err := db.Has(blockReceiptsKey(number, hash), "receipts"); !has || err != nil {
		res = false
	}
	log.Debugf("DB HasReceipts. key:r......, number:%d, hash:%x, result:%v", number, hash, res)
	return true
}

// ReadReceiptsRLP retrieves all the transaction receipts belonging to a block in RLP encoding.
func ReadReceiptsRLP(db database.Reader, hash common.Hash, number uint64) rlp.RawValue {
	var data []byte
	_ = db.ReadAncients(func(reader database.AncientReaderOp) error {
		// Check if the data is in ancients
		if isCanon(reader, number, hash) {
			data, _ = reader.Ancient(freezerReceiptTable, number)
			return nil
		}
		// If not, try reading from leveldb
		data, _ = db.Get(blockReceiptsKey(number, hash), "receipts")
		return nil
	})

	log.Debugf("DB ReadReceiptsRLP. key:r......, number:%d, hash:%x, vSize:%d", number, hash, len(data))
	return data
}

// ReadRawReceipts retrieves all the transaction receipts belonging to a block.
// The receipt metadata fields are not guaranteed to be populated, so they
// should not be used. Use ReadReceipts instead if the metadata is needed.
func ReadRawReceipts(db database.Reader, hash common.Hash, number uint64) model.Receipts {
	// Retrieve the flattened receipt slice
	data := ReadReceiptsRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	// Convert the receipts from their storage form to their internal representation
	var storageReceipts []*model.ReceiptForStorage
	if err := rlp.DecodeBytes(data, &storageReceipts); err != nil {
		log.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}
	receipts := make(model.Receipts, len(storageReceipts))
	for i, storageReceipt := range storageReceipts {
		receipts[i] = (*model.Receipt)(storageReceipt)
	}

	log.Debugf("DB ReadRawReceipts. key:r......, number:%d, hash:%x, receipts:%d", number, hash, len(receipts))
	return receipts
}

// ReadReceipts retrieves all the transaction receipts belonging to a block, including
// its corresponding metadata fields. If it is unable to populate these metadata
// fields then nil is returned.
//
// The current implementation populates these metadata fields by reading the receipts'
// corresponding block body, so if the block body is not found it will return nil even
// if the receipt itself is stored.
func ReadReceipts(db database.Reader, hash common.Hash, number uint64, config *config.ChainConfig) model.Receipts {
	// We're deriving many fields from the block body, retrieve beside the receipt
	receipts := ReadRawReceipts(db, hash, number)
	if receipts == nil {
		return nil
	}
	body := ReadBody(db, hash, number)
	if body == nil {
		log.Error("Missing body but have receipt", "hash", hash, "number", number)
		return nil
	}
	if err := receipts.DeriveFields(config, hash, number, body.Transactions); err != nil {
		log.Error("Failed to derive block receipts fields", "hash", hash, "number", number, "err", err)
		return nil
	}

	log.Debugf("DB ReadReceipts. key:r......, number:%d, hash:%x, receipts:%d", number, hash, len(receipts))
	return receipts
}

// WriteReceipts stores all the transaction receipts belonging to a block.
func WriteReceipts(db database.KeyValueWriter, hash common.Hash, number uint64, receipts model.Receipts) {
	// Convert the receipts into their storage form and serialize them
	storageReceipts := make([]*model.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*model.ReceiptForStorage)(receipt)
	}
	byteArray, err := rlp.EncodeToBytes(storageReceipts)
	if err != nil {
		log.Critical("Failed to encode block receipts", "err", err)
	}
	// Store the flattened receipt slice
	if err := db.Put(blockReceiptsKey(number, hash), byteArray, "receipts"); err != nil {
		log.Critical("Failed to store block receipts", "err", err)
	}

	log.Debugf("DB WriteReceipts. key:r......, number:%d, hash:%x, receipts:%d, byteSize:%d", number, hash, len(receipts), len(byteArray))
}

// DeleteReceipts removes all receipt data associated with a block hash.
func DeleteReceipts(db database.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(blockReceiptsKey(number, hash), "receipts"); err != nil {
		log.Critical("Failed to delete block receipts", "err", err)
	}

	log.Debugf("DB DeleteReceipts. key:r......, number:%d, hash:%x", number, hash)
}

// storedReceiptRLP is the storage encoding of a receipt.
// Re-definition in core/types/receipt.go.
type storedReceiptRLP struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	Logs              []*model.LogForStorage
}

// ReceiptLogs is a barebone version of ReceiptForStorage which only keeps
// the list of logs. When decoding a stored receipt into this object we
// avoid creating the bloom filter.
type receiptLogs struct {
	Logs []*model.Log
}

// DecodeRLP implements rlp.Decoder.
func (r *receiptLogs) DecodeRLP(s *rlp.Stream) error {
	var stored storedReceiptRLP
	if err := s.Decode(&stored); err != nil {
		return err
	}
	r.Logs = make([]*model.Log, len(stored.Logs))
	for i, log := range stored.Logs {
		r.Logs[i] = (*model.Log)(log)
	}
	return nil
}

// DeriveLogFields fills the logs in receiptLogs with information such as block number, txhash, etc.
func deriveLogFields(receipts []*receiptLogs, hash common.Hash, number uint64, txs model.Transactions) error {
	logIndex := uint(0)
	if len(txs) != len(receipts) {
		return errors.New("transaction and receipt count mismatch")
	}
	for i := 0; i < len(receipts); i++ {
		txHash := txs[i].Hash()
		// The derived log fields can simply be set from the block and transaction
		for j := 0; j < len(receipts[i].Logs); j++ {
			receipts[i].Logs[j].BlockNumber = number
			receipts[i].Logs[j].BlockHash = hash
			receipts[i].Logs[j].TxHash = txHash
			receipts[i].Logs[j].TxIndex = uint(i)
			receipts[i].Logs[j].Index = logIndex
			logIndex++
		}
	}
	return nil
}

// ReadLogs retrieves the logs for all transactions in a block. The log fields
// are populated with metadata. In case the receipts or the block body
// are not found, a nil is returned.
func ReadLogs(db database.Reader, hash common.Hash, number uint64, config *config.ChainConfig) [][]*model.Log {
	//log.Debugf("DB ReadLogs start. number:%d, hash:%x", number, hash)
	// Retrieve the flattened receipt slice
	data := ReadReceiptsRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	var receipts []*receiptLogs
	if err := rlp.DecodeBytes(data, &receipts); err != nil {
		// Receipts might be in the legacy format, try decoding that.
		// TODO: to be removed after users migrated
		if logs := readLegacyLogs(db, hash, number, config); logs != nil {
			return logs
		}
		log.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}

	body := ReadBody(db, hash, number)
	if body == nil {
		log.Error("Missing body but have receipt", "hash", hash, "number", number)
		return nil
	}
	if err := deriveLogFields(receipts, hash, number, body.Transactions); err != nil {
		log.Error("Failed to derive block receipts fields", "hash", hash, "number", number, "err", err)
		return nil
	}
	logs := make([][]*model.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}

	log.Debugf("DB ReadLogs. number:%d, hash:%x, receipts:%d", number, hash, len(receipts))
	return logs
}

// readLegacyLogs is a temporary workaround for when trying to read logs
// from a block which has its receipt stored in the legacy format. It'll
// be removed after users have migrated their freezer databases.
func readLegacyLogs(db database.Reader, hash common.Hash, number uint64, config *config.ChainConfig) [][]*model.Log {
	receipts := ReadReceipts(db, hash, number, config)
	if receipts == nil {
		return nil
	}
	logs := make([][]*model.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs
}

// ReadBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body. If either the header or body could not
// be retrieved nil is returned.
//
// Note, due to concurrent download of header and block body the header and thus
// canonical hash can be stored in the database but the body data not (yet).
func ReadBlock(db database.Reader, hash common.Hash, number uint64) *model.Block {
	header := ReadHeader(db, hash, number)
	if header == nil {
		return nil
	}
	body := ReadBody(db, hash, number)
	if body == nil {
		return nil
	}
	block := model.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles)

	log.Debugf("DB ReadBlock. number:%d, hash:%x, txs:%d, uncles:%d", number, hash, len(body.Transactions), len(body.Uncles))
	return block
}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlock(db database.KeyValueWriter, block *model.Block) {
	WriteBody(db, block.Hash(), block.NumberU64(), block.Body())
	WriteHeader(db, block.Header())
	log.Debugf("DB WriteBlock. number:%d, hash:%x, txs:%d, uncles:%d", block.NumberU64(), block.Hash(), len(block.Body().Transactions), len(block.Body().Uncles))
}

// WriteAncientBlocks writes entire block data into ancient store and returns the total written size.
func WriteAncientBlocks(db database.AncientWriter, blocks []*model.Block, receipts []model.Receipts, td *big.Int) (int64, error) {
	log.Debugf("DB WriteAncientBlocks. blocks:%d, receipts:%d, td:%d", len(blocks), len(receipts), td)

	var (
		tdSum      = new(big.Int).Set(td)
		stReceipts []*model.ReceiptForStorage
	)
	return db.ModifyAncients(func(op database.AncientWriteOp) error {
		for i, block := range blocks {
			// Convert receipts to storage format and sum up total difficulty.
			stReceipts = stReceipts[:0]
			for _, receipt := range receipts[i] {
				stReceipts = append(stReceipts, (*model.ReceiptForStorage)(receipt))
			}
			header := block.Header()
			if i > 0 {
				tdSum.Add(tdSum, header.Difficulty)
			}
			if err := writeAncientBlock(op, block, header, stReceipts, tdSum); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeAncientBlock(op database.AncientWriteOp, block *model.Block, header *model.Header, receipts []*model.ReceiptForStorage, td *big.Int) error {
	num := block.NumberU64()
	log.Debugf("DB writeAncientBlock. block number:%d, hash:%x, receipts:%d, td:%d", num, header.Root, len(receipts), td)

	if err := op.AppendRaw(freezerHashTable, num, block.Hash().Bytes()); err != nil {
		return fmt.Errorf("can't add block %d hash: %v", num, err)
	}
	if err := op.Append(freezerHeaderTable, num, header); err != nil {
		return fmt.Errorf("can't append block header %d: %v", num, err)
	}
	if err := op.Append(freezerBodiesTable, num, block.Body()); err != nil {
		return fmt.Errorf("can't append block body %d: %v", num, err)
	}
	if err := op.Append(freezerReceiptTable, num, receipts); err != nil {
		return fmt.Errorf("can't append block %d receipts: %v", num, err)
	}
	if err := op.Append(freezerDifficultyTable, num, td); err != nil {
		return fmt.Errorf("can't append block %d total difficulty: %v", num, err)
	}
	return nil
}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlock(db database.KeyValueWriter, hash common.Hash, number uint64) {
	DeleteReceipts(db, hash, number)
	DeleteHeader(db, hash, number)
	DeleteBody(db, hash, number)
	DeleteTd(db, hash, number)
	log.Debugf("DB DeleteBlock. number:%d, hash:%x", number, hash)
}

// DeleteBlockWithoutNumber removes all block data associated with a hash, except
// the hash to number mapping.
func DeleteBlockWithoutNumber(db database.KeyValueWriter, hash common.Hash, number uint64) {
	DeleteReceipts(db, hash, number)
	deleteHeaderWithoutNumber(db, hash, number)
	DeleteBody(db, hash, number)
	DeleteTd(db, hash, number)
	log.Debugf("DB DeleteBlockWithoutNumber. number:%d, hash:%x", number, hash)
}

const badBlockToKeep = 10

type badBlock struct {
	Header *model.Header
	Body   *model.Body
}

// badBlockList implements the sort interface to allow sorting a list of
// bad blocks by their number in the reverse order.
type badBlockList []*badBlock

func (s badBlockList) Len() int { return len(s) }
func (s badBlockList) Less(i, j int) bool {
	return s[i].Header.Number.Uint64() < s[j].Header.Number.Uint64()
}
func (s badBlockList) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// ReadBadBlock retrieves the bad block with the corresponding block hash.
func ReadBadBlock(db database.Reader, hash common.Hash) *model.Block {
	blob, err := db.Get(badBlockKey, "invalidBlock")
	if err != nil {
		return nil
	}
	var badBlocks badBlockList
	if err := rlp.DecodeBytes(blob, &badBlocks); err != nil {
		return nil
	}
	for _, bad := range badBlocks {
		if bad.Header.Hash() == hash {
			block := model.NewBlockWithHeader(bad.Header).WithBody(bad.Body.Transactions, bad.Body.Uncles)

			log.Debugf("DB ReadBadBlock. key:InvalidBlock, number:%d, hash:%x", block.NumberU64(), hash)
			return block
		}
	}
	return nil
}

// ReadAllBadBlocks retrieves all the bad blocks in the database.
// All returned blocks are sorted in reverse order by number.
func ReadAllBadBlocks(db database.Reader) []*model.Block {
	blob, err := db.Get(badBlockKey, "invalidBlock")
	if err != nil {
		return nil
	}
	var badBlocks badBlockList
	if err := rlp.DecodeBytes(blob, &badBlocks); err != nil {
		return nil
	}
	var blocks []*model.Block
	for _, bad := range badBlocks {
		blocks = append(blocks, model.NewBlockWithHeader(bad.Header).WithBody(bad.Body.Transactions, bad.Body.Uncles))
	}

	log.Debugf("DB ReadAllBadBlocks. key:InvalidBlock, blocks:%d", len(blocks))
	return blocks
}

// WriteBadBlock serializes the bad block into the database. If the cumulated
// bad blocks exceeds the limitation, the oldest will be dropped.
func WriteBadBlock(db database.KeyValueStore, block *model.Block) {
	blob, err := db.Get(badBlockKey, "invalidBlock")
	if err != nil {
		log.Warning("Failed to load old bad blocks", "error", err)
	}
	var badBlocks badBlockList
	if len(blob) > 0 {
		if err := rlp.DecodeBytes(blob, &badBlocks); err != nil {
			log.Critical("Failed to decode old bad blocks", "error", err)
		}
	}
	for _, b := range badBlocks {
		if b.Header.Number.Uint64() == block.NumberU64() && b.Header.Hash() == block.Hash() {
			log.Info("Skip duplicated bad block", "number", block.NumberU64(), "hash", block.Hash())
			return
		}
	}
	badBlocks = append(badBlocks, &badBlock{
		Header: block.Header(),
		Body:   block.Body(),
	})
	sort.Sort(sort.Reverse(badBlocks))
	if len(badBlocks) > badBlockToKeep {
		badBlocks = badBlocks[:badBlockToKeep]
	}
	data, err := rlp.EncodeToBytes(badBlocks)
	if err != nil {
		log.Critical("Failed to encode bad blocks", "err", err)
	}
	if err := db.Put(badBlockKey, data, "invalidBlock"); err != nil {
		log.Critical("Failed to write bad blocks", "err", err)
	}

	log.Debugf("DB WriteBadBlock. key:InvalidBlock, blocks:%d, vSize:%d", len(badBlocks), len(data))
}

// DeleteBadBlocks deletes all the bad blocks from the database
func DeleteBadBlocks(db database.KeyValueWriter) {
	if err := db.Delete(badBlockKey, "invalidBlock"); err != nil {
		log.Critical("Failed to delete bad blocks", "err", err)
	}
	log.Debugf("DB DeleteBadBlocks. key: InvalidBlock")
}

// FindCommonAncestor returns the last common ancestor of two block headers
func FindCommonAncestor(db database.Reader, a, b *model.Header) *model.Header {
	for bn := b.Number.Uint64(); a.Number.Uint64() > bn; {
		a = ReadHeader(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
	}
	for an := a.Number.Uint64(); an < b.Number.Uint64(); {
		b = ReadHeader(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	for a.Hash() != b.Hash() {
		a = ReadHeader(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
		b = ReadHeader(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	return a
}

// ReadHeadHeader returns the current canonical head header.
func ReadHeadHeader(db database.Reader) *model.Header {
	headHeaderHash := ReadHeadHeaderHash(db)
	if headHeaderHash == (common.Hash{}) {
		return nil
	}
	headHeaderNumber := ReadHeaderNumber(db, headHeaderHash)
	if headHeaderNumber == nil {
		return nil
	}
	return ReadHeader(db, headHeaderHash, *headHeaderNumber)
}

// ReadHeadBlock returns the current canonical head block.
func ReadHeadBlock(db database.Reader) *model.Block {
	headBlockHash := ReadHeadBlockHash(db)
	if headBlockHash == (common.Hash{}) {
		return nil
	}
	headBlockNumber := ReadHeaderNumber(db, headBlockHash)
	if headBlockNumber == nil {
		return nil
	}
	return ReadBlock(db, headBlockHash, *headBlockNumber)
}
