package miner

import (
	"math/big"
	"testing"
	"time"

	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/genesis"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/consensus/claude"
	"github.com/entropyio/go-entropy/consensus/clique"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/evm"
)

var (
	// Test chain configurations
	testTxPoolConfig  blockchain.TxPoolConfig
	ethashChainConfig *config.ChainConfig
	cliqueChainConfig *config.ChainConfig
	claudeChainConfig *config.ChainConfig

	// Test accounts
	testBankKey, _  = crypto.GenerateKey()
	testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
	testBankFunds   = big.NewInt(1000000000000000000)

	acc1Key, _ = crypto.GenerateKey()
	acc1Addr   = crypto.PubkeyToAddress(acc1Key.PublicKey)

	// Test transactions
	pendingTxs []*model.Transaction
	newTxs     []*model.Transaction
)

func init() {
	testTxPoolConfig = blockchain.DefaultTxPoolConfig
	testTxPoolConfig.Journal = ""

	ethashChainConfig = config.TestChainConfig
	cliqueChainConfig = config.TestChainConfig
	claudeChainConfig = config.TestChainConfig

	cliqueChainConfig.Clique = &config.CliqueConfig{
		Period: 10,
		Epoch:  30000,
	}

	validators := []common.Address{
		common.HexToAddress("0x44d1ce0b7cb3588bca96151fe1bc05af38f91b6e"),
		common.HexToAddress("0xa60a3886b552ff9992cfcd208ec1152079e046c2"),
		common.HexToAddress("0x4e080e49f62694554871e669aeb4ebe17c4a9670"),
	}
	claudeChainConfig.Claude = &config.ClaudeConfig{
		Validators: validators,
	}

	tx1, _ := model.SignTx(model.NewTransaction(0, acc1Addr, big.NewInt(1000), config.TxGas, nil, nil), model.HomesteadSigner{}, testBankKey)
	pendingTxs = append(pendingTxs, tx1)
	tx2, _ := model.SignTx(model.NewTransaction(1, acc1Addr, big.NewInt(1000), config.TxGas, nil, nil), model.HomesteadSigner{}, testBankKey)
	newTxs = append(newTxs, tx2)
}

// testWorkerBackend implements worker.Backend interfaces and wraps all information needed during the testing.
type testWorkerBackend struct {
	db         database.Database
	txPool     *blockchain.TxPool
	chain      *blockchain.BlockChain
	testTxFeed event.Feed
	uncleBlock *model.Block
}

func newTestWorkerBackend(t *testing.T, chainConfig *config.ChainConfig, engine consensus.Engine) *testWorkerBackend {
	var (
		db    = database.NewMemDatabase()
		gspec = genesis.Genesis{
			Config: chainConfig,
			Alloc:  genesis.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
		}
	)

	switch engine.(type) {
	case *clique.Clique:
		gspec.ExtraData = make([]byte, 32+common.AddressLength+65)
		copy(gspec.ExtraData[32:], testBankAddress[:])
	case *ethash.Ethash:
	case *claude.Claude:
	default:
		t.Fatal("unexpect consensus engine type")
	}
	genesisObj := gspec.MustCommit(db)

	chain, _ := blockchain.NewBlockChain(db, nil, gspec.Config, engine, evm.Config{})
	txpool := blockchain.NewTxPool(testTxPoolConfig, chainConfig, chain)
	blocks, _ := blockchain.GenerateChain(chainConfig, genesisObj, engine, db, 1, func(i int, gen *blockchain.BlockGen) {
		gen.SetCoinbase(acc1Addr)
	})

	return &testWorkerBackend{
		db:         db,
		chain:      chain,
		txPool:     txpool,
		uncleBlock: blocks[0],
	}
}

func (b *testWorkerBackend) BlockChain() *blockchain.BlockChain { return b.chain }
func (b *testWorkerBackend) TxPool() *blockchain.TxPool         { return b.txPool }
func (b *testWorkerBackend) PostChainEvents(events []interface{}) {
	b.chain.PostChainEvents(events, nil)
}

func newTestWorker(t *testing.T, chainConfig *config.ChainConfig, engine consensus.Engine) (*worker, *testWorkerBackend) {
	backend := newTestWorkerBackend(t, chainConfig, engine)
	backend.txPool.AddLocals(pendingTxs)
	w := newWorker(chainConfig, engine, backend, new(event.TypeMux))
	w.setEntropyBase(testBankAddress)
	return w, backend
}

func TestPendingStateAndBlockEthash(t *testing.T) {
	testPendingStateAndBlock(t, ethashChainConfig, ethash.NewFaker())
}

func TestPendingStateAndBlockClique(t *testing.T) {
	testPendingStateAndBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, database.NewMemDatabase()))
}

func testPendingStateAndBlock(t *testing.T, chainConfig *config.ChainConfig, engine consensus.Engine) {
	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine)
	defer w.close()

	// Ensure snapshot has been updated.
	time.Sleep(100 * time.Millisecond)
	block, state := w.pending()
	if block.NumberU64() != 1 {
		t.Errorf("block number mismatch, has %d, want %d", block.NumberU64(), 1)
	}
	if balance := state.GetBalance(acc1Addr); balance.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("account balance mismatch, has %d, want %d", balance, 1000)
	}
	b.txPool.AddLocals(newTxs)
	// Ensure the new tx events has been processed
	time.Sleep(100 * time.Millisecond)
	block, state = w.pending()
	if balance := state.GetBalance(acc1Addr); balance.Cmp(big.NewInt(2000)) != 0 {
		t.Errorf("account balance mismatch, has %d, want %d", balance, 2000)
	}
}

func TestEmptyWorkEthash(t *testing.T) {
	testEmptyWork(t, ethashChainConfig, ethash.NewFaker())
}

func TestEmptyWorkClique(t *testing.T) {
	testEmptyWork(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, database.NewMemDatabase()))
}

func TestEmptyWorkClaude(t *testing.T) {
	testEmptyWork(t, claudeChainConfig, claude.New(cliqueChainConfig.Claude, database.NewMemDatabase()))
}

func testEmptyWork(t *testing.T, chainConfig *config.ChainConfig, engine consensus.Engine) {
	defer engine.Close()

	w, _ := newTestWorker(t, chainConfig, engine)
	defer w.close()

	var (
		taskCh    = make(chan struct{}, 2)
		taskIndex int
	)

	checkEqual := func(t *testing.T, task *task, index int) {
		receiptLen, balance := 0, big.NewInt(0)
		if index == 1 {
			receiptLen, balance = 1, big.NewInt(1000)
		}
		if len(task.receipts) != receiptLen {
			t.Errorf("receipt number mismatch has %d, want %d", len(task.receipts), receiptLen)
		}
		if task.state.GetBalance(acc1Addr).Cmp(balance) != 0 {
			t.Errorf("account balance mismatch has %d, want %d", task.state.GetBalance(acc1Addr), balance)
		}
	}

	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 1 {
			checkEqual(t, task, taskIndex)
			taskIndex += 1
			taskCh <- struct{}{}
		}
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}

	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()
	for i := 0; i < 2; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}
}

func TestStreamUncleBlock(t *testing.T) {
	consensus := ethash.NewFaker()
	defer consensus.Close()

	w, b := newTestWorker(t, ethashChainConfig, consensus)
	defer w.close()

	var taskCh = make(chan struct{})

	taskIndex := 0
	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 1 {
			if taskIndex == 2 {
				has := task.block.Header().UncleHash
				want := model.CalcUncleHash([]*model.Header{b.uncleBlock.Header()})
				if has != want {
					t.Errorf("uncle hash mismatch, has %s, want %s", has.Hex(), want.Hex())
				}
			}
			taskCh <- struct{}{}
			taskIndex += 1
		}
	}
	w.skipSealHook = func(task *task) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}

	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()
	// Ignore the first two works
	for i := 0; i < 2; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}
	b.PostChainEvents([]interface{}{blockchain.ChainSideEvent{Block: b.uncleBlock}})

	select {
	case <-taskCh:
	case <-time.NewTimer(time.Second).C:
		t.Error("new task timeout")
	}
}
