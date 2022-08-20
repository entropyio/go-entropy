package tracers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/entropy/entropyapi"
	"github.com/entropyio/go-entropy/entropy/tracers/tracelogger"
	"github.com/entropyio/go-entropy/evm"
	"github.com/entropyio/go-entropy/rpc"
	"math/big"
	"reflect"
	"sort"
	"testing"
	"time"
)

var (
	errStateNotFound       = errors.New("state not found")
	errBlockNotFound       = errors.New("block not found")
	errTransactionNotFound = errors.New("transaction not found")
)

type testBackend struct {
	chainConfig *config.ChainConfig
	engine      consensus.Engine
	chaindb     database.Database
	chain       *blockchain.BlockChain
}

func newTestBackend(t *testing.T, n int, gspec *blockchain.Genesis, generator func(i int, b *blockchain.BlockGen)) *testBackend {
	backend := &testBackend{
		chainConfig: config.TestChainConfig,
		engine:      ethash.NewFaker(),
		chaindb:     rawdb.NewMemoryDatabase(),
	}
	// Generate blocks for testing
	gspec.Config = backend.chainConfig
	var (
		gendb   = rawdb.NewMemoryDatabase()
		genesis = gspec.MustCommit(gendb)
	)
	blocks, _ := blockchain.GenerateChain(backend.chainConfig, genesis, backend.engine, gendb, n, generator)

	// Import the canonical chain
	gspec.MustCommit(backend.chaindb)
	cacheConfig := &blockchain.CacheConfig{
		TrieCleanLimit:    256,
		TrieDirtyLimit:    256,
		TrieTimeLimit:     5 * time.Minute,
		SnapshotLimit:     0,
		TrieDirtyDisabled: true, // Archive mode
	}
	chain, err := blockchain.NewBlockChain(backend.chaindb, cacheConfig, backend.chainConfig, backend.engine, evm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	backend.chain = chain
	return backend
}

func (b *testBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*model.Header, error) {
	return b.chain.GetHeaderByHash(hash), nil
}

func (b *testBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Header, error) {
	if number == rpc.PendingBlockNumber || number == rpc.LatestBlockNumber {
		return b.chain.CurrentHeader(), nil
	}
	return b.chain.GetHeaderByNumber(uint64(number)), nil
}

func (b *testBackend) BlockByHash(ctx context.Context, hash common.Hash) (*model.Block, error) {
	return b.chain.GetBlockByHash(hash), nil
}

func (b *testBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Block, error) {
	if number == rpc.PendingBlockNumber || number == rpc.LatestBlockNumber {
		return b.chain.CurrentBlock(), nil
	}
	return b.chain.GetBlockByNumber(uint64(number)), nil
}

func (b *testBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*model.Transaction, common.Hash, uint64, uint64, error) {
	tx, hash, blockNumber, index := rawdb.ReadTransaction(b.chaindb, txHash)
	if tx == nil {
		return nil, common.Hash{}, 0, 0, errTransactionNotFound
	}
	return tx, hash, blockNumber, index, nil
}

func (b *testBackend) RPCGasCap() uint64 {
	return 25000000
}

func (b *testBackend) ChainConfig() *config.ChainConfig {
	return b.chainConfig
}

func (b *testBackend) Engine() consensus.Engine {
	return b.engine
}

func (b *testBackend) ChainDb() database.Database {
	return b.chaindb
}

func (b *testBackend) StateAtBlock(ctx context.Context, block *model.Block, reexec uint64, base *state.StateDB, checkLive bool, preferDisk bool) (*state.StateDB, error) {
	statedb, err := b.chain.StateAt(block.Root())
	if err != nil {
		return nil, errStateNotFound
	}
	return statedb, nil
}

func (b *testBackend) StateAtTransaction(ctx context.Context, block *model.Block, txIndex int, reexec uint64) (blockchain.Message, evm.BlockContext, *state.StateDB, error) {
	parent := b.chain.GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		return nil, evm.BlockContext{}, nil, errBlockNotFound
	}
	statedb, err := b.chain.StateAt(parent.Root())
	if err != nil {
		return nil, evm.BlockContext{}, nil, errStateNotFound
	}
	if txIndex == 0 && len(block.Transactions()) == 0 {
		return nil, evm.BlockContext{}, statedb, nil
	}
	// Recompute transactions up to the target index.
	signer := model.MakeSigner(b.chainConfig, block.Number())
	for idx, tx := range block.Transactions() {
		msg, _ := tx.AsMessage(signer, block.BaseFee())
		txContext := blockchain.NewEVMTxContext(msg)
		context := blockchain.NewEVMBlockContext(block.Header(), b.chain, nil)
		if idx == txIndex {
			return msg, context, statedb, nil
		}
		vmenv := evm.NewEVM(context, txContext, statedb, b.chainConfig, evm.Config{})
		if _, err := blockchain.ApplyMessage(vmenv, msg, new(blockchain.GasPool).AddGas(tx.Gas())); err != nil {
			return nil, evm.BlockContext{}, nil, fmt.Errorf("transaction %#x failed: %v", tx.Hash(), err)
		}
		statedb.Finalise(false)
	}
	return nil, evm.BlockContext{}, nil, fmt.Errorf("transaction index %d out of range for block %#x", txIndex, block.Hash())
}

func TestTraceCall(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(3)
	genesis := &blockchain.Genesis{Alloc: blockchain.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(config.Ether)},
		accounts[1].addr: {Balance: big.NewInt(config.Ether)},
		accounts[2].addr: {Balance: big.NewInt(config.Ether)},
	}}
	genBlocks := 10
	signer := model.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, genBlocks, genesis, func(i int, b *blockchain.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := model.SignTx(model.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), config.TxGas, b.BaseFee(), nil), signer, accounts[0].key)
		b.AddTx(tx)
	}))
	var testSuite = []struct {
		blockNumber rpc.BlockNumber
		call        entropyapi.TransactionArgs
		config      *TraceCallConfig
		expectErr   error
		expect      string
	}{
		// Standard JSON trace upon the genesis, plain transfer.
		{
			blockNumber: rpc.BlockNumber(0),
			call: entropyapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: nil,
			expect:    `{"gas":21000,"failed":false,"returnValue":"","structLogs":[]}`,
		},
		// Standard JSON trace upon the head, plain transfer.
		{
			blockNumber: rpc.BlockNumber(genBlocks),
			call: entropyapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: nil,
			expect:    `{"gas":21000,"failed":false,"returnValue":"","structLogs":[]}`,
		},
		// Standard JSON trace upon the non-existent block, error expects
		{
			blockNumber: rpc.BlockNumber(genBlocks + 1),
			call: entropyapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: fmt.Errorf("block #%d not found", genBlocks+1),
			//expect:    nil,
		},
		// Standard JSON trace upon the latest block
		{
			blockNumber: rpc.LatestBlockNumber,
			call: entropyapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: nil,
			expect:    `{"gas":21000,"failed":false,"returnValue":"","structLogs":[]}`,
		},
		// Tracing on 'pending' should fail:
		{
			blockNumber: rpc.PendingBlockNumber,
			call: entropyapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: errors.New("tracing on top of pending is not supported"),
		},
		{
			blockNumber: rpc.LatestBlockNumber,
			call: entropyapi.TransactionArgs{
				From:  &accounts[0].addr,
				Input: &hexutil.Bytes{0x43}, // blocknumber
			},
			config: &TraceCallConfig{
				BlockOverrides: &entropyapi.BlockOverrides{Number: (*hexutil.Big)(big.NewInt(0x1337))},
			},
			expectErr: nil,
			expect: ` {"gas":53018,"failed":false,"returnValue":"","structLogs":[
		{"pc":0,"op":"NUMBER","gas":24946984,"gasCost":2,"depth":1,"stack":[]},
		{"pc":1,"op":"STOP","gas":24946982,"gasCost":0,"depth":1,"stack":["0x1337"]}]}`,
		},
	}
	for i, testspec := range testSuite {
		result, err := api.TraceCall(context.Background(), testspec.call, rpc.BlockNumberOrHash{BlockNumber: &testspec.blockNumber}, testspec.config)
		if testspec.expectErr != nil {
			if err == nil {
				t.Errorf("test %d: expect error %v, got nothing", i, testspec.expectErr)
				continue
			}
			if !reflect.DeepEqual(err, testspec.expectErr) {
				t.Errorf("test %d: error mismatch, want %v, git %v", i, testspec.expectErr, err)
			}
		} else {
			if err != nil {
				t.Errorf("test %d: expect no error, got %v", i, err)
				continue
			}
			var have *tracelogger.ExecutionResult
			if err := json.Unmarshal(result.(json.RawMessage), &have); err != nil {
				t.Errorf("test %d: failed to unmarshal result %v", i, err)
			}
			var want *tracelogger.ExecutionResult
			if err := json.Unmarshal([]byte(testspec.expect), &want); err != nil {
				t.Errorf("test %d: failed to unmarshal result %v", i, err)
			}
			if !reflect.DeepEqual(have, want) {
				t.Errorf("test %d: result mismatch, want %v, got %v", i, testspec.expect, string(result.(json.RawMessage)))
			}
		}
	}
}

func TestTraceTransaction(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(2)
	genesis := &blockchain.Genesis{Alloc: blockchain.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(config.Ether)},
		accounts[1].addr: {Balance: big.NewInt(config.Ether)},
	}}
	target := common.Hash{}
	signer := model.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, 1, genesis, func(i int, b *blockchain.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := model.SignTx(model.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), config.TxGas, b.BaseFee(), nil), signer, accounts[0].key)
		b.AddTx(tx)
		target = tx.Hash()
	}))
	result, err := api.TraceTransaction(context.Background(), target, nil)
	if err != nil {
		t.Errorf("Failed to trace transaction %v", err)
	}
	var have *tracelogger.ExecutionResult
	if err := json.Unmarshal(result.(json.RawMessage), &have); err != nil {
		t.Errorf("failed to unmarshal result %v", err)
	}
	if !reflect.DeepEqual(have, &tracelogger.ExecutionResult{
		Gas:         config.TxGas,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  []tracelogger.StructLogRes{},
	}) {
		t.Error("Transaction tracing result is different")
	}
}

func TestTraceBlock(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(3)
	genesis := &blockchain.Genesis{Alloc: blockchain.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(config.Ether)},
		accounts[1].addr: {Balance: big.NewInt(config.Ether)},
		accounts[2].addr: {Balance: big.NewInt(config.Ether)},
	}}
	genBlocks := 10
	signer := model.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, genBlocks, genesis, func(i int, b *blockchain.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := model.SignTx(model.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), config.TxGas, b.BaseFee(), nil), signer, accounts[0].key)
		b.AddTx(tx)
	}))

	var testSuite = []struct {
		blockNumber rpc.BlockNumber
		config      *TraceConfig
		want        string
		expectErr   error
	}{
		// Trace genesis block, expect error
		{
			blockNumber: rpc.BlockNumber(0),
			expectErr:   errors.New("genesis is not traceable"),
		},
		// Trace head block
		{
			blockNumber: rpc.BlockNumber(genBlocks),
			want:        `[{"result":{"gas":21000,"failed":false,"returnValue":"","structLogs":[]}}]`,
		},
		// Trace non-existent block
		{
			blockNumber: rpc.BlockNumber(genBlocks + 1),
			expectErr:   fmt.Errorf("block #%d not found", genBlocks+1),
		},
		// Trace latest block
		{
			blockNumber: rpc.LatestBlockNumber,
			want:        `[{"result":{"gas":21000,"failed":false,"returnValue":"","structLogs":[]}}]`,
		},
		// Trace pending block
		{
			blockNumber: rpc.PendingBlockNumber,
			want:        `[{"result":{"gas":21000,"failed":false,"returnValue":"","structLogs":[]}}]`,
		},
	}
	for i, tc := range testSuite {
		result, err := api.TraceBlockByNumber(context.Background(), tc.blockNumber, tc.config)
		if tc.expectErr != nil {
			if err == nil {
				t.Errorf("test %d, want error %v", i, tc.expectErr)
				continue
			}
			if !reflect.DeepEqual(err, tc.expectErr) {
				t.Errorf("test %d: error mismatch, want %v, get %v", i, tc.expectErr, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("test %d, want no error, have %v", i, err)
			continue
		}
		have, _ := json.Marshal(result)
		want := tc.want
		if string(have) != want {
			t.Errorf("test %d, result mismatch, have\n%v\n, want\n%v\n", i, string(have), want)
		}
	}
}

func TestTracingWithOverrides(t *testing.T) {
	t.Parallel()
	// Initialize test accounts
	accounts := newAccounts(3)
	genesis := &blockchain.Genesis{Alloc: blockchain.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(config.Ether)},
		accounts[1].addr: {Balance: big.NewInt(config.Ether)},
		accounts[2].addr: {Balance: big.NewInt(config.Ether)},
	}}
	genBlocks := 10
	signer := model.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, genBlocks, genesis, func(i int, b *blockchain.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := model.SignTx(model.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), config.TxGas, b.BaseFee(), nil), signer, accounts[0].key)
		b.AddTx(tx)
	}))
	randomAccounts := newAccounts(3)
	type res struct {
		Gas         int
		Failed      bool
		ReturnValue string
	}
	var testSuite = []struct {
		blockNumber rpc.BlockNumber
		call        entropyapi.TransactionArgs
		config      *TraceCallConfig
		expectErr   error
		want        string
	}{
		// Call which can only succeed if state is state overridden
		{
			blockNumber: rpc.LatestBlockNumber,
			call: entropyapi.TransactionArgs{
				From:  &randomAccounts[0].addr,
				To:    &randomAccounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config: &TraceCallConfig{
				StateOverrides: &entropyapi.StateOverride{
					randomAccounts[0].addr: entropyapi.OverrideAccount{Balance: newRPCBalance(new(big.Int).Mul(big.NewInt(1), big.NewInt(config.Ether)))},
				},
			},
			want: `{"gas":21000,"failed":false,"returnValue":""}`,
		},
		// Invalid call without state overriding
		{
			blockNumber: rpc.LatestBlockNumber,
			call: entropyapi.TransactionArgs{
				From:  &randomAccounts[0].addr,
				To:    &randomAccounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    &TraceCallConfig{},
			expectErr: blockchain.ErrInsufficientFunds,
		},
		// Successful simple contract call
		//
		// // SPDX-License-Identifier: GPL-3.0
		//
		//  pragma solidity >=0.7.0 <0.8.0;
		//
		//  /**
		//   * @title Storage
		//   * @dev Store & retrieve value in a variable
		//   */
		//  contract Storage {
		//      uint256 public number;
		//      constructor() {
		//          number = block.number;
		//      }
		//  }
		{
			blockNumber: rpc.LatestBlockNumber,
			call: entropyapi.TransactionArgs{
				From: &randomAccounts[0].addr,
				To:   &randomAccounts[2].addr,
				Data: newRPCBytes(common.Hex2Bytes("8381f58a")), // call number()
			},
			config: &TraceCallConfig{
				//Tracer: &tracer,
				StateOverrides: &entropyapi.StateOverride{
					randomAccounts[2].addr: entropyapi.OverrideAccount{
						Code:      newRPCBytes(common.Hex2Bytes("6080604052348015600f57600080fd5b506004361060285760003560e01c80638381f58a14602d575b600080fd5b60336049565b6040518082815260200191505060405180910390f35b6000548156fea2646970667358221220eab35ffa6ab2adfe380772a48b8ba78e82a1b820a18fcb6f59aa4efb20a5f60064736f6c63430007040033")),
						StateDiff: newStates([]common.Hash{{}}, []common.Hash{common.BigToHash(big.NewInt(123))}),
					},
				},
			},
			want: `{"gas":23347,"failed":false,"returnValue":"000000000000000000000000000000000000000000000000000000000000007b"}`,
		},
		{ // Override blocknumber
			blockNumber: rpc.LatestBlockNumber,
			call: entropyapi.TransactionArgs{
				From: &accounts[0].addr,
				// BLOCKNUMBER PUSH1 MSTORE
				Input: newRPCBytes(common.Hex2Bytes("4360005260206000f3")),
				//&hexutil.Bytes{0x43}, // blocknumber
			},
			config: &TraceCallConfig{
				BlockOverrides: &entropyapi.BlockOverrides{Number: (*hexutil.Big)(big.NewInt(0x1337))},
			},
			want: `{"gas":59537,"failed":false,"returnValue":"0000000000000000000000000000000000000000000000000000000000001337"}`,
		},
		{ // Override blocknumber, and query a blockhash
			blockNumber: rpc.LatestBlockNumber,
			call: entropyapi.TransactionArgs{
				From: &accounts[0].addr,
				Input: &hexutil.Bytes{
					0x60, 0x00, 0x40, // BLOCKHASH(0)
					0x60, 0x00, 0x52, // STORE memory offset 0
					0x61, 0x13, 0x36, 0x40, // BLOCKHASH(0x1336)
					0x60, 0x20, 0x52, // STORE memory offset 32
					0x61, 0x13, 0x37, 0x40, // BLOCKHASH(0x1337)
					0x60, 0x40, 0x52, // STORE memory offset 64
					0x60, 0x60, 0x60, 0x00, 0xf3, // RETURN (0-96)

				}, // blocknumber
			},
			config: &TraceCallConfig{
				BlockOverrides: &entropyapi.BlockOverrides{Number: (*hexutil.Big)(big.NewInt(0x1337))},
			},
			want: `{"gas":72666,"failed":false,"returnValue":"000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"}`,
		},
	}
	for i, tc := range testSuite {
		result, err := api.TraceCall(context.Background(), tc.call, rpc.BlockNumberOrHash{BlockNumber: &tc.blockNumber}, tc.config)
		if tc.expectErr != nil {
			if err == nil {
				t.Errorf("test %d: want error %v, have nothing", i, tc.expectErr)
				continue
			}
			if !errors.Is(err, tc.expectErr) {
				t.Errorf("test %d: error mismatch, want %v, have %v", i, tc.expectErr, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("test %d: want no error, have %v", i, err)
			continue
		}
		// Turn result into res-struct
		var (
			have res
			want res
		)
		resBytes, _ := json.Marshal(result)
		_ = json.Unmarshal(resBytes, &have)
		_ = json.Unmarshal([]byte(tc.want), &want)
		if !reflect.DeepEqual(have, want) {
			t.Errorf("test %d, result mismatch, have\n%v\n, want\n%v\n", i, string(resBytes), want)
		}
	}
}

type Account struct {
	key  *ecdsa.PrivateKey
	addr common.Address
}

type Accounts []Account

func (a Accounts) Len() int           { return len(a) }
func (a Accounts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Accounts) Less(i, j int) bool { return bytes.Compare(a[i].addr.Bytes(), a[j].addr.Bytes()) < 0 }

func newAccounts(n int) (accounts Accounts) {
	for i := 0; i < n; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)
		accounts = append(accounts, Account{key: key, addr: addr})
	}
	sort.Sort(accounts)
	return accounts
}

func newRPCBalance(balance *big.Int) **hexutil.Big {
	rpcBalance := (*hexutil.Big)(balance)
	return &rpcBalance
}

func newRPCBytes(bytes []byte) *hexutil.Bytes {
	rpcBytes := hexutil.Bytes(bytes)
	return &rpcBytes
}

func newStates(keys []common.Hash, vals []common.Hash) *map[common.Hash]common.Hash {
	if len(keys) != len(vals) {
		panic("invalid input")
	}
	m := make(map[common.Hash]common.Hash)
	for i := 0; i < len(keys); i++ {
		m[keys[i]] = vals[i]
	}
	return &m
}
