package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/mathutil"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/database"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/database/trie"
	"math/big"
	"strings"
)

//go:generate go run github.com/fjl/gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate go run github.com/fjl/gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config     *config.ChainConfig `json:"config"`
	Nonce      uint64              `json:"nonce"`
	Timestamp  uint64              `json:"timestamp"`
	ExtraData  []byte              `json:"extraData"`
	GasLimit   uint64              `json:"gasLimit"   gencodec:"required"`
	Difficulty *big.Int            `json:"difficulty" gencodec:"required"`
	Mixhash    common.Hash         `json:"mixHash"`
	Coinbase   common.Address      `json:"coinbase"`
	Alloc      GenesisAlloc        `json:"alloc"      gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
	BaseFee    *big.Int    `json:"baseFeePerGas"`
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

// flush adds allocated genesis accounts into a fresh new statedb and
// commit the state changes into the given database handler.
func (ga *GenesisAlloc) flush(db database.Database) (common.Hash, error) {
	stateDB, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	if err != nil {
		return common.Hash{}, err
	}
	for addr, account := range *ga {
		stateDB.AddBalance(addr, account.Balance)
		stateDB.SetCode(addr, account.Code)
		stateDB.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			stateDB.SetState(addr, key, value)
		}
	}
	root, err := stateDB.Commit(false)
	if err != nil {
		return common.Hash{}, err
	}
	err = stateDB.Database().TrieDB().Commit(root, true, nil)
	if err != nil {
		return common.Hash{}, err
	}
	return root, nil
}

// write writes the json marshaled genesis state into database
// with the given block hash as the unique identifier.
func (ga *GenesisAlloc) write(db database.KeyValueWriter, hash common.Hash) error {
	blob, err := json.Marshal(ga)
	if err != nil {
		return err
	}
	rawdb.WriteGenesisState(db, hash, blob)
	return nil
}

// CommitGenesisState loads the stored genesis state with the given block
// hash and commits them into the given database handler.
func CommitGenesisState(db database.Database, hash common.Hash) error {
	var alloc = new(GenesisAlloc)
	blob := rawdb.ReadGenesisState(db, hash)
	if len(blob) != 0 {
		if err := alloc.UnmarshalJSON(blob); err != nil {
			return err
		}
	} else {
		// Genesis allocation is missing and there are several possibilities:
		// the node is legacy which doesn't persist the genesis allocation or
		// the persisted allocation is just lost.
		// - supported networks(mainnet, testnets), recover with defined allocations
		// - private network, can't recover
		var genesis *Genesis
		switch hash {
		case config.MainnetGenesisHash:
			genesis = DefaultGenesisBlock()
		case config.TestnetGenesisHash:
			genesis = DefaultTestnetGenesisBlock()
		}
		if genesis != nil {
			alloc = &genesis.Alloc
		} else {
			return errors.New("not found")
		}
	}
	_, err := alloc.flush(db)
	return err
}

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Nonce      mathutil.HexOrDecimal64
	Timestamp  mathutil.HexOrDecimal64
	ExtraData  hexutil.Bytes
	GasLimit   mathutil.HexOrDecimal64
	GasUsed    mathutil.HexOrDecimal64
	Number     mathutil.HexOrDecimal64
	Difficulty *mathutil.HexOrDecimal256
	BaseFee    *mathutil.HexOrDecimal256
	Alloc      map[common.UnprefixedAddress]GenesisAccount
}

type genesisAccountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *mathutil.HexOrDecimal256
	Nonce      mathutil.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

// storageJSON represents a 256 bit byte array, but allows less than 256 bits when
// unmarshaling from hex.
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}
	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		fmt.Println(err)
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// GenesisMismatchError is raised when trying to overwrite an existing
// genesis block with an incompatible one.
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database contains incompatible genesis (have %x, new %x)", e.Stored, e.New)
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *config.CompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db database.Database, genesis *Genesis) (*config.ChainConfig, common.Hash, error) {
	return SetupGenesisBlockWithOverride(db, genesis, nil)
}

func SetupGenesisBlockWithOverride(db database.Database, genesis *Genesis, overrideTerminalTotalDifficulty *big.Int) (*config.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return config.DefaultChainConfig, common.Hash{}, errGenesisNoConfig
	}
	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			glog.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			glog.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		if err != nil {
			return genesis.Config, common.Hash{}, err
		}
		return genesis.Config, block.Hash(), nil
	}

	// We have the genesis block in database(perhaps in ancient database)
	// but the corresponding state is missing.
	header := rawdb.ReadHeader(db, stored, 0)
	if _, err := state.New(header.Root, state.NewDatabaseWithConfig(db, nil), nil); err != nil {
		if genesis == nil {
			genesis = DefaultGenesisBlock()
		}
		// Ensure the stored genesis matches with the given one.
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
		block, err := genesis.Commit(db)
		if err != nil {
			return genesis.Config, hash, err
		}
		return genesis.Config, block.Hash(), nil
	}
	// Check whether the genesis block is already written.
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	} else {
		genesis = DefaultGenesisBlock()
	}
	// Get the existing chain configuration.
	newConfig := genesis.configOrDefault(stored)
	if overrideTerminalTotalDifficulty != nil {
		newConfig.TerminalTotalDifficulty = overrideTerminalTotalDifficulty
	}
	if err := newConfig.CheckConfigForkOrder(); err != nil {
		return newConfig, common.Hash{}, err
	}
	storedConfig := rawdb.ReadChainConfig(db, stored)
	if storedConfig == nil {
		glog.Warning("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newConfig)
		return newConfig, stored, nil
	}
	// Special case: if a private network is being used (no genesis and also no
	// mainnet hash in the database), we must not apply the `configOrDefault`
	// chain config as that would be AllProtocolChanges (applying any new fork
	// on top of an existing private network genesis block). In that case, only
	// apply the overrides.
	if genesis == nil && stored != config.MainnetGenesisHash {
		newConfig = storedConfig
		if overrideTerminalTotalDifficulty != nil {
			newConfig.TerminalTotalDifficulty = overrideTerminalTotalDifficulty
		}
	}
	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return newConfig, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedConfig.CheckCompatible(newConfig, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return newConfig, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newConfig)
	return newConfig, stored, nil
}

func (genesis *Genesis) configOrDefault(ghash common.Hash) *config.ChainConfig {
	switch {
	case genesis != nil:
		return genesis.Config
	case ghash == config.MainnetGenesisHash:
		return config.MainnetChainConfig
	case ghash == config.TestnetGenesisHash:
		return config.TestnetChainConfig
	default:
		return config.DefaultChainConfig
	}
}

// ToBlock creates the genesis block and writes state of a genesis specification
// to the given database (or discards it if nil).
func (genesis *Genesis) ToBlock(db database.Database) *model.Block {
	if db == nil {
		db = rawdb.NewMemoryDatabase()
	}
	root, err := genesis.Alloc.flush(db)
	if err != nil {
		panic(err)
	}

	// add claudeContext
	// claudeContext := initGenesisClaudeContext(g, trie.NewDatabase(db))
	// claudeContextHash := claudeContext.ToHash()

	head := &model.Header{
		Number:     new(big.Int).SetUint64(genesis.Number),
		Nonce:      model.EncodeNonce(genesis.Nonce),
		Time:       genesis.Timestamp,
		ParentHash: genesis.ParentHash,
		Extra:      genesis.ExtraData,
		GasLimit:   genesis.GasLimit,
		GasUsed:    genesis.GasUsed,
		BaseFee:    genesis.BaseFee,
		Difficulty: genesis.Difficulty,
		MixDigest:  genesis.Mixhash,
		Coinbase:   genesis.Coinbase,
		Root:       root,
		// ClaudeCtxHash: claudeContextHash,
	}
	if genesis.GasLimit == 0 {
		head.GasLimit = config.GenesisGasLimit
	}
	if genesis.Difficulty == nil && genesis.Mixhash == (common.Hash{}) {
		head.Difficulty = config.GenesisDifficulty
	}
	block := model.NewBlock(head, nil, nil, nil, trie.NewStackTrie(nil))

	glog.Warningf("ToBlock create genesis. number:%d, hash:%x, root:%x", block.NumberU64(), block.Hash(), block.Root())
	return block
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (genesis *Genesis) Commit(db database.Database) (*model.Block, error) {
	glog.Warningf("Start Commit genesis block with DB: %+v", db)

	block := genesis.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, errors.New("can't commit genesis block with number > 0")
	}

	configObj := genesis.Config
	if configObj == nil {
		configObj = config.DefaultChainConfig
	}
	if err := configObj.CheckConfigForkOrder(); err != nil {
		return nil, err
	}
	if configObj.Clique != nil && len(block.Extra()) < 32+crypto.SignatureLength {
		return nil, errors.New("can't start clique chain without signers")
	}
	if err := genesis.Alloc.write(db, block.Hash()); err != nil {
		return nil, err
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), block.Difficulty())
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadFastBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())
	rawdb.WriteChainConfig(db, block.Hash(), configObj)

	glog.Warningf("End Commit genesis block with hash: %x", block.Hash())
	return block, nil
}

// MustCommit writes the genesis block and state to db, panicking on error.
// The block is committed as the canonical head block.
func (genesis *Genesis) MustCommit(db database.Database) *model.Block {
	block, err := genesis.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}

// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
func GenesisBlockForTesting(db database.Database, addr common.Address, balance *big.Int) *model.Block {
	g := Genesis{
		Alloc:   GenesisAlloc{addr: {Balance: balance}},
		BaseFee: big.NewInt(config.InitialBaseFee),
	}
	return g.MustCommit(db)
}

// DefaultGenesisBlock returns the Entropy main net genesis block.
func DefaultGenesisBlock() *Genesis {
	return &Genesis{
		Config:     config.MainnetChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x11bbe8db4e347b4e8c937c1c8370e4b5ed33adb3db69cbdb7a38e1e50b1b82fa"),
		GasLimit:   5000,
		Difficulty: big.NewInt(17179869184),
		Alloc:      decodePrealloc(mainNetAllocData),
	}
}

// DefaultTestnetGenesisBlock returns the Ropsten network genesis block.
func DefaultTestnetGenesisBlock() *Genesis {
	return &Genesis{
		Config:     config.TestnetChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		GasLimit:   16777216,
		Difficulty: big.NewInt(1048576),
		Alloc:      decodePrealloc(testNetAllocData),
	}
}

// DeveloperGenesisBlock returns the 'entropy --dev' genesis block.
func DeveloperGenesisBlock(period uint64, gasLimit uint64, faucet common.Address) *Genesis {
	// Override the default period to the user requested one
	configObj := *config.DefaultChainConfig
	configObj.Clique = &config.CliqueConfig{
		Period: period,
		Epoch:  configObj.Clique.Epoch,
	}

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	return &Genesis{
		Config:     &configObj,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, crypto.SignatureLength)...),
		GasLimit:   gasLimit,
		BaseFee:    big.NewInt(config.InitialBaseFee),
		Difficulty: big.NewInt(1),
		Alloc: map[common.Address]GenesisAccount{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			common.BytesToAddress([]byte{9}): {Balance: big.NewInt(1)}, // BLAKE2b
			faucet:                           {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
}

func decodePrealloc(data string) GenesisAlloc {
	var p []struct{ Addr, Balance *big.Int }
	if err := rlp.NewStream(strings.NewReader(data), 0).Decode(&p); err != nil {
		panic(err)
	}
	ga := make(GenesisAlloc, len(p))
	for _, account := range p {
		ga[common.BigToAddress(account.Addr)] = GenesisAccount{Balance: account.Balance}
	}
	return ga
}

/**
func initGenesisClaudeContext(g *Genesis, db *trie.TrieDatabase) *model.ClaudeContext {
	dc, err := model.NewClaudeContextFromHash(*db, &model.ClaudeContextHash{})
	if err != nil {
		return nil
	}
	if g.Config != nil && g.Config.Claude != nil && g.Config.Claude.Validators != nil {
		dc.SetValidators(g.Config.Claude.Validators)
		for _, validator := range g.Config.Claude.Validators {
			dc.DelegateTrie().TryUpdate(append(validator.Bytes(), validator.Bytes()...), validator.Bytes())
			dc.CandidateTrie().TryUpdate(validator.Bytes(), validator.Bytes())
		}
	}
	return dc
}
*/
