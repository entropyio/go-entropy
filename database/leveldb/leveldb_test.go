package leveldb

import (
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/dbtest"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"testing"
)

func TestLevelDB(t *testing.T) {
	t.Run("DatabaseSuite", func(t *testing.T) {
		dbtest.TestDatabaseSuite(t, func() database.KeyValueStore {
			db, err := leveldb.Open(storage.NewMemStorage(), nil)
			if err != nil {
				t.Fatal(err)
			}
			return &Database{
				db: db,
			}
		})
	})
}
