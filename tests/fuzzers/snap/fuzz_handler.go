package snap

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/entropy/protocols/snap"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/server/p2p"
	"github.com/entropyio/go-entropy/server/p2p/enode"
	fuzz "github.com/google/gofuzz"
	"math/big"
	"time"
)

var trieRoot common.Hash

func getChain() *blockchain.BlockChain {
	db := rawdb.NewMemoryDatabase()
	ga := make(blockchain.GenesisAlloc, 1000)
	var a = make([]byte, 20)
	var mkStorage = func(k, v int) (common.Hash, common.Hash) {
		var kB = make([]byte, 32)
		var vB = make([]byte, 32)
		binary.LittleEndian.PutUint64(kB, uint64(k))
		binary.LittleEndian.PutUint64(vB, uint64(v))
		return common.BytesToHash(kB), common.BytesToHash(vB)
	}
	storage := make(map[common.Hash]common.Hash)
	for i := 0; i < 10; i++ {
		k, v := mkStorage(i, i)
		storage[k] = v
	}
	for i := 0; i < 1000; i++ {
		binary.LittleEndian.PutUint64(a, uint64(i+0xff))
		acc := blockchain.GenesisAccount{Balance: big.NewInt(int64(i))}
		if i%2 == 1 {
			acc.Storage = storage
		}
		ga[common.BytesToAddress(a)] = acc
	}
	gspec := blockchain.Genesis{
		Config: config.TestChainConfig,
		Alloc:  ga,
	}
	genesis := gspec.MustCommit(db)
	blocks, _ := blockchain.GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, 2,
		func(i int, gen *blockchain.BlockGen) {})
	cacheConf := &blockchain.CacheConfig{
		TrieCleanLimit:      0,
		TrieDirtyLimit:      0,
		TrieTimeLimit:       5 * time.Minute,
		TrieCleanNoPrefetch: true,
		TrieCleanRejournal:  0,
		SnapshotLimit:       100,
		SnapshotWait:        true,
	}
	trieRoot = blocks[len(blocks)-1].Root()
	bc, _ := blockchain.NewBlockChain(db, cacheConf, gspec.Config, ethash.NewFaker(), evm.Config{}, nil, nil)
	if _, err := bc.InsertChain(blocks); err != nil {
		panic(err)
	}
	return bc
}

type dummyBackend struct {
	chain *blockchain.BlockChain
}

func (d *dummyBackend) Chain() *blockchain.BlockChain          { return d.chain }
func (d *dummyBackend) RunPeer(*snap.Peer, snap.Handler) error { return nil }
func (d *dummyBackend) PeerInfo(enode.ID) interface{}          { return "Foo" }
func (d *dummyBackend) Handle(*snap.Peer, snap.Packet) error   { return nil }

type dummyRW struct {
	code       uint64
	data       []byte
	writeCount int
}

func (d *dummyRW) ReadMsg() (p2p.Msg, error) {
	return p2p.Msg{
		Code:       d.code,
		Payload:    bytes.NewReader(d.data),
		ReceivedAt: time.Now(),
		Size:       uint32(len(d.data)),
	}, nil
}

func (d *dummyRW) WriteMsg(p2p.Msg) error {
	d.writeCount++
	return nil
}

func doFuzz(input []byte, obj interface{}, code int) int {
	if len(input) > 1024*4 {
		return -1
	}
	bc := getChain()
	defer bc.Stop()
	backend := &dummyBackend{bc}
	fuzz.NewFromGoFuzz(input).Fuzz(obj)
	var data []byte
	switch p := obj.(type) {
	case *snap.GetTrieNodesPacket:
		p.Root = trieRoot
		data, _ = rlp.EncodeToBytes(obj)
	default:
		data, _ = rlp.EncodeToBytes(obj)
	}
	cli := &dummyRW{
		code: uint64(code),
		data: data,
	}
	peer := snap.NewFakePeer(65, "gazonk01", cli)
	err := snap.HandleMessage(backend, peer)
	switch {
	case err == nil && cli.writeCount != 1:
		panic(fmt.Sprintf("Expected 1 response, got %d", cli.writeCount))
	case err != nil && cli.writeCount != 0:
		panic(fmt.Sprintf("Expected 0 response, got %d", cli.writeCount))
	}
	return 1
}

// To run a fuzzer, do
// $ CGO_ENABLED=0 go-fuzz-build -func FuzzTrieNodes
// $ go-fuzz

func FuzzARange(input []byte) int {
	return doFuzz(input, &snap.GetAccountRangePacket{}, snap.GetAccountRangeMsg)
}
func FuzzSRange(input []byte) int {
	return doFuzz(input, &snap.GetStorageRangesPacket{}, snap.GetStorageRangesMsg)
}
func FuzzByteCodes(input []byte) int {
	return doFuzz(input, &snap.GetByteCodesPacket{}, snap.GetByteCodesMsg)
}
func FuzzTrieNodes(input []byte) int {
	return doFuzz(input, &snap.GetTrieNodesPacket{}, snap.GetTrieNodesMsg)
}
