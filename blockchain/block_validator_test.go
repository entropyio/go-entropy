package blockchain

import (
	"encoding/json"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/consensus/beacon"
	"github.com/entropyio/go-entropy/consensus/clique"
	"github.com/entropyio/go-entropy/consensus/ethash"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/evm"
	"math/big"
	"runtime"
	"testing"
	"time"
)

// Tests that simple header verification works, for both good and bad blocks.
func TestHeaderVerification(t *testing.T) {
	// Create a simple chain to verify
	var (
		testdb    = rawdb.NewMemoryDatabase()
		gspec     = &Genesis{Config: config.TestChainConfig}
		genesis   = gspec.MustCommit(testdb)
		blocks, _ = GenerateChain(config.TestChainConfig, genesis, ethash.NewFaker(), testdb, 8, nil)
	)
	headers := make([]*model.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	// Run the header checker for blocks one-by-one, checking for both valid and invalid nonces
	chain, _ := NewBlockChain(testdb, nil, config.TestChainConfig, ethash.NewFaker(), evm.Config{}, nil, nil)
	defer chain.Stop()

	for i := 0; i < len(blocks); i++ {
		for j, valid := range []bool{true, false} {
			var results <-chan error

			if valid {
				engine := ethash.NewFaker()
				_, results = engine.VerifyHeaders(chain, []*model.Header{headers[i]}, []bool{true})
			} else {
				engine := ethash.NewFakeFailer(headers[i].Number.Uint64())
				_, results = engine.VerifyHeaders(chain, []*model.Header{headers[i]}, []bool{true})
			}
			// Wait for the verification result
			select {
			case result := <-results:
				if (result == nil) != valid {
					t.Errorf("test %d.%d: validity mismatch: have %v, want %v", i, j, result, valid)
				}
			case <-time.After(time.Second):
				t.Fatalf("test %d.%d: verification timeout", i, j)
			}
			// Make sure no more data is returned
			select {
			case result := <-results:
				t.Fatalf("test %d.%d: unexpected result returned: %v", i, j, result)
			case <-time.After(25 * time.Millisecond):
			}
		}
		_, _ = chain.InsertChain(blocks[i : i+1])
	}
}

func TestHeaderVerificationForMergingClique(t *testing.T) { testHeaderVerificationForMerging(t, true) }
func TestHeaderVerificationForMergingEthash(t *testing.T) { testHeaderVerificationForMerging(t, false) }

// Tests the verification for eth1/2 merging, including pre-merge and post-merge
func testHeaderVerificationForMerging(t *testing.T, isClique bool) {
	var (
		testdb      = rawdb.NewMemoryDatabase()
		preBlocks   []*model.Block
		postBlocks  []*model.Block
		runEngine   consensus.Engine
		chainConfig *config.ChainConfig
		merger      = consensus.NewMerger(rawdb.NewMemoryDatabase())
	)
	if isClique {
		var (
			key, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
			addr   = crypto.PubkeyToAddress(key.PublicKey)
			engine = clique.New(config.TestChainConfig.Clique, testdb)
		)
		genspec := &Genesis{
			ExtraData: make([]byte, 32+common.AddressLength+crypto.SignatureLength),
			Alloc: map[common.Address]GenesisAccount{
				addr: {Balance: big.NewInt(1)},
			},
			BaseFee: big.NewInt(config.InitialBaseFee),
		}
		copy(genspec.ExtraData[32:], addr[:])
		genesis := genspec.MustCommit(testdb)

		genEngine := beacon.New(engine)
		preBlocks, _ = GenerateChain(config.TestChainConfig, genesis, genEngine, testdb, 8, nil)
		td := 0
		for i, block := range preBlocks {
			header := block.Header()
			if i > 0 {
				header.ParentHash = preBlocks[i-1].Hash()
			}
			header.Extra = make([]byte, 32+crypto.SignatureLength)
			header.Difficulty = big.NewInt(2)

			sig, _ := crypto.Sign(genEngine.SealHash(header).Bytes(), key)
			copy(header.Extra[len(header.Extra)-crypto.SignatureLength:], sig)
			preBlocks[i] = block.WithSeal(header)
			// calculate td
			td += int(block.Difficulty().Uint64())
		}
		configObj := *config.TestChainConfig
		configObj.TerminalTotalDifficulty = big.NewInt(int64(td))
		postBlocks, _ = GenerateChain(&configObj, preBlocks[len(preBlocks)-1], genEngine, testdb, 8, nil)
		chainConfig = &configObj
		runEngine = beacon.New(engine)
	} else {
		gspec := &Genesis{Config: config.TestChainConfig}
		genesis := gspec.MustCommit(testdb)
		genEngine := beacon.New(ethash.NewFaker())

		preBlocks, _ = GenerateChain(config.TestChainConfig, genesis, genEngine, testdb, 8, nil)
		td := 0
		for _, block := range preBlocks {
			// calculate td
			td += int(block.Difficulty().Uint64())
		}
		configObj := *config.TestChainConfig
		configObj.TerminalTotalDifficulty = big.NewInt(int64(td))
		postBlocks, _ = GenerateChain(config.TestChainConfig, preBlocks[len(preBlocks)-1], genEngine, testdb, 8, nil)

		chainConfig = &configObj
		runEngine = beacon.New(ethash.NewFaker())
	}

	preHeaders := make([]*model.Header, len(preBlocks))
	for i, block := range preBlocks {
		preHeaders[i] = block.Header()

		blob, _ := json.Marshal(block.Header())
		t.Logf("Log header before the merging %d: %v", block.NumberU64(), string(blob))
	}
	postHeaders := make([]*model.Header, len(postBlocks))
	for i, block := range postBlocks {
		postHeaders[i] = block.Header()

		blob, _ := json.Marshal(block.Header())
		t.Logf("Log header after the merging %d: %v", block.NumberU64(), string(blob))
	}
	// Run the header checker for blocks one-by-one, checking for both valid and invalid nonces
	chain, _ := NewBlockChain(testdb, nil, chainConfig, runEngine, evm.Config{}, nil, nil)
	defer chain.Stop()

	// Verify the blocks before the merging
	for i := 0; i < len(preBlocks); i++ {
		_, results := runEngine.VerifyHeaders(chain, []*model.Header{preHeaders[i]}, []bool{true})
		// Wait for the verification result
		select {
		case result := <-results:
			if result != nil {
				t.Errorf("test %d: verification failed %v", i, result)
			}
		case <-time.After(time.Second):
			t.Fatalf("test %d: verification timeout", i)
		}
		// Make sure no more data is returned
		select {
		case result := <-results:
			t.Fatalf("test %d: unexpected result returned: %v", i, result)
		case <-time.After(25 * time.Millisecond):
		}
		_, _ = chain.InsertChain(preBlocks[i : i+1])
	}

	// Make the transition
	merger.ReachTTD()
	merger.FinalizePoS()

	// Verify the blocks after the merging
	for i := 0; i < len(postBlocks); i++ {
		_, results := runEngine.VerifyHeaders(chain, []*model.Header{postHeaders[i]}, []bool{true})
		// Wait for the verification result
		select {
		case result := <-results:
			if result != nil {
				t.Errorf("test %d: verification failed %v", i, result)
			}
		case <-time.After(time.Second):
			t.Fatalf("test %d: verification timeout", i)
		}
		// Make sure no more data is returned
		select {
		case result := <-results:
			t.Fatalf("test %d: unexpected result returned: %v", i, result)
		case <-time.After(25 * time.Millisecond):
		}
		_ = chain.InsertBlockWithoutSetHead(postBlocks[i])
	}

	// Verify the blocks with pre-merge blocks and post-merge blocks
	var (
		headers []*model.Header
		seals   []bool
	)
	for _, block := range preBlocks {
		headers = append(headers, block.Header())
		seals = append(seals, true)
	}
	for _, block := range postBlocks {
		headers = append(headers, block.Header())
		seals = append(seals, true)
	}
	_, results := runEngine.VerifyHeaders(chain, headers, seals)
	for i := 0; i < len(headers); i++ {
		select {
		case result := <-results:
			if result != nil {
				t.Errorf("test %d: verification failed %v", i, result)
			}
		case <-time.After(time.Second):
			t.Fatalf("test %d: verification timeout", i)
		}
	}
	// Make sure no more data is returned
	select {
	case result := <-results:
		t.Fatalf("unexpected result returned: %v", result)
	case <-time.After(25 * time.Millisecond):
	}
}

// Tests that concurrent header verification works, for both good and bad blocks.
func TestHeaderConcurrentVerification2(t *testing.T)  { testHeaderConcurrentVerification(t, 2) }
func TestHeaderConcurrentVerification8(t *testing.T)  { testHeaderConcurrentVerification(t, 8) }
func TestHeaderConcurrentVerification32(t *testing.T) { testHeaderConcurrentVerification(t, 32) }

func testHeaderConcurrentVerification(t *testing.T, threads int) {
	// Create a simple chain to verify
	var (
		testdb    = rawdb.NewMemoryDatabase()
		gspec     = &Genesis{Config: config.TestChainConfig}
		genesis   = gspec.MustCommit(testdb)
		blocks, _ = GenerateChain(config.TestChainConfig, genesis, ethash.NewFaker(), testdb, 8, nil)
	)
	headers := make([]*model.Header, len(blocks))
	seals := make([]bool, len(blocks))

	for i, block := range blocks {
		headers[i] = block.Header()
		seals[i] = true
	}
	// Set the number of threads to verify on
	old := runtime.GOMAXPROCS(threads)
	defer runtime.GOMAXPROCS(old)

	// Run the header checker for the entire block chain at once both for a valid and
	// also an invalid chain (enough if one arbitrary block is invalid).
	for i, valid := range []bool{true, false} {
		var results <-chan error

		if valid {
			chain, _ := NewBlockChain(testdb, nil, config.TestChainConfig, ethash.NewFaker(), evm.Config{}, nil, nil)
			_, results = chain.engine.VerifyHeaders(chain, headers, seals)
			chain.Stop()
		} else {
			chain, _ := NewBlockChain(testdb, nil, config.TestChainConfig, ethash.NewFakeFailer(uint64(len(headers)-1)), evm.Config{}, nil, nil)
			_, results = chain.engine.VerifyHeaders(chain, headers, seals)
			chain.Stop()
		}
		// Wait for all the verification results
		checks := make(map[int]error)
		for j := 0; j < len(blocks); j++ {
			select {
			case result := <-results:
				checks[j] = result

			case <-time.After(time.Second):
				t.Fatalf("test %d.%d: verification timeout", i, j)
			}
		}
		// Check nonce check validity
		for j := 0; j < len(blocks); j++ {
			want := valid || (j < len(blocks)-2) // We chose the last-but-one nonce in the chain to fail
			if (checks[j] == nil) != want {
				t.Errorf("test %d.%d: validity mismatch: have %v, want %v", i, j, checks[j], want)
			}
			if !want {
				// A few blocks after the first error may pass verification due to concurrent
				// workers. We don't care about those in this test, just that the correct block
				// errors out.
				break
			}
		}
		// Make sure no more data is returned
		select {
		case result := <-results:
			t.Fatalf("test %d: unexpected result returned: %v", i, result)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// Tests that aborting a header validation indeed prevents further checks from being
// run, as well as checks that no left-over goroutines are leaked.
func TestHeaderConcurrentAbortion2(t *testing.T)  { testHeaderConcurrentAbortion(t, 2) }
func TestHeaderConcurrentAbortion8(t *testing.T)  { testHeaderConcurrentAbortion(t, 8) }
func TestHeaderConcurrentAbortion32(t *testing.T) { testHeaderConcurrentAbortion(t, 32) }

func testHeaderConcurrentAbortion(t *testing.T, threads int) {
	// Create a simple chain to verify
	var (
		testdb    = rawdb.NewMemoryDatabase()
		gspec     = &Genesis{Config: config.TestChainConfig}
		genesis   = gspec.MustCommit(testdb)
		blocks, _ = GenerateChain(config.TestChainConfig, genesis, ethash.NewFaker(), testdb, 1024, nil)
	)
	headers := make([]*model.Header, len(blocks))
	seals := make([]bool, len(blocks))

	for i, block := range blocks {
		headers[i] = block.Header()
		seals[i] = true
	}
	// Set the number of threads to verify on
	old := runtime.GOMAXPROCS(threads)
	defer runtime.GOMAXPROCS(old)

	// Start the verifications and immediately abort
	chain, _ := NewBlockChain(testdb, nil, config.TestChainConfig, ethash.NewFakeDelayer(time.Millisecond), evm.Config{}, nil, nil)
	defer chain.Stop()

	abort, results := chain.engine.VerifyHeaders(chain, headers, seals)
	close(abort)

	// Deplete the results channel
	verified := 0
	for depleted := false; !depleted; {
		select {
		case result := <-results:
			if result != nil {
				t.Errorf("header %d: validation failed: %v", verified, result)
			}
			verified++
		case <-time.After(50 * time.Millisecond):
			depleted = true
		}
	}
	// Check that abortion was honored by not processing too many POWs
	if verified > 2*threads {
		t.Errorf("verification count too large: have %d, want below %d", verified, 2*threads)
	}
}

func TestCalcGasLimit(t *testing.T) {
	for i, tc := range []struct {
		pGasLimit uint64
		max       uint64
		min       uint64
	}{
		{20000000, 20019530, 19980470},
		{40000000, 40039061, 39960939},
	} {
		// Increase
		if have, want := CalcGasLimit(tc.pGasLimit, 2*tc.pGasLimit), tc.max; have != want {
			t.Errorf("test %d: have %d want <%d", i, have, want)
		}
		// Decrease
		if have, want := CalcGasLimit(tc.pGasLimit, 0), tc.min; have != want {
			t.Errorf("test %d: have %d want >%d", i, have, want)
		}
		// Small decrease
		if have, want := CalcGasLimit(tc.pGasLimit, tc.pGasLimit-1), tc.pGasLimit-1; have != want {
			t.Errorf("test %d: have %d want %d", i, have, want)
		}
		// Small increase
		if have, want := CalcGasLimit(tc.pGasLimit, tc.pGasLimit+1), tc.pGasLimit+1; have != want {
			t.Errorf("test %d: have %d want %d", i, have, want)
		}
		// No change
		if have, want := CalcGasLimit(tc.pGasLimit, tc.pGasLimit), tc.pGasLimit; have != want {
			t.Errorf("test %d: have %d want %d", i, have, want)
		}
	}
}
