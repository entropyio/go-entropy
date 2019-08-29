package mapper

import (
	"bytes"
	"encoding/binary"
	"math/big"

	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
)

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadCanonicalHash(db database.Reader, number uint64) common.Hash {
	data, _ := db.Ancient(freezerHashTable, number)
	if len(data) == 0 {
		key := headerHashKey(number)
		data, _ = db.Get(key)
		//mapperLog.Debugf("ReadCanonicalHash: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)

		// In the background freezer is moving data from leveldb to flatten files.
		// So during the first check for ancient db, the data is not yet in there,
		// but when we reach into leveldb, the data was already moved. That would
		// result in a not found error.
		if len(data) == 0 {
			data, _ = db.Ancient(freezerHashTable, number)
		}
	}
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteCanonicalHash(db database.KeyValueWriter, hash common.Hash, number uint64) {
	key := headerHashKey(number)
	data := hash.Bytes()
	if err := db.Put(key, data); err != nil {
		mapperLog.Critical("Failed to store number to hash mapping", "err", err)
	}
	//mapperLog.Debugf("WriteCanonicalHash: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHash(db database.KeyValueWriter, number uint64) {
	key := headerHashKey(number)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete number to hash mapping", "err", err)
	}
	//mapperLog.Debugf("DeleteCanonicalHash: number=%d, key=%c, key=%X", number, key[0], key)
}

// ReadAllHashes retrieves all the hashes assigned to blocks at a certain heights,
// both canonical and reorged forks included.
func ReadAllHashes(db database.Iteratee, number uint64) []common.Hash {
	prefix := headerKeyPrefix(number)

	hashes := make([]common.Hash, 0, 1)
	it := db.NewIteratorWithPrefix(prefix)
	defer it.Release()

	for it.Next() {
		if key := it.Key(); len(key) == len(prefix)+32 {
			hashes = append(hashes, common.BytesToHash(key[len(key)-32:]))
		}
	}
	return hashes
}

// ReadHeaderNumber returns the header number assigned to a hash.
func ReadHeaderNumber(db database.KeyValueReader, hash common.Hash) *uint64 {
	key := headerNumberKey(hash)
	data, _ := db.Get(key)
	//mapperLog.Debugf("ReadHeaderNumber: key=%c, key=%X, data=%X", key[0], key, data)

	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// WriteHeaderNumber stores the hash->number mapping.
func WriteHeaderNumber(db database.KeyValueWriter, hash common.Hash, number uint64) {
	key := headerNumberKey(hash)
	enc := encodeBlockNumber(number)
	if err := db.Put(key, enc); err != nil {
		mapperLog.Critical("Failed to store hash to number mapping", "err", err)
	}
	//mapperLog.Debugf("WriteHeadeNumber: number=%d, key=%c, key=%X, data=%X", number, key[0], key, enc)
}

// DeleteHeaderNumber removes hash->number mapping.
func DeleteHeaderNumber(db database.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(headerNumberKey(hash)); err != nil {
		mapperLog.Critical("Failed to delete hash to number mapping", "err", err)
	}
}

// ReadHeadHeaderHash retrieves the hash of the current canonical head header.
func ReadHeadHeaderHash(db database.KeyValueReader) common.Hash {
	data, _ := db.Get(headHeaderKey)
	//mapperLog.Debugf("ReadHeadHeaderHash: key=%s, key=%X, data=%X", headHeaderKey, headHeaderKey, data)

	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadHeaderHash stores the hash of the current canonical head header.
func WriteHeadHeaderHash(db database.KeyValueWriter, hash common.Hash) {
	data := hash.Bytes()
	if err := db.Put(headHeaderKey, data); err != nil {
		mapperLog.Critical("Failed to store last header's hash", "err", err)
	}
	//mapperLog.Debugf("WriteHeadHeaderHash: key=%s, key=%X, data=%X", headHeaderKey, headHeaderKey, data)
}

// ReadHeadBlockHash retrieves the hash of the current canonical head block.
func ReadHeadBlockHash(db database.Reader) common.Hash {
	data, _ := db.Get(headBlockKey)
	//mapperLog.Debugf("ReadHeadBlockHash: key=%s, key=%X, data=%X", headBlockKey, headBlockKey, data)

	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHash(db database.KeyValueWriter, hash common.Hash) {
	data := hash.Bytes()
	if err := db.Put(headBlockKey, data); err != nil {
		mapperLog.Critical("Failed to store last block's hash", "err", err)
	}
	//mapperLog.Debugf("WriteHeadBlockHash: key=%s, key=%X, data=%X", headBlockKey, headBlockKey, data)
}

// ReadHeadFastBlockHash retrieves the hash of the current fast-sync head block.
func ReadHeadFastBlockHash(db database.Reader) common.Hash {
	data, _ := db.Get(headFastBlockKey)
	//mapperLog.Debugf("ReadHeadFastBlockHash: key=%s, key=%X, data=%X", headFastBlockKey, headFastBlockKey, data)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadFastBlockHash stores the hash of the current fast-sync head block.
func WriteHeadFastBlockHash(db database.KeyValueWriter, hash common.Hash) {
	data := hash.Bytes()
	if err := db.Put(headFastBlockKey, data); err != nil {
		mapperLog.Critical("Failed to store last fast block's hash", "err", err)
	}
	//mapperLog.Debugf("WriteHeadFastBlockHash: key=%s, key=%X, data=%X", headFastBlockKey, headFastBlockKey, data)
}

// ReadFastTrieProgress retrieves the number of tries nodes fast synced to allow
// reporting correct numbers across restarts.
func ReadFastTrieProgress(db database.Reader) uint64 {
	data, _ := db.Get(fastTrieProgressKey)
	//mapperLog.Debugf("ReadFastTrieProgress: key=%s, key=%X, data=%X", fastTrieProgressKey, fastTrieProgressKey, data)
	if len(data) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(data).Uint64()
}

// WriteFastTrieProgress stores the fast sync trie process counter to support
// retrieving it across restarts.
func WriteFastTrieProgress(db database.KeyValueWriter, count uint64) {
	data := new(big.Int).SetUint64(count).Bytes()
	if err := db.Put(fastTrieProgressKey, data); err != nil {
		mapperLog.Critical("Failed to store fast sync trie progress", "err", err)
	}
	//mapperLog.Debugf("WriteFastTrieProgress: key=%c, key=%X, data=%X", headFastBlockKey, headFastBlockKey, data)
}

// ReadHeaderRLP retrieves a block header in its raw RLP database encoding.
func ReadHeaderRLP(db database.Reader, hash common.Hash, number uint64) rlputil.RawValue {
	data, _ := db.Ancient(freezerHeaderTable, number)
	if len(data) == 0 {
		key := headerKey(number, hash)
		data, _ = db.Get(key)

		//mapperLog.Debugf("ReadHeaderRLP: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
		// In the background freezer is moving data from leveldb to flatten files.
		// So during the first check for ancient db, the data is not yet in there,
		// but when we reach into leveldb, the data was already moved. That would
		// result in a not found error.
		if len(data) == 0 {
			data, _ = db.Ancient(freezerHeaderTable, number)
		}
	}

	return data
}

// HasHeader verifies the existence of a block header corresponding to the hash.
func HasHeader(db database.Reader, hash common.Hash, number uint64) bool {
	if has, err := db.Ancient(freezerHashTable, number); err == nil && common.BytesToHash(has) == hash {
		return true
	}

	key := headerKey(number, hash)
	if has, err := db.Has(key); !has || err != nil {
		return false
	}
	return true
}

// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeader(db database.Reader, hash common.Hash, number uint64) *model.Header {
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
func WriteHeader(db database.KeyValueWriter, header *model.Header) {
	var (
		hash   = header.Hash()
		number = header.Number.Uint64()
	)
	// Write the hash -> number mapping
	WriteHeaderNumber(db, hash, number)

	// Write the encoded header
	data, err := rlputil.EncodeToBytes(header)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode header", "err", err)
	}
	key := headerKey(number, hash)
	if err := db.Put(key, data); err != nil {
		mapperLog.Critical("Failed to store header", "err", err)
	}
	//mapperLog.Debugf("WriteHeaderRLP: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeader(db database.KeyValueWriter, hash common.Hash, number uint64) {
	deleteHeaderWithoutNumber(db, hash, number)

	key := headerNumberKey(hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete hash to number mapping", "err", err)
	}
	//mapperLog.Debugf("DeleteHeader header number: number=%d, key=%c, key=%X", number, key[0], key)
}

// deleteHeaderWithoutNumber removes only the block header but does not remove
// the hash to number mapping.
func deleteHeaderWithoutNumber(db database.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(headerKey(number, hash)); err != nil {
		mapperLog.Critical("Failed to delete header", "err", err)
	}
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLP(db database.Reader, hash common.Hash, number uint64) rlputil.RawValue {
	data, _ := db.Ancient(freezerBodiesTable, number)
	if len(data) == 0 {
		key := blockBodyKey(number, hash)
		data, _ = db.Get(key)
		//mapperLog.Debugf("ReadBodyRLP: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)

		// In the background freezer is moving data from leveldb to flatten files.
		// So during the first check for ancient db, the data is not yet in there,
		// but when we reach into leveldb, the data was already moved. That would
		// result in a not found error.
		if len(data) == 0 {
			data, _ = db.Ancient(freezerBodiesTable, number)
		}
	}
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLP(db database.KeyValueWriter, hash common.Hash, number uint64, rlp rlputil.RawValue) {
	key := blockBodyKey(number, hash)
	if err := db.Put(key, rlp); err != nil {
		mapperLog.Critical("Failed to store block body", "err", err)
	}
	//mapperLog.Debugf("WriteBodyRLP: number=%d, key=%c, key=%X, data=%X", number, key[0], key, rlp)
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBody(db database.Reader, hash common.Hash, number uint64) bool {
	if has, err := db.Ancient(freezerHashTable, number); err == nil && common.BytesToHash(has) == hash {
		return true
	}

	key := blockBodyKey(number, hash)
	if has, err := db.Has(key); !has || err != nil {
		return false
	}
	return true
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBody(db database.Reader, hash common.Hash, number uint64) *model.Body {
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
func WriteBody(db database.KeyValueWriter, hash common.Hash, number uint64, body *model.Body) {
	data, err := rlputil.EncodeToBytes(body)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode body", "err", err)
	}
	WriteBodyRLP(db, hash, number, data)
}

// DeleteBody removes all block body data associated with a hash.
func DeleteBody(db database.KeyValueWriter, hash common.Hash, number uint64) {
	key := blockBodyKey(number, hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete block body", "err", err)
	}
	//mapperLog.Debugf("DeleteBody: key=%X", key)
}

// ReadTdRLP retrieves a block's total difficulty corresponding to the hash in RLP encoding.
func ReadTdRLP(db database.Reader, hash common.Hash, number uint64) rlputil.RawValue {
	data, _ := db.Ancient(freezerDifficultyTable, number)
	if len(data) == 0 {
		key := headerTDKey(number, hash)
		data, _ = db.Get(key)
		//mapperLog.Debugf("ReadTd: number=%d, key=%c, key=%X, data=%X", number, key[0], key, data)

		// In the background freezer is moving data from leveldb to flatten files.
		// So during the first check for ancient db, the data is not yet in there,
		// but when we reach into leveldb, the data was already moved. That would
		// result in a not found error.
		if len(data) == 0 {
			data, _ = db.Ancient(freezerDifficultyTable, number)
		}
	}
	return data
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTd(db database.Reader, hash common.Hash, number uint64) *big.Int {
	data := ReadTdRLP(db, hash, number)
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
func WriteTd(db database.KeyValueWriter, hash common.Hash, number uint64, td *big.Int) {
	data, err := rlputil.EncodeToBytes(td)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode block total difficulty", "err", err)
	}
	key := headerTDKey(number, hash)
	if err := db.Put(key, data); err != nil {
		mapperLog.Critical("Failed to store block total difficulty", "err", err)
	}
	//mapperLog.Debugf("WriteTd: number=%d, key=%c, key=%X, value=%X", number, key[0], key, data)
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTd(db database.KeyValueWriter, hash common.Hash, number uint64) {
	key := headerTDKey(number, hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete block total difficulty", "err", err)
	}
	//mapperLog.Debugf("DeleteTd: number=%d, key=%c, key=%X", number, key[0], key)
}

// HasReceipts verifies the existence of all the transaction receipts belonging
// to a block.
func HasReceipts(db database.Reader, hash common.Hash, number uint64) bool {
	if has, err := db.Ancient(freezerHashTable, number); err == nil && common.BytesToHash(has) == hash {
		return true
	}

	if has, err := db.Has(blockReceiptsKey(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadReceiptsRLP retrieves all the transaction receipts belonging to a block in RLP encoding.
func ReadReceiptsRLP(db database.Reader, hash common.Hash, number uint64) rlputil.RawValue {
	data, _ := db.Ancient(freezerReceiptTable, number)
	if len(data) == 0 {
		key := blockReceiptsKey(number, hash)
		data, _ = db.Get(key)
		//mapperLog.Debugf("ReadReceipts: number=%d, key=%c, key=%X, value=%X", number, key[0], key, data)

		// In the background freezer is moving data from leveldb to flatten files.
		// So during the first check for ancient db, the data is not yet in there,
		// but when we reach into leveldb, the data was already moved. That would
		// result in a not found error.
		if len(data) == 0 {
			data, _ = db.Ancient(freezerReceiptTable, number)
		}
	}
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
	storageReceipts := []*model.ReceiptForStorage{}
	if err := rlputil.DecodeBytes(data, &storageReceipts); err != nil {
		mapperLog.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}
	receipts := make(model.Receipts, len(storageReceipts))
	for i, storageReceipt := range storageReceipts {
		receipts[i] = (*model.Receipt)(storageReceipt)
	}
	return receipts
}

// ReadReceipts retrieves all the transaction receipts belonging to a block, including
// its correspoinding metadata fields. If it is unable to populate these metadata
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
		mapperLog.Error("Missing body but have receipt", "hash", hash, "number", number)
		return nil
	}
	if err := receipts.DeriveFields(config, hash, number, body.Transactions); err != nil {
		mapperLog.Error("Failed to derive block receipts fields", "hash", hash, "number", number, "err", err)
		return nil
	}
	return receipts
}

// WriteReceipts stores all the transaction receipts belonging to a block.
func WriteReceipts(db database.KeyValueWriter, hash common.Hash, number uint64, receipts model.Receipts) {
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
	//mapperLog.Debugf("WriteReceipts: number=%d, key=%c, key=%X, value=%X", number, key[0], key, bytesArray)
}

// DeleteReceipts removes all receipt data associated with a block hash.
func DeleteReceipts(db database.KeyValueWriter, hash common.Hash, number uint64) {
	key := blockReceiptsKey(number, hash)
	if err := db.Delete(key); err != nil {
		mapperLog.Critical("Failed to delete block receipts", "err", err)
	}
	//mapperLog.Debugf("DeleteReceipts: number=%d, key=%c, key=%X", number, key[0], key)
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
	return model.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles)
}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlock(db database.KeyValueWriter, block *model.Block) {
	WriteBody(db, block.Hash(), block.NumberU64(), block.Body())
	WriteHeader(db, block.Header())
}

// WriteAncientBlock writes entire block data into ancient store and returns the total written size.
func WriteAncientBlock(db database.AncientWriter, block *model.Block, receipts model.Receipts, td *big.Int) int {
	// Encode all block components to RLP format.
	headerBlob, err := rlputil.EncodeToBytes(block.Header())
	if err != nil {
		mapperLog.Critical("Failed to RLP encode block header", "err", err)
	}
	bodyBlob, err := rlputil.EncodeToBytes(block.Body())
	if err != nil {
		mapperLog.Critical("Failed to RLP encode body", "err", err)
	}
	storageReceipts := make([]*model.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*model.ReceiptForStorage)(receipt)
	}
	receiptBlob, err := rlputil.EncodeToBytes(storageReceipts)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode block receipts", "err", err)
	}
	tdBlob, err := rlputil.EncodeToBytes(td)
	if err != nil {
		mapperLog.Critical("Failed to RLP encode block total difficulty", "err", err)
	}
	// Write all blob to flatten files.
	err = db.AppendAncient(block.NumberU64(), block.Hash().Bytes(), headerBlob, bodyBlob, receiptBlob, tdBlob)
	if err != nil {
		mapperLog.Critical("Failed to write block data to ancient store", "err", err)
	}
	return len(headerBlob) + len(bodyBlob) + len(receiptBlob) + len(tdBlob) + common.HashLength
}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlock(db database.KeyValueWriter, hash common.Hash, number uint64) {
	DeleteReceipts(db, hash, number)
	DeleteHeader(db, hash, number)
	DeleteBody(db, hash, number)
	DeleteTd(db, hash, number)
}

// DeleteBlockWithoutNumber removes all block data associated with a hash, except
// the hash to number mapping.
func DeleteBlockWithoutNumber(db database.KeyValueWriter, hash common.Hash, number uint64) {
	DeleteReceipts(db, hash, number)
	deleteHeaderWithoutNumber(db, hash, number)
	DeleteBody(db, hash, number)
	DeleteTd(db, hash, number)
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
