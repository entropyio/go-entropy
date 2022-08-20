package remotedb

import (
	"bytes"
	"errors"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/rpc"
	"strings"
)

var log = logger.NewLogger("[remoteDB]")

// Database is a key-value lookup for a remote database via debug_dbGet.
type Database struct {
	remote *rpc.Client
}

func (db *Database) Has(key []byte, from string) (bool, error) {
	res := true
	if _, err := db.Get(key, from); err != nil {
		res = false
	}

	log.Debugf("remote Has %s. key:%s, value:%v", from, getStringKey(key), res)
	return res, nil
}

func (db *Database) Get(key []byte, from string) ([]byte, error) {
	var resp hexutil.Bytes
	err := db.remote.Call(&resp, "debug_dbGet", hexutil.Bytes(key))
	if err != nil {
		return nil, err
	}

	log.Debugf("remote Get %s. key:%s, vSize:%v", from, getStringKey(key), len(resp))
	return resp, nil
}

func (db *Database) HasAncient(kind string, number uint64) (bool, error) {
	if _, err := db.Ancient(kind, number); err != nil {
		return false, nil
	}
	return true, nil
}

func (db *Database) Ancient(kind string, number uint64) ([]byte, error) {
	var resp hexutil.Bytes
	err := db.remote.Call(&resp, "debug_dbAncient", kind, number)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (db *Database) AncientRange(kind string, start, count, maxBytes uint64) ([][]byte, error) {
	panic("not supported")
}

func (db *Database) Ancients() (uint64, error) {
	var resp uint64
	err := db.remote.Call(&resp, "debug_dbAncients")
	return resp, err
}

func (db *Database) Tail() (uint64, error) {
	panic("not supported")
}

func (db *Database) AncientSize(kind string) (uint64, error) {
	panic("not supported")
}

func (db *Database) ReadAncients(fn func(op database.AncientReaderOp) error) (err error) {
	return fn(db)
}

func (db *Database) Put([]byte, []byte, string) error {
	panic("not supported")
}

func (db *Database) Delete([]byte, string) error {
	panic("not supported")
}

func (db *Database) ModifyAncients(f func(database.AncientWriteOp) error) (int64, error) {
	panic("not supported")
}

func (db *Database) TruncateHead(n uint64) error {
	panic("not supported")
}

func (db *Database) TruncateTail(n uint64) error {
	panic("not supported")
}

func (db *Database) Sync() error {
	return nil
}

func (db *Database) MigrateTable(s string, f func([]byte) ([]byte, error)) error {
	panic("not supported")
}

func (db *Database) NewBatch() database.Batch {
	panic("not supported")
}

func (db *Database) NewBatchWithSize(size int) database.Batch {
	panic("not supported")
}

func (db *Database) NewIterator(prefix []byte, start []byte) database.Iterator {
	panic("not supported")
}

func (db *Database) Stat(property string) (string, error) {
	panic("not supported")
}

func (db *Database) AncientDatadir() (string, error) {
	panic("not supported")
}

func (db *Database) Compact(start []byte, limit []byte) error {
	return nil
}

func (db *Database) NewSnapshot() (database.Snapshot, error) {
	panic("not supported")
}

func (db *Database) Close() error {
	db.remote.Close()
	return nil
}

func dialRPC(endpoint string) (*rpc.Client, error) {
	if endpoint == "" {
		return nil, errors.New("endpoint must be specified")
	}
	if strings.HasPrefix(endpoint, "rpc:") || strings.HasPrefix(endpoint, "ipc:") {
		// Backwards compatibility with geth < 1.5 which required
		// these prefixes.
		endpoint = endpoint[4:]
	}
	return rpc.Dial(endpoint)
}

func New(endpoint string) (database.Database, error) {
	client, err := dialRPC(endpoint)
	if err != nil {
		return nil, err
	}
	return &Database{
		remote: client,
	}, nil
}

func getStringKey(key []byte) string {
	var buf bytes.Buffer
	for b := range key {
		if key[b] > 0x20 && key[b] < 0x7F {
			buf.WriteByte(key[b])
		} else {
			buf.WriteByte(0x7E)
		}
	}
	return buf.String()
}
