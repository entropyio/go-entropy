// This file contains a miner stress test based on the Ethash consensus engine.
package main

import (
	"crypto/ecdsa"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/fdlimit"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/entropy/entropyconfig"
	"github.com/entropyio/go-entropy/miner"
	"github.com/entropyio/go-entropy/server/node"
	"github.com/entropyio/go-entropy/server/p2p"
	"github.com/entropyio/go-entropy/server/p2p/enode"
	"math/big"
	"math/rand"
	"os"
	"os/signal"
	"time"
)

func main() {
	fdlimit.Raise(2048)

	// Generate a batch of accounts to seal and fund with
	faucets := make([]*ecdsa.PrivateKey, 128)
	for i := 0; i < len(faucets); i++ {
		faucets[i], _ = crypto.GenerateKey()
	}
	// Pre-generate the ethash mining DAG so we don't race
	ethash.MakeDataset(1, entropyconfig.Defaults.Ethash.DatasetDir)

	// Create an Ethash network based off of the Ropsten config
	genesis := makeGenesis(faucets)

	// Handle interrupts.
	interruptCh := make(chan os.Signal, 5)
	signal.Notify(interruptCh, os.Interrupt)

	var (
		stacks []*node.Node
		nodes  []*entropy.Entropy
		enodes []*enode.Node
	)
	for i := 0; i < 4; i++ {
		// Start the node and wait until it's up
		stack, ethBackend, err := makeMiner(genesis)
		if err != nil {
			panic(err)
		}
		defer stack.Close()

		for stack.Server().NodeInfo().Ports.Listener == 0 {
			time.Sleep(250 * time.Millisecond)
		}
		// Connect the node to all the previous ones
		for _, n := range enodes {
			stack.Server().AddPeer(n)
		}
		// Start tracking the node and its enode
		stacks = append(stacks, stack)
		nodes = append(nodes, ethBackend)
		enodes = append(enodes, stack.Server().Self())
	}

	// Iterate over all the nodes and start mining
	time.Sleep(3 * time.Second)
	for _, node := range nodes {
		if err := node.StartMining(1); err != nil {
			panic(err)
		}
	}
	time.Sleep(3 * time.Second)

	// Start injecting transactions from the faucets like crazy
	nonces := make([]uint64, len(faucets))
	for {
		// Stop when interrupted.
		select {
		case <-interruptCh:
			for _, node := range stacks {
				node.Close()
			}
			return
		default:
		}

		// Pick a random mining node
		index := rand.Intn(len(faucets))
		backend := nodes[index%len(nodes)]

		// Create a self transaction and inject into the pool
		tx, err := model.SignTx(model.NewTransaction(nonces[index], crypto.PubkeyToAddress(faucets[index].PublicKey), new(big.Int), 21000, big.NewInt(100000000000+rand.Int63n(65536)), nil), model.HomesteadSigner{}, faucets[index])
		if err != nil {
			panic(err)
		}
		if err := backend.TxPool().AddLocal(tx); err != nil {
			panic(err)
		}
		nonces[index]++

		// Wait if we're too saturated
		if pend, _ := backend.TxPool().Stats(); pend > 2048 {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// makeGenesis creates a custom Ethash genesis block based on some pre-defined
// faucet accounts.
func makeGenesis(faucets []*ecdsa.PrivateKey) *blockchain.Genesis {
	genesis := blockchain.DefaultGenesisBlock()
	genesis.Difficulty = config.MinimumDifficulty
	genesis.GasLimit = 25000000

	genesis.Config.ChainID = big.NewInt(18)

	genesis.Alloc = blockchain.GenesisAlloc{}
	for _, faucet := range faucets {
		genesis.Alloc[crypto.PubkeyToAddress(faucet.PublicKey)] = blockchain.GenesisAccount{
			Balance: new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil),
		}
	}
	return genesis
}

func makeMiner(genesis *blockchain.Genesis) (*node.Node, *entropy.Entropy, error) {
	// Define the basic configurations for the Entropy node
	datadir, _ := os.MkdirTemp("", "")

	configObj := &node.Config{
		Name:    "entropy",
		Version: config.Version,
		DataDir: datadir,
		P2P: p2p.Config{
			ListenAddr:  "0.0.0.0:0",
			NoDiscovery: true,
			MaxPeers:    25,
		},
		UseLightweightKDF: true,
	}
	// Create the node and configure a full Entropy node on it
	stack, err := node.New(configObj)
	if err != nil {
		return nil, nil, err
	}
	ethBackend, err := entropy.New(stack, &entropyconfig.Config{
		Genesis:         genesis,
		NetworkId:       genesis.Config.ChainID.Uint64(),
		SyncMode:        downloader.FullSync,
		DatabaseCache:   256,
		DatabaseHandles: 256,
		TxPool:          blockchain.DefaultTxPoolConfig,
		GPO:             entropyconfig.Defaults.GPO,
		Ethash:          entropyconfig.Defaults.Ethash,
		Miner: miner.Config{
			EntropyBase: common.Address{1},
			GasCeil:     genesis.GasLimit * 11 / 10,
			GasPrice:    big.NewInt(1),
			Recommit:    time.Second,
		},
	})
	if err != nil {
		return nil, nil, err
	}

	err = stack.Start()
	return stack, ethBackend, err
}
