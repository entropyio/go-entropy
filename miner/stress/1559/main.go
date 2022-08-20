// This file contains a miner stress test for eip 1559.
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

var (
	londonBlock = big.NewInt(30) // Predefined london fork block for activating eip 1559.
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
	var (
		nonces = make([]uint64, len(faucets))

		// The signer activates the 1559 features even before the fork,
		// so the new 1559 txs can be created with this signer.
		signer = model.LatestSignerForChainID(genesis.Config.ChainID)
	)
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

		headHeader := backend.BlockChain().CurrentHeader()
		baseFee := headHeader.BaseFee

		// Create a self transaction and inject into the pool. The legacy
		// and 1559 transactions can all be created by random even if the
		// fork is not happened.
		tx := makeTransaction(nonces[index], faucets[index], signer, baseFee)
		if err := backend.TxPool().AddLocal(tx); err != nil {
			continue
		}
		nonces[index]++

		// Wait if we're too saturated
		if pend, _ := backend.TxPool().Stats(); pend > 4192 {
			time.Sleep(100 * time.Millisecond)
		}

		// Wait if the basefee is raised too fast
		if baseFee != nil && baseFee.Cmp(new(big.Int).Mul(big.NewInt(100), big.NewInt(config.GWei))) > 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func makeTransaction(nonce uint64, privKey *ecdsa.PrivateKey, signer model.Signer, baseFee *big.Int) *model.Transaction {
	// Generate legacy transaction
	if rand.Intn(2) == 0 {
		tx, err := model.SignTx(model.NewTransaction(nonce, crypto.PubkeyToAddress(privKey.PublicKey), new(big.Int), 21000, big.NewInt(100000000000+rand.Int63n(65536)), nil), signer, privKey)
		if err != nil {
			panic(err)
		}
		return tx
	}
	// Generate eip 1559 transaction
	recipient := crypto.PubkeyToAddress(privKey.PublicKey)

	// Feecap and feetip are limited to 32 bytes. Offer a sightly
	// larger buffer for creating both valid and invalid transactions.
	var buf = make([]byte, 32+5)
	rand.Read(buf)
	gasTipCap := new(big.Int).SetBytes(buf)

	// If the given base fee is nil(the 1559 is still not available),
	// generate a fake base fee in order to create 1559 tx forcibly.
	if baseFee == nil {
		baseFee = new(big.Int).SetInt64(int64(rand.Int31()))
	}
	// Generate the feecap, 75% valid feecap and 25% unguaranted.
	var gasFeeCap *big.Int
	if rand.Intn(4) == 0 {
		rand.Read(buf)
		gasFeeCap = new(big.Int).SetBytes(buf)
	} else {
		gasFeeCap = new(big.Int).Add(baseFee, gasTipCap)
	}
	return model.MustSignNewTx(privKey, signer, &model.DynamicFeeTx{
		ChainID:    signer.ChainID(),
		Nonce:      nonce,
		GasTipCap:  gasTipCap,
		GasFeeCap:  gasFeeCap,
		Gas:        21000,
		To:         &recipient,
		Value:      big.NewInt(100),
		Data:       nil,
		AccessList: nil,
	})
}

// makeGenesis creates a custom Ethash genesis block based on some pre-defined
// faucet accounts.
func makeGenesis(faucets []*ecdsa.PrivateKey) *blockchain.Genesis {
	genesis := blockchain.DefaultGenesisBlock()

	genesis.Config = config.EthashChainConfig
	genesis.Config.LondonBlock = londonBlock
	genesis.Difficulty = config.MinimumDifficulty

	// Small gaslimit for easier basefee moving testing.
	genesis.GasLimit = 8_000_000

	genesis.Config.ChainID = big.NewInt(18)

	genesis.Alloc = blockchain.GenesisAlloc{}
	for _, faucet := range faucets {
		genesis.Alloc[crypto.PubkeyToAddress(faucet.PublicKey)] = blockchain.GenesisAccount{
			Balance: new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil),
		}
	}
	if londonBlock.Sign() == 0 {
		//log.Info("Enabled the eip 1559 by default")
	} else {
		//log.Info("Registered the london fork", "number", londonBlock)
	}
	return genesis
}

func makeMiner(genesis *blockchain.Genesis) (*node.Node, *entropy.Entropy, error) {
	// Define the basic configurations for the Entropy node
	datadir, _ := os.MkdirTemp("", "")

	config := &node.Config{
		Name:    "geth",
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
	stack, err := node.New(config)
	if err != nil {
		return nil, nil, err
	}
	ethBackend, err := entropy.New(stack, &entropy.Config{
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
