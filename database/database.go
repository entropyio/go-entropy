package database

/*
database package implement the product LDB storage and test memory storage.
*/

// Code using batches should try to add this much data to the batch.
// The value was determined empirically.
const IdealBatchSize = 100 * 1024

// DatabaseReader wraps the Has and Get method of a backing data store.
type DBReader interface {
	Has(key []byte) (bool, error)
	Get(key []byte) ([]byte, error)
}

// DatabaseWriter wraps the Put method of a backing data store.
type DBWriter interface {
	Put(key []byte, value []byte) error
}

// DatabaseDeleter wraps the Delete method of a backing data store.
type DBDeleter interface {
	Delete(key []byte) error
}

// Database wraps all database operations. All methods are safe for concurrent use.
type Database interface {
	Has(key []byte) (bool, error)
	Get(key []byte) ([]byte, error)

	Put(key []byte, value []byte) error

	Delete(key []byte) error

	Close()
	NewBatch() Batch
}

// Batch is a write-only database that commits changes to its host database
// when Write is called. Batch cannot be used concurrently.
type Batch interface {
	Put(key []byte, value []byte) error

	Delete(key []byte) error

	ValueSize() int // amount of data in the batch
	Write() error
	Reset()
}
