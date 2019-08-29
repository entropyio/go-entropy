package mapper

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
	"math/big"
)

// ReadTxLookupEntry retrieves the positional metadata associated with a transaction
// hash to allow retrieving the transaction or receipt by hash.
func ReadTxLookupEntry(db database.Reader, hash common.Hash) *uint64 {
	key := txLookupKey(hash)
	data, _ := db.Get(key)
	//mapperLog.Debugf("ReadTxLookupEntry: hash=%X, key=%X, data=%X", hash, key, data)

	if len(data) == 0 {
		return nil
	}
	// Database v6 tx lookup just stores the block number
	if len(data) < common.HashLength {
		number := new(big.Int).SetBytes(data).Uint64()
		return &number
	}
	// Database v4-v5 tx lookup format just stores the hash
	if len(data) == common.HashLength {
		return ReadHeaderNumber(db, common.BytesToHash(data))
	}
	// Finally try database v3 tx lookup format
	var entry LegacyTxLookupEntry
	if err := rlputil.DecodeBytes(data, &entry); err != nil {
		mapperLog.Error("Invalid transaction lookup entry rlputil", "hash", hash, "blob", data, "err", err)
		return nil
	}
	return &entry.BlockIndex
}

// WriteTxLookupEntries stores a positional metadata for every transaction from
// a block, enabling hash based transaction and receipt lookups.
func WriteTxLookupEntries(db database.KeyValueWriter, block *model.Block) {
	number := block.Number().Bytes()
	for _, tx := range block.Transactions() {
		key := txLookupKey(tx.Hash())
		//mapperLog.Debugf("WriteTxLookupEntries: hash=%X, key=%X, data=%X", tx.Hash(), key, number)

		if err := db.Put(key, number); err != nil {
			mapperLog.Critical("Failed to store transaction lookup entry", "err", err)
		}
	}
}

// DeleteTxLookupEntry removes all transaction data associated with a hash.
func DeleteTxLookupEntry(db database.KeyValueWriter, hash common.Hash) {
	key := txLookupKey(hash)
	//mapperLog.Debugf("DeleteTxLookupEntry: hash=%X, key=%X", hash, key)
	db.Delete(key)
}

// ReadTransaction retrieves a specific transaction from the database, along with
// its added positional metadata.
func ReadTransaction(db database.Reader, hash common.Hash) (*model.Transaction, common.Hash, uint64, uint64) {
	blockNumber := ReadTxLookupEntry(db, hash)
	if blockNumber == nil {
		return nil, common.Hash{}, 0, 0
	}
	blockHash := ReadCanonicalHash(db, *blockNumber)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}
	body := ReadBody(db, blockHash, *blockNumber)
	if body == nil {
		mapperLog.Error("Transaction referenced missing", "number", blockNumber, "hash", blockHash)
		return nil, common.Hash{}, 0, 0
	}
	for txIndex, tx := range body.Transactions {
		if tx.Hash() == hash {
			return tx, blockHash, *blockNumber, uint64(txIndex)
		}
	}
	mapperLog.Error("Transaction not found", "number", blockNumber, "hash", blockHash, "txhash", hash)
	return nil, common.Hash{}, 0, 0
}

// ReadReceipt retrieves a specific transaction receipt from the database, along with
// its added positional metadata.
func ReadReceipt(db database.Reader, hash common.Hash, config *config.ChainConfig) (*model.Receipt, common.Hash, uint64, uint64) {
	// Retrieve the context of the receipt based on the transaction hash
	blockNumber := ReadTxLookupEntry(db, hash)
	if blockNumber == nil {
		return nil, common.Hash{}, 0, 0
	}
	blockHash := ReadCanonicalHash(db, *blockNumber)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}
	// Read all the receipts from the block and return the one with the matching hash
	receipts := ReadReceipts(db, blockHash, *blockNumber, config)
	for receiptIndex, receipt := range receipts {
		if receipt.TxHash == hash {
			return receipt, blockHash, *blockNumber, uint64(receiptIndex)
		}
	}
	mapperLog.Error("Receipt not found", "number", blockNumber, "hash", blockHash, "txhash", hash)
	return nil, common.Hash{}, 0, 0
}

// ReadBloomBits retrieves the compressed bloom bit vector belonging to the given
// section and bit index from the.
func ReadBloomBits(db database.KeyValueReader, bit uint, section uint64, head common.Hash) ([]byte, error) {
	return db.Get(bloomBitsKey(bit, section, head))
}

// WriteBloomBits stores the compressed bloom bits vector belonging to the given
// section and bit index.
func WriteBloomBits(db database.KeyValueWriter, bit uint, section uint64, head common.Hash, bits []byte) {
	key := bloomBitsKey(bit, section, head)
	//mapperLog.Debugf("WriteBloomBits: head=%X, key=%X, data=%X", head, key, bits)

	if err := db.Put(key, bits); err != nil {
		mapperLog.Critical("Failed to store bloom bits", "err", err)
	}
}
