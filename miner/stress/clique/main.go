// This file contains a miner stress test based on the Clique consensus engine.
package main

import (
	"bytes"
	"crypto/ecdsa"
	"github.com/entropyio/go-entropy/accounts/keystore"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/fdlimit"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/entropy/entropyconfig"
	"github.com/entropyio/go-entropy/logger"
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

var log = logger.NewLogger("[clique]")

func main() {
	fdlimit.Raise(2048)

	// Generate a batch of accounts to seal and fund with
	faucets := make([]*ecdsa.PrivateKey, 128)
	for i := 0; i < len(faucets); i++ {
		faucets[i], _ = crypto.GenerateKey()
	}
	sealers := make([]*ecdsa.PrivateKey, 4)
	for i := 0; i < len(sealers); i++ {
		sealers[i], _ = crypto.GenerateKey()
	}
	// Create a Clique network based off of the Rinkeby config
	genesis := makeGenesis(faucets, sealers)

	// Handle interrupts.
	interruptCh := make(chan os.Signal, 5)
	signal.Notify(interruptCh, os.Interrupt)

	var (
		stacks []*node.Node
		nodes  []*entropy.Entropy
		enodes []*enode.Node
	)
	for _, sealer := range sealers {
		// Start the node and wait until it's up
		stack, ethBackend, err := makeSealer(genesis)
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

		// Inject the signer key and start sealing with it
		ks := keystore.NewKeyStore(stack.KeyStoreDir(), keystore.LightScryptN, keystore.LightScryptP)
		signer, err := ks.ImportECDSA(sealer, "")
		if err != nil {
			panic(err)
		}
		if err := ks.Unlock(signer, ""); err != nil {
			panic(err)
		}
		stack.AccountManager().AddBackend(ks)
	}

	// Iterate over all the nodes and start signing on them
	time.Sleep(3 * time.Second)
	for _, node := range nodes {
		if err := node.StartMining(1); err != nil {
			panic(err)
		}
	}
	time.Sleep(3 * time.Second)

	// Start injecting transactions from the faucet like crazy
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

		// Pick a random signer node
		index := rand.Intn(len(faucets))
		backend := nodes[index%len(nodes)]

		// Create a self transaction and inject into the pool
		tx, err := model.SignTx(model.NewTransaction(nonces[index], crypto.PubkeyToAddress(faucets[index].PublicKey), new(big.Int), 21000, big.NewInt(100000000000), nil), model.HomesteadSigner{}, faucets[index])
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

// makeGenesis creates a custom Clique genesis block based on some pre-defined
// signer and faucet accounts.
func makeGenesis(faucets []*ecdsa.PrivateKey, sealers []*ecdsa.PrivateKey) *blockchain.Genesis {
	// Create a Clique network based off of the Rinkeby config
	genesis := blockchain.DefaultGenesisBlock()
	genesis.GasLimit = 25000000

	genesis.Config.ChainID = big.NewInt(18)
	genesis.Config.Clique.Period = 1

	genesis.Alloc = blockchain.GenesisAlloc{}
	for _, faucet := range faucets {
		genesis.Alloc[crypto.PubkeyToAddress(faucet.PublicKey)] = blockchain.GenesisAccount{
			Balance: new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil),
		}
	}
	// Sort the signers and embed into the extra-data section
	signers := make([]common.Address, len(sealers))
	for i, sealer := range sealers {
		signers[i] = crypto.PubkeyToAddress(sealer.PublicKey)
	}
	for i := 0; i < len(signers); i++ {
		for j := i + 1; j < len(signers); j++ {
			if bytes.Compare(signers[i][:], signers[j][:]) > 0 {
				signers[i], signers[j] = signers[j], signers[i]
			}
		}
	}
	genesis.ExtraData = make([]byte, 32+len(signers)*common.AddressLength+65)
	for i, signer := range signers {
		copy(genesis.ExtraData[32+i*common.AddressLength:], signer[:])
	}
	// Return the genesis block for initialization
	return genesis
}

func makeSealer(genesis *blockchain.Genesis) (*node.Node, *entropy.Entropy, error) {
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
	}
	// Start the node and configure a full Entropy node on it
	stack, err := node.New(configObj)
	if err != nil {
		return nil, nil, err
	}
	// Create and register the backend
	ethBackend, err := entropy.New(stack, &entropyconfig.Config{
		Genesis:         genesis,
		NetworkId:       genesis.Config.ChainID.Uint64(),
		SyncMode:        downloader.FullSync,
		DatabaseCache:   256,
		DatabaseHandles: 256,
		TxPool:          blockchain.DefaultTxPoolConfig,
		GPO:             entropyconfig.Defaults.GPO,
		Miner: miner.Config{
			GasCeil:  genesis.GasLimit * 11 / 10,
			GasPrice: big.NewInt(1),
			Recommit: time.Second,
		},
	})
	if err != nil {
		return nil, nil, err
	}

	err = stack.Start()
	return stack, ethBackend, err
}
