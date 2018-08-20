package mapper

import (
	"bytes"
	"encoding/binary"
	"math/big"

	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/database"
)

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadCanonicalHash(db database.DBReader, number uint64) common.Hash {
	key := headerHashKey(number)
	data, _ := db.Get(key)
	mapperLog.Debugf("ReadCanonicalHash: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteCanonicalHash(db database.DBWriter, hash common.Hash, number uint64) {
	key := headerHashKey(number)
	data := hash.Bytes()
	if err := db.Put(key, data); err != nil {
		mapperLog.Critical("Failed to store number to hash mapping", "err", err)
	}
	mapperLog.Debugf("WriteCanonicalHash: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHash(db database.DBDeleter, number uint64) {
	key := headerHashKey(number)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete number to hash mapping", "err", err)
	}
	mapperLog.Debugf("DeleteCanonicalHash: number=%d, key=%c, key=%X", number, key[0], key)
}

// ReadHeaderNumber returns the header number assigned to a hash.
func ReadHeaderNumber(db database.DBReader, hash common.Hash) *uint64 {
	key := headerNumberKey(hash)
	data, _ := db.Get(key)
	mapperLog.Debugf("ReadHeaderNumber: key=%c, key=%X, data=%X", key[0], key, data)

	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// ReadHeadHeaderHash retrieves the hash of the current canonical head header.
func ReadHeadHeaderHash(db database.DBReader) common.Hash {
	data, _ := db.Get(headHeaderKey)
	mapperLog.Debugf("ReadHeadHeaderHash: key=%s, key=%X, data=%X", headHeaderKey, headHeaderKey, data)

	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadHeaderHash stores the hash of the current canonical head header.
func WriteHeadHeaderHash(db database.DBWriter, hash common.Hash) {
	data := hash.Bytes()
	if err := db.Put(headHeaderKey, data); err != nil {
		mapperLog.Critical("Failed to store last header's hash", "err", err)
	}
	mapperLog.Debugf("WriteHeadHeaderHash: key=%s, key=%X, data=%X", headHeaderKey, headHeaderKey, data)
}

// ReadHeadBlockHash retrieves the hash of the current canonical head block.
func ReadHeadBlockHash(db database.DBReader) common.Hash {
	data, _ := db.Get(headBlockKey)
	mapperLog.Debugf("ReadHeadBlockHash: key=%s, key=%X, data=%X", headBlockKey, headBlockKey, data)

	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHash(db database.DBWriter, hash common.Hash) {
	data := hash.Bytes()
	if err := db.Put(headBlockKey, data); err != nil {
		mapperLog.Critical("Failed to store last block's hash", "err", err)
	}
	mapperLog.Debugf("WriteHeadBlockHash: key=%s, key=%X, data=%X", headBlockKey, headBlockKey, data)
}

// ReadHeadFastBlockHash retrieves the hash of the current fast-sync head block.
func ReadHeadFastBlockHash(db database.DBReader) common.Hash {
	data, _ := db.Get(headFastBlockKey)
	mapperLog.Debugf("ReadHeadFastBlockHash: key=%s, key=%X, data=%X", headFastBlockKey, headFastBlockKey, data)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadFastBlockHash stores the hash of the current fast-sync head block.
func WriteHeadFastBlockHash(db database.DBWriter, hash common.Hash) {
	data := hash.Bytes()
	if err := db.Put(headFastBlockKey, data); err != nil {
		mapperLog.Critical("Failed to store last fast block's hash", "err", err)
	}
	mapperLog.Debugf("WriteHeadFastBlockHash: key=%s, key=%X, data=%X", headFastBlockKey, headFastBlockKey, data)
}

// ReadFastTrieProgress retrieves the number of tries nodes fast synced to allow
// reporting correct numbers across restarts.
func ReadFastTrieProgress(db database.DBReader) uint64 {
	data, _ := db.Get(fastTrieProgressKey)
	mapperLog.Debugf("ReadFastTrieProgress: key=%s, key=%X, data=%X", fastTrieProgressKey, fastTrieProgressKey, data)
	if len(data) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(data).Uint64()
}

// WriteFastTrieProgress stores the fast sync trie process counter to support
// retrieving it across restarts.
func WriteFastTrieProgress(db database.DBWriter, count uint64) {
	data := new(big.Int).SetUint64(count).Bytes()
	if err := db.Put(fastTrieProgressKey, data); err != nil {
		mapperLog.Critical("Failed to store fast sync trie progress", "err", err)
	}
	mapperLog.Debugf("WriteFastTrieProgress: key=%c, key=%X, data=%X", headFastBlockKey, headFastBlockKey, data)
}

// ReadHeaderRLP retrieves a block header in its raw RLP database encoding.
func ReadHeaderRLP(db database.DBReader, hash common.Hash, number uint64) rlputil.RawValue {
	key := headerKey(number, hash)
	data, _ := db.Get(key)
	mapperLog.Debugf("ReadHeaderRLP: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
	return data
}

// HasHeader verifies the existence of a block header corresponding to the hash.
func HasHeader(db database.DBReader, hash common.Hash, number uint64) bool {
	key := headerKey(number, hash)
	if has, err := db.Has(key); !has || err != nil {
		return false
	}
	return true
}

// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeader(db database.DBReader, hash common.Hash, number uint64) *model.Header {
	data := ReadHeaderRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	header := new(model.Header)
	if err := rlputil.Decode(bytes.NewReader(data), header); err != nil {
		mapperLog.Errorf("Invalid block header RLP. hash: %x, error: %s", hash, err)
		return nil
	}
	return header
}

// WriteHeader stores a block header into the database and also stores the hash-
// to-number mapping.
func WriteHeader(db database.DBWriter, header *model.Header) {
	// Write the hash -> number mapping
	var (
		hash    = header.Hash()
		number  = header.Number.Uint64()
		encoded = encodeBlockNumber(number)
	)
	key := headerNumberKey(hash)
	if err := db.Put(key, encoded); err != nil {
		mapperLog.Critical("Failed to store hash to number mapping", "err", err)
	}
	mapperLog.Debugf("WriteHeader header number: number=%d, key=%c, key=%X, data=%X", number, key[0], key, encoded)

	// Write the encoded header
	data, err := rlputil.EncodeToBytes(header)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode header", "err", err)
	}
	key = headerKey(number, hash)
	if err := db.Put(key, data); err != nil {
		mapperLog.Critical("Failed to store header", "err", err)
	}
	mapperLog.Debugf("WriteHeader header rlputil: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeader(db database.DBDeleter, hash common.Hash, number uint64) {
	key := headerKey(number, hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete header", "err", err)
	}
	mapperLog.Debugf("DeleteHeader header rlputil: number=%d, key=%c, key=%X", number, key[0], key)

	key = headerNumberKey(hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete hash to number mapping", "err", err)
	}
	mapperLog.Debugf("DeleteHeader header number: number=%d, key=%c, key=%X", number, key[0], key)
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLP(db database.DBReader, hash common.Hash, number uint64) rlputil.RawValue {
	key := blockBodyKey(number, hash)
	data, _ := db.Get(key)
	mapperLog.Debugf("ReadBodyRLP: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLP(db database.DBWriter, hash common.Hash, number uint64, rlp rlputil.RawValue) {
	key := blockBodyKey(number, hash)
	if err := db.Put(key, rlp); err != nil {
		mapperLog.Critical("Failed to store block body", "err", err)
	}
	mapperLog.Debugf("WriteBodyRLP: number=%d, key=%c, key=%X, data=%X", number, key[0], key, rlp)
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBody(db database.DBReader, hash common.Hash, number uint64) bool {
	key := blockBodyKey(number, hash)
	if has, err := db.Has(key); !has || err != nil {
		return false
	}
	return true
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBody(db database.DBReader, hash common.Hash, number uint64) *model.Body {
	data := ReadBodyRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	body := new(model.Body)
	if err := rlputil.Decode(bytes.NewReader(data), body); err != nil {
		mapperLog.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}
	return body
}

// WriteBody storea a block body into the database.
func WriteBody(db database.DBWriter, hash common.Hash, number uint64, body *model.Body) {
	data, err := rlputil.EncodeToBytes(body)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode body", "err", err)
	}
	WriteBodyRLP(db, hash, number, data)
}

// DeleteBody removes all block body data associated with a hash.
func DeleteBody(db database.DBDeleter, hash common.Hash, number uint64) {
	key := blockBodyKey(number, hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete block body", "err", err)
	}
	mapperLog.Debugf("DeleteBody: key=%X", key)
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTd(db database.DBReader, hash common.Hash, number uint64) *big.Int {
	key := headerTDKey(number, hash)
	data, _ := db.Get(key)
	mapperLog.Debugf("ReadTd: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)

	if len(data) == 0 {
		return nil
	}
	td := new(big.Int)
	if err := rlputil.Decode(bytes.NewReader(data), td); err != nil {
		mapperLog.Error("Invalid block total difficulty RLP", "hash", hash, "err", err)
		return nil
	}
	return td
}

// WriteTd stores the total difficulty of a block into the database.
func WriteTd(db database.DBWriter, hash common.Hash, number uint64, td *big.Int) {
	data, err := rlputil.EncodeToBytes(td)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode block total difficulty", "err", err)
	}
	key := headerTDKey(number, hash)
	if err := db.Put(key, data); err != nil {
		mapperLog.Critical("Failed to store block total difficulty", "err", err)
	}
	mapperLog.Debugf("WriteTd: number=%d, key=%c, key=%X, value=%X", number, key[0], key, data)
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTd(db database.DBDeleter, hash common.Hash, number uint64) {
	key := headerTDKey(number, hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete block total difficulty", "err", err)
	}
	mapperLog.Debugf("DeleteTd: number=%d, key=%c, key=%X", number, key[0], key)
}

// ReadReceipts retrieves all the transaction receipts belonging to a block.
func ReadReceipts(db database.DBReader, hash common.Hash, number uint64) model.Receipts {
	key := blockReceiptsKey(number, hash)
	// Retrieve the flattened receipt slice
	data, _ := db.Get(key)
	mapperLog.Debugf("ReadReceipts: number=%d, key=%c, key=%X, value=%X", number, key[0], key, data)

	if len(data) == 0 {
		return nil
	}
	// Convert the revceipts from their storage form to their internal representation
	var storageReceipts []*model.ReceiptForStorage
	if err := rlputil.DecodeBytes(data, &storageReceipts); err != nil {
		mapperLog.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}
	receipts := make(model.Receipts, len(storageReceipts))
	for i, receipt := range storageReceipts {
		receipts[i] = (*model.Receipt)(receipt)
	}
	return receipts
}

// WriteReceipts stores all the transaction receipts belonging to a block.
func WriteReceipts(db database.DBWriter, hash common.Hash, number uint64, receipts model.Receipts) {
	// Convert the receipts into their storage form and serialize them
	storageReceipts := make([]*model.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*model.ReceiptForStorage)(receipt)
	}
	bytesArray, err := rlputil.EncodeToBytes(storageReceipts)
	if err != nil {
		mapperLog.Critical("Failed to encode block receipts", "err", err)
	}
	// Store the flattened receipt slice
	key := blockReceiptsKey(number, hash)
	if err := db.Put(key, bytesArray); err != nil {
		mapperLog.Critical("Failed to store block receipts", "err", err)
	}
	mapperLog.Debugf("WriteReceipts: number=%d, key=%c, key=%X, value=%X", number, key[0], key, bytesArray)
}

// DeleteReceipts removes all receipt data associated with a block hash.
func DeleteReceipts(db database.DBDeleter, hash common.Hash, number uint64) {
	key := blockReceiptsKey(number, hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete block receipts", "err", err)
	}
	mapperLog.Debugf("DeleteReceipts: number=%d, key=%c, key=%X", number, key[0], key)
}

// ReadBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body. If either the header or body could not
// be retrieved nil is returned.
//
// Note, due to concurrent download of header and block body the header and thus
// canonical hash can be stored in the database but the body data not (yet).
func ReadBlock(db database.DBReader, hash common.Hash, number uint64) *model.Block {
	header := ReadHeader(db, hash, number)
	if header == nil {
		return nil
	}
	body := ReadBody(db, hash, number)
	if body == nil {
		return nil
	}
	return model.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles)
}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlock(db database.DBWriter, block *model.Block) {
	WriteBody(db, block.Hash(), block.NumberU64(), block.Body())
	WriteHeader(db, block.Header())
}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlock(db database.DBDeleter, hash common.Hash, number uint64) {
	DeleteReceipts(db, hash, number)
	DeleteHeader(db, hash, number)
	DeleteBody(db, hash, number)
	DeleteTd(db, hash, number)
}

// FindCommonAncestor returns the last common ancestor of two block headers
func FindCommonAncestor(db database.DBReader, a, b *model.Header) *model.Header {
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
