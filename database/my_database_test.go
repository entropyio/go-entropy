package database_test

import (
	"fmt"
	"testing"

	"github.com/entropyio/go-entropy/database"
	"os"
)

func newTestLDBMy() (*database.LDBDatabase, func()) {
	dirname := "/Users/wangzhen/Desktop/blockchain/test_entropy"
	fmt.Println("dirname: ", dirname)
	db, err := database.NewLDBDatabase(dirname, 0, 0)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}

	return db, func() {
		db.Close()
		os.RemoveAll(dirname)
	}
}

func TestLDB_PutGetMy(t *testing.T) {
	db, remove := newTestLDBMy()
	defer remove()

	db.Put([]byte("abcd1"), []byte("abcd1"))
	fmt.Println("put abcd1: abcd1")

	db.Put([]byte("12345"), []byte("12345"))
	fmt.Println("put 12345: 12345")

	data, _ := db.Get([]byte("12345"))
	fmt.Println("get 12345: ", data)

	db.Put([]byte("12345"), []byte("54321"))
	fmt.Println("put 12345: 54321")

	data, _ = db.Get([]byte("12345"))
	fmt.Println("get 12345: ", data)

	err := db.Delete([]byte("12345"))
	if err != nil {
		t.Fatalf("delete %q failed: %v", []byte("12345"), err)
	}
	fmt.Println("delete 12345: ", err)

	data, err = db.Get([]byte("12345"))
	if err != nil {
		fmt.Println("get 12345 failed: ", err)
	}
	fmt.Println("get 12345: ", data)
}

func TestMemoryDB_PutGetMy(t *testing.T) {
	db := database.NewMemDatabaseWithCap(10)

	db.Put([]byte("abcd1"), []byte("abcd1"))
	fmt.Println("put abcd1: abcd1")

	db.Put([]byte("12345"), []byte("12345"))
	fmt.Println("put 12345: 12345")

	data, _ := db.Get([]byte("12345"))
	fmt.Println("get 12345: ", data)

	db.Put([]byte("12345"), []byte("54321"))
	fmt.Println("put 12345: 54321")

	data, _ = db.Get([]byte("12345"))
	fmt.Println("get 12345: ", data)

	err := db.Delete([]byte("12345"))
	if err != nil {
		t.Fatalf("delete %q failed: %v", []byte("12345"), err)
	}
	fmt.Println("delete 12345: ", err)

	data, err = db.Get([]byte("12345"))
	if err != nil {
		fmt.Println("get 12345 failed: ", err)
	}
	fmt.Println("get 12345: ", data)
}
