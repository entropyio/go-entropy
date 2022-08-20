package memorydb

import (
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/dbtest"
	"testing"
)

func TestMemoryDB(t *testing.T) {
	t.Run("DatabaseSuite", func(t *testing.T) {
		dbtest.TestDatabaseSuite(t, func() database.KeyValueStore {
			return New()
		})
	})
}
