package config

import (
	"fmt"
	"math/big"

	"encoding/binary"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
)

const (
	ConsensusTypeEthash = 1
	ConsensusTypeClique = 2
	ConsensusTypeClaude = 3
)

var (
	// EthashChainConfig is the chain parameters to run a node on the pow network.
	EthashChainConfig = &ChainConfig{
		ChainID: big.NewInt(1),

		HomesteadBlock:      big.NewInt(1150000),
		EIP150Block:         big.NewInt(2463000),
		EIP150Hash:          common.HexToHash("0x2086799aeebeae135c246c65021c82b4e15a2c451340993aacfd2751886514f0"),
		EIP155Block:         big.NewInt(2675000),
		EIP158Block:         big.NewInt(2675000),
		ByzantiumBlock:      big.NewInt(4370000),
		ConstantinopleBlock: big.NewInt(7280000),
		PetersburgBlock:     big.NewInt(7280000),

		Ethash:        new(EthashConfig),
		Clique:        &CliqueConfig{Period: 15, Epoch: 30000},
		Claude:        new(ClaudeConfig),
		ConsensusType: ConsensusTypeEthash,
	}

	// CliqueChainConfig contains the chain parameters to run a node on the pos network.
	CliqueChainConfig = &ChainConfig{
		ChainID: big.NewInt(2),

		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d"),
		EIP155Block:         big.NewInt(10),
		EIP158Block:         big.NewInt(10),
		ByzantiumBlock:      big.NewInt(1700000),
		ConstantinopleBlock: big.NewInt(4230000),
		PetersburgBlock:     big.NewInt(4939394),

		Ethash:        new(EthashConfig),
		Clique:        &CliqueConfig{Period: 15, Epoch: 30000},
		Claude:        new(ClaudeConfig),
		ConsensusType: ConsensusTypeClique,
	}

	// ClaudeChainConfig contains the chain parameters to run a node on the dpos network.
	ClaudeChainConfig = &ChainConfig{
		ChainID: big.NewInt(4),

		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d"),
		EIP155Block:         big.NewInt(10),
		EIP158Block:         big.NewInt(10),
		ByzantiumBlock:      big.NewInt(1700000),
		ConstantinopleBlock: big.NewInt(4230000),
		PetersburgBlock:     big.NewInt(4939394),

		Ethash:        new(EthashConfig),
		Clique:        &CliqueConfig{Period: 15, Epoch: 30000},
		Claude:        new(ClaudeConfig),
		ConsensusType: ConsensusTypeClaude,
	}
)

var (
	// default config
	DefaultChainConfig = EthashChainConfig

	MainnetChainConfig = EthashChainConfig
	TestnetChainConfig = CliqueChainConfig

	// for local test config - change it to test each consensus
	TestChainConfig = DefaultChainConfig
)

// TrustedCheckpoints associates each known checkpoint with the genesis hash of
// the chain it belongs to.
var TrustedCheckpoints = map[common.Hash]*TrustedCheckpoint{
	MainnetGenesisHash: MainnetTrustedCheckpoint,
	TestnetGenesisHash: TestnetTrustedCheckpoint,
}

// CheckpointOracles associates each known checkpoint oracles with the genesis hash of
// the chain it belongs to.
var CheckpointOracles = map[common.Hash]*CheckpointOracleConfig{
	MainnetGenesisHash: MainnetCheckpointOracle,
	TestnetGenesisHash: TestnetCheckpointOracle,
}

var (
	MainnetGenesisHash = common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")
	TestnetGenesisHash = common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d")

	// MainnetTrustedCheckpoint contains the light client trusted checkpoint for the main network.
	MainnetTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 246,
		SectionHead:  common.HexToHash("0xb86fbe8a2b1f3c576d06fe1721cd976f98ac1cbf1823da16ef74811e85fd44ac"),
		CHTRoot:      common.HexToHash("0xe99b397f908a391d0d6bd41d1c19cea4bf5051a9695c94d58de44c538d7a1037"),
		BloomRoot:    common.HexToHash("0xa1c1e064ccc16690c5fbabf600c4c7ebb2d8e8fcc674e59365087a77fb391a47"),
	}

	// MainnetCheckpointOracle contains a set of configs for the main network oracle.
	MainnetCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0x9a9070028361F7AAbeB3f2F2Dc07F82C4a98A02a"),
		Signers: []common.Address{
			common.HexToAddress("0x1b2C260efc720BE89101890E4Db589b44E950527"), // Peter
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), // Martin
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), // Zsolt
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), // Gary
			common.HexToAddress("0x0DF8fa387C602AE62559cC4aFa4972A7045d6707"), // Guillaume
		},
		Threshold: 2,
	}

	// TestnetTrustedCheckpoint contains the light client trusted checkpoint for the Ropsten test network.
	TestnetTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 180,
		SectionHead:  common.HexToHash("0xc5741683f9fcff7b670732deef2ebe6e7ff7a7bb29249401300b13b4eee690a6"),
		CHTRoot:      common.HexToHash("0xf03ccebbf71a0998833afdf0e7c29095138c2df1cee6ed44ad9da62b5206b8ad"),
		BloomRoot:    common.HexToHash("0xec46c48cf218891c91ad1139d3b3aec7cf385d4c1100c06711e56b83d8993b23"),
	}

	// TestnetCheckpointOracle contains a set of configs for the Ropsten test network oracle.
	TestnetCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0xEF79475013f154E6A65b54cB2742867791bf0B84"),
		Signers: []common.Address{
			common.HexToAddress("0x32162F3581E88a5f62e8A61892B42C46E2c18f7b"), // Peter
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), // Martin
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), // Zsolt
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), // Gary
			common.HexToAddress("0x0DF8fa387C602AE62559cC4aFa4972A7045d6707"), // Guillaume
		},
		Threshold: 2,
	}
)

// TrustedCheckpoint represents a set of post-processed trie roots (CHT and
// BloomTrie) associated with the appropriate section index and head hash. It is
// used to start light syncing from this checkpoint and avoid downloading the
// entire header chain while still being able to securely access old headers/logs.
type TrustedCheckpoint struct {
	SectionIndex uint64      `json:"sectionIndex"`
	SectionHead  common.Hash `json:"sectionHead"`
	CHTRoot      common.Hash `json:"chtRoot"`
	BloomRoot    common.Hash `json:"bloomRoot"`
}

// HashEqual returns an indicator comparing the itself hash with given one.
func (c *TrustedCheckpoint) HashEqual(hash common.Hash) bool {
	if c.Empty() {
		return hash == common.Hash{}
	}
	return c.Hash() == hash
}

// Hash returns the hash of checkpoint's four key fields(index, sectionHead, chtRoot and bloomTrieRoot).
func (c *TrustedCheckpoint) Hash() common.Hash {
	buf := make([]byte, 8+3*common.HashLength)
	binary.BigEndian.PutUint64(buf, c.SectionIndex)
	copy(buf[8:], c.SectionHead.Bytes())
	copy(buf[8+common.HashLength:], c.CHTRoot.Bytes())
	copy(buf[8+2*common.HashLength:], c.BloomRoot.Bytes())
	return crypto.Keccak256Hash(buf)
}

// Empty returns an indicator whether the checkpoint is regarded as empty.
func (c *TrustedCheckpoint) Empty() bool {
	return c.SectionHead == (common.Hash{}) || c.CHTRoot == (common.Hash{}) || c.BloomRoot == (common.Hash{})
}

// CheckpointOracleConfig represents a set of checkpoint contract(which acts as an oracle)
// config which used for light client checkpoint syncing.
type CheckpointOracleConfig struct {
	Address   common.Address   `json:"address"`
	Signers   []common.Address `json:"signers"`
	Threshold uint64           `json:"threshold"`
}

// ChainConfig is the core config which determines the blockchain settings.
//
// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` // Homestead switch block (nil = no fork, 0 = already homestead)
	// EIP150 implements the Gas price changes (https://github.com/ethereum/EIPs/issues/150)
	EIP150Block *big.Int    `json:"eip150Block,omitempty"` // EIP150 HF block (nil = no fork)
	EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`  // EIP150 HF hash (needed for header only clients as only gas pricing changed)

	EIP155Block *big.Int `json:"eip155Block,omitempty"` // EIP155 HF block
	EIP158Block *big.Int `json:"eip158Block,omitempty"` // EIP158 HF block

	ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`      // Byzantium switch block (nil = no fork, 0 = already on byzantium)
	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"` // Constantinople switch block (nil = no fork, 0 = already activated)
	PetersburgBlock     *big.Int `json:"petersburgBlock,omitempty"`     // Petersburg switch block (nil = same as Constantinople)
	IstanbulBlock       *big.Int `json:"istanbulBlock,omitempty"`       // Istanbul switch block (nil = no fork, 0 = already on istanbul)
	EWASMBlock          *big.Int `json:"ewasmBlock,omitempty"`          // EWASM switch block (nil = no fork, 0 = already activated)

	// Various consensus engines
	Ethash        *EthashConfig `json:"ethash,omitempty"`
	Clique        *CliqueConfig `json:"clique,omitempty"`
	Claude        *ClaudeConfig `json:"claude,omitempty"`
	ConsensusType int           `json:"consensusType,omitempty"`
}

// EthashConfig is the consensus engine configs for proof-of-work based sealing.
type EthashConfig struct{}

// String implements the stringer interface, returning the consensus engine details.
func (c *EthashConfig) String() string {
	return "ethash"
}

// CliqueConfig is the consensus engine configs for proof-of-authority based sealing.
type CliqueConfig struct {
	Period uint64 `json:"period"` // Number of seconds between blocks to enforce
	Epoch  uint64 `json:"epoch"`  // Epoch length to reset votes and checkpoint
}

// String implements the stringer interface, returning the consensus engine details.
func (c *CliqueConfig) String() string {
	return "clique"
}

//--- add dpos start ---//
// ClaudeConfig is the consensus engine configs for delegated proof-of-stake based sealing.
type ClaudeConfig struct {
	Validators []common.Address `json:"validators"` // Genesis validator list
}

// String implements the stringer interface, returning the consensus engine details.
func (cc *ClaudeConfig) String() string {
	return "claude"
}

//--- add dpos end ---//

// String implements the fmt.Stringer interface.
func (cc *ChainConfig) String() string {
	var engine interface{}
	switch cc.ConsensusType {
	case ConsensusTypeEthash:
		engine = cc.Ethash
	case ConsensusTypeClique:
		engine = cc.Clique
	case ConsensusTypeClaude:
		engine = cc.Claude
	default:
		engine = "unknown"
	}
	return fmt.Sprintf("{ChainID: %v Homestead: %v EIP150: %v EIP155: %v EIP158: %v Byzantium: %v Constantinople: %v  Petersburg: %v Istanbul: %v Engine: %v}",
		cc.ChainID,
		cc.HomesteadBlock,
		cc.EIP150Block,
		cc.EIP155Block,
		cc.EIP158Block,
		cc.ByzantiumBlock,
		cc.ConstantinopleBlock,
		cc.PetersburgBlock,
		cc.IstanbulBlock,
		engine,
	)
}

// IsHomestead returns whether num is either equal to the homestead block or greater.
func (cc *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(cc.HomesteadBlock, num)
}

// IsEIP150 returns whether num is either equal to the EIP150 fork block or greater.
func (cc *ChainConfig) IsEIP150(num *big.Int) bool {
	return isForked(cc.EIP150Block, num)
}

// IsEIP155 returns whether num is either equal to the EIP155 fork block or greater.
func (cc *ChainConfig) IsEIP155(num *big.Int) bool {
	return isForked(cc.EIP155Block, num)
}

// IsEIP158 returns whether num is either equal to the EIP158 fork block or greater.
func (cc *ChainConfig) IsEIP158(num *big.Int) bool {
	return isForked(cc.EIP158Block, num)
}

// IsByzantium returns whether num is either equal to the Byzantium fork block or greater.
func (cc *ChainConfig) IsByzantium(num *big.Int) bool {
	return isForked(cc.ByzantiumBlock, num)
}

// IsConstantinople returns whether num is either equal to the Constantinople fork block or greater.
func (cc *ChainConfig) IsConstantinople(num *big.Int) bool {
	return isForked(cc.ConstantinopleBlock, num)
}

// IsPetersburg returns whether num is either
// - equal to or greater than the PetersburgBlock fork block,
// - OR is nil, and Constantinople is active
func (cc *ChainConfig) IsPetersburg(num *big.Int) bool {
	return isForked(cc.PetersburgBlock, num) || cc.PetersburgBlock == nil && isForked(cc.ConstantinopleBlock, num)
}

// IsIstanbul returns whether num is either equal to the Istanbul fork block or greater.
func (cc *ChainConfig) IsIstanbul(num *big.Int) bool {
	return isForked(cc.IstanbulBlock, num)
}

// IsEWASM returns whether num represents a block number after the EWASM fork
func (cc *ChainConfig) IsEWASM(num *big.Int) bool {
	return isForked(cc.EWASMBlock, num)
}

// GasTable returns the gas table corresponding to the current phase (homestead or homestead reprice).
//
// The returned GasTable's fields shouldn't, under any circumstances, be changed.
func (cc *ChainConfig) GasTable(num *big.Int) GasTable {
	if num == nil {
		return GasTableHomestead
	}
	switch {
	case cc.IsConstantinople(num):
		return GasTableConstantinople
	case cc.IsEIP158(num):
		return GasTableEIP158
	case cc.IsEIP150(num):
		return GasTableEIP150
	default:
		return GasTableHomestead
	}
}

// CheckConfigForkOrder checks that we don't "skip" any forks, geth isn't pluggable enough
// to guarantee that forks
func (cc *ChainConfig) CheckConfigForkOrder() error {
	type fork struct {
		name  string
		block *big.Int
	}
	var lastFork fork
	for _, cur := range []fork{
		{"homesteadBlock", cc.HomesteadBlock},
		{"eip150Block", cc.EIP150Block},
		{"eip155Block", cc.EIP155Block},
		{"eip158Block", cc.EIP158Block},
		{"byzantiumBlock", cc.ByzantiumBlock},
		{"constantinopleBlock", cc.ConstantinopleBlock},
		{"petersburgBlock", cc.PetersburgBlock},
		{"istanbulBlock", cc.IstanbulBlock},
	} {
		if lastFork.name != "" {
			// Next one must be higher number
			if lastFork.block == nil && cur.block != nil {
				return fmt.Errorf("unsupported fork ordering: %v not enabled, but %v enabled at %v",
					lastFork.name, cur.name, cur.block)
			}
			if lastFork.block != nil && cur.block != nil {
				if lastFork.block.Cmp(cur.block) > 0 {
					return fmt.Errorf("unsupported fork ordering: %v enabled at %v, but %v enabled at %v",
						lastFork.name, lastFork.block, cur.name, cur.block)
				}
			}
		}
		lastFork = cur
	}
	return nil
}

// CheckCompatible checks whether scheduled fork transitions have been imported
// with a mismatching chain configuration.
func (cc *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *ConfigCompatError {
	bhead := new(big.Int).SetUint64(height)

	// Iterate checkCompatible to find the lowest conflict.
	var lasterr *ConfigCompatError
	for {
		err := cc.checkCompatible(newcfg, bhead)
		if err == nil || (lasterr != nil && err.RewindTo == lasterr.RewindTo) {
			break
		}
		lasterr = err
		bhead.SetUint64(err.RewindTo)
	}
	return lasterr
}

func (cc *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	if isForkIncompatible(cc.HomesteadBlock, newcfg.HomesteadBlock, head) {
		return newCompatError("Homestead fork block", cc.HomesteadBlock, newcfg.HomesteadBlock)
	}

	if isForkIncompatible(cc.EIP150Block, newcfg.EIP150Block, head) {
		return newCompatError("EIP150 fork block", cc.EIP150Block, newcfg.EIP150Block)
	}
	if isForkIncompatible(cc.EIP155Block, newcfg.EIP155Block, head) {
		return newCompatError("EIP155 fork block", cc.EIP155Block, newcfg.EIP155Block)
	}
	if isForkIncompatible(cc.EIP158Block, newcfg.EIP158Block, head) {
		return newCompatError("EIP158 fork block", cc.EIP158Block, newcfg.EIP158Block)
	}
	if cc.IsEIP158(head) && !configNumEqual(cc.ChainID, newcfg.ChainID) {
		return newCompatError("EIP158 chain ID", cc.EIP158Block, newcfg.EIP158Block)
	}
	if isForkIncompatible(cc.ByzantiumBlock, newcfg.ByzantiumBlock, head) {
		return newCompatError("Byzantium fork block", cc.ByzantiumBlock, newcfg.ByzantiumBlock)
	}
	if isForkIncompatible(cc.ConstantinopleBlock, newcfg.ConstantinopleBlock, head) {
		return newCompatError("Constantinople fork block", cc.ConstantinopleBlock, newcfg.ConstantinopleBlock)
	}
	if isForkIncompatible(cc.PetersburgBlock, newcfg.PetersburgBlock, head) {
		return newCompatError("Petersburg fork block", cc.PetersburgBlock, newcfg.PetersburgBlock)
	}
	if isForkIncompatible(cc.IstanbulBlock, newcfg.IstanbulBlock, head) {
		return newCompatError("Istanbul fork block", cc.IstanbulBlock, newcfg.IstanbulBlock)
	}
	if isForkIncompatible(cc.EWASMBlock, newcfg.EWASMBlock, head) {
		return newCompatError("ewasm fork block", cc.EWASMBlock, newcfg.EWASMBlock)
	}
	return nil
}

// isForkIncompatible returns true if a fork scheduled at s1 cannot be rescheduled to
// block s2 because head is already past the fork.
func isForkIncompatible(s1, s2, head *big.Int) bool {
	return (isForked(s1, head) || isForked(s2, head)) && !configNumEqual(s1, s2)
}

// isForked returns whether a fork scheduled at block s is active at the given head block.
func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func configNumEqual(x, y *big.Int) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return x.Cmp(y) == 0
}

// ConfigCompatError is raised if the locally-stored blockchain is initialised with a
// ChainConfig that would alter the past.
type ConfigCompatError struct {
	What string
	// block numbers of the stored and new configurations
	StoredConfig, NewConfig *big.Int
	// the block number to which the local chain must be rewound to correct the error
	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

// Rules wraps ChainConfig and is merely syntactic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainID                                                 *big.Int
	IsHomestead, IsEIP150, IsEIP155, IsEIP158               bool
	IsByzantium, IsConstantinople, IsPetersburg, IsIstanbul bool
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{
		ChainID:          new(big.Int).Set(chainID),
		IsHomestead:      c.IsHomestead(num),
		IsEIP150:         c.IsEIP150(num),
		IsEIP155:         c.IsEIP155(num),
		IsEIP158:         c.IsEIP158(num),
		IsByzantium:      c.IsByzantium(num),
		IsConstantinople: c.IsConstantinople(num),
		IsPetersburg:     c.IsPetersburg(num),
		IsIstanbul:       c.IsIstanbul(num),
	}
}
