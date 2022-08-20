package entropyio_test

import (
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"strings"
	"testing"
)

func TestLevelDB(t *testing.T) {
	dbPath := "/Users/eric/Desktop/docker/go-entropy/EntropyNet/entropy/chaindata"
	//dbPath := "/Users/eric/Desktop/docker/go-entropy/EntropyNet/entropy/lightchaindata"
	keyNums := 100

	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		fmt.Println("ERROR: Database can not be opened!")
		return
	}

	iter := db.NewIterator(nil, nil)
	key := ""
	for i := 0; i < keyNums; i++ {
		iter.Next()
		key = string(iter.Key())
		if key == "" {
			return
		}
		key = strings.ReplaceAll(key, "\r", "")
		key = strings.ReplaceAll(key, "\r", "")
		fmt.Printf("index:%d || key:%s || value:%+v\n", i, key, iter.Value())
	}
}
