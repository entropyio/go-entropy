package entropy

import (
	"github.com/entropyio/go-entropy/account"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/server/node"
	"reflect"
	"testing"
	"time"
)

var (
	key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	addr1   = crypto.PubkeyToAddress(key1.PublicKey)
)

func TestMinerStart(t *testing.T) {
	nodeConfig := &node.Config{
		Name:    "miner-test",
		DataDir: "/Users/wangzhen/Desktop/blockchain/GoEntropy/data",
	}

	ctx := &node.ServiceContext{
		Config:         nodeConfig,
		Services:       make(map[reflect.Type]node.Service),
		EventMux:       new(event.TypeMux),
		AccountManager: new(account.Manager),
	}

	config := &Config{}

	entropy, _ := New(ctx, config)
	entropy.SetEntropyBase(addr1)

	log.Warningf("entropy backend StartMining...")
	entropy.StartMining(true)
	time.Sleep(6000 * time.Second)

	entropy.StopMining()
	log.Warningf("entropy backend StopMining")
}
