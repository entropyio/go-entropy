package config

import (
	"encoding/binary"
	"fmt"
	"github.com/entropyio/go-entropy/common"
	"golang.org/x/crypto/sha3"
	"math/big"
)

const (
	ConsensusTypeEthash = 1
	ConsensusTypeClique = 2
	ConsensusTypeClaude = 3
)

var (
	// EthashChainConfig is the chain parameters to run a node on the main network.
	EthashChainConfig = &ChainConfig{
		ChainID:        big.NewInt(ConsensusTypeEthash),
		HomesteadBlock: big.NewInt(1_150_000),
		EIP150Block:    big.NewInt(2_463_000),
		LondonBlock:    big.NewInt(12_965_000),
		Ethash:         new(EthashConfig),
		Clique:         nil,
		//Claude:              new(ClaudeConfig),
		//ConsensusType:       ConsensusTypeEthash,
	}

	// CliqueChainConfig contains the chain parameters to run a node on the pos network.
	CliqueChainConfig = &ChainConfig{
		ChainID:        big.NewInt(ConsensusTypeClique),
		HomesteadBlock: big.NewInt(1_150_000),
		EIP150Block:    big.NewInt(2_463_000),
		LondonBlock:    big.NewInt(12_965_000),
		Ethash:         nil,
		Clique:         &CliqueConfig{Period: 15, Epoch: 30000},
		//Claude:              new(ClaudeConfig),
		//ConsensusType:       ConsensusTypeClique,
	}

	// ClaudeChainConfig contains the chain parameters to run a node on the dpos network.
	ClaudeChainConfig = &ChainConfig{
		ChainID:        big.NewInt(ConsensusTypeClaude),
		HomesteadBlock: big.NewInt(1_150_000),
		EIP150Block:    big.NewInt(2_463_000),
		LondonBlock:    big.NewInt(12_965_000),
		Ethash:         nil,
		Clique:         nil,
		//Claude:              new(ClaudeConfig),
		//ConsensusType:       ConsensusTypeClaude,
	}
)

var (
	// default config
	DefaultChainConfig = EthashChainConfig
	MainnetChainConfig = EthashChainConfig
	TestnetChainConfig = CliqueChainConfig
	TestChainConfig    = EthashChainConfig
)

// TrustedCheckpoints associates each known checkpoint with the genesis hash of
// the chain it belongs to.
var TrustedCheckpoints = map[common.Hash]*TrustedCheckpoint{
	MainnetGenesisHash: MainnetTrustedCheckpoint,
	TestnetGenesisHash: TestnetTrustedCheckpoint,
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

	// TestnetTrustedCheckpoint contains the light client trusted checkpoint for the Ropsten test network.
	TestnetTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 180,
		SectionHead:  common.HexToHash("0xc5741683f9fcff7b670732deef2ebe6e7ff7a7bb29249401300b13b4eee690a6"),
		CHTRoot:      common.HexToHash("0xf03ccebbf71a0998833afdf0e7c29095138c2df1cee6ed44ad9da62b5206b8ad"),
		BloomRoot:    common.HexToHash("0xec46c48cf218891c91ad1139d3b3aec7cf385d4c1100c06711e56b83d8993b23"),
	}
)

// NetworkNames are user friendly names to use in the chain spec banner.
var NetworkNames = map[string]string{
	MainnetChainConfig.ChainID.String(): "mainNet",
	CliqueChainConfig.ChainID.String():  "cliqueNet",
	ClaudeChainConfig.ChainID.String():  "claudeNet",
}

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
	var sectionIndex [8]byte
	binary.BigEndian.PutUint64(sectionIndex[:], c.SectionIndex)

	w := sha3.NewLegacyKeccak256()
	w.Write(sectionIndex[:])
	w.Write(c.SectionHead[:])
	w.Write(c.CHTRoot[:])
	w.Write(c.BloomRoot[:])

	var h common.Hash
	w.Sum(h[:0])
	return h
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
	EIP150Block    *big.Int `json:"eip150Block,omitempty"`    // EIP150 HF block (nil = no fork)
	LondonBlock    *big.Int `json:"londonBlock,omitempty"`    // London switch block (nil = no fork, 0 = already on london)

	// TerminalTotalDifficulty is the amount of total difficulty reached by
	// the network that triggers the consensus upgrade.
	TerminalTotalDifficulty *big.Int `json:"terminalTotalDifficulty,omitempty"`

	// Various consensus engines
	Ethash *EthashConfig `json:"ethash,omitempty"`
	Clique *CliqueConfig `json:"clique,omitempty"`
	//Claude        *ClaudeConfig `json:"claude,omitempty"`
	//ConsensusType int           `json:"consensusType,omitempty"`
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
// String implements the fmt.Stringer interface.
func (cc *ChainConfig) String() string {
	var banner string

	// Create some basinc network config output
	network := NetworkNames[cc.ChainID.String()]
	if network == "" {
		network = "unknown"
	}
	banner += fmt.Sprintf("Chain ID:  %v (%s)\n", cc.ChainID, network)
	switch {
	case cc.Ethash != nil:
		if cc.TerminalTotalDifficulty == nil {
			banner += "Consensus: Ethash (proof-of-work)\n"
		} else {
			banner += "Consensus: Beacon (proof-of-stake), merged from Ethash (proof-of-work)\n"
		}
	case cc.Clique != nil:
		if cc.TerminalTotalDifficulty == nil {
			banner += "Consensus: Clique (proof-of-authority)\n"
		} else {
			banner += "Consensus: Beacon (proof-of-stake), merged from Clique (proof-of-authority)\n"
		}
	default:
		banner += "Consensus: unknown\n"
	}
	banner += "\n"

	// Create a list of forks with a short description of them. Forks that only
	// makes sense for mainnet should be optional at printing to avoid bloating
	// the output for testnets and private networks.
	banner += "Pre-Merge hard forks:\n"
	banner += fmt.Sprintf(" - Homestead:                   %-8v (https://github.com/entropy/execution-specs/blob/master/network-upgrades/mainnet-upgrades/homestead.md)\n", cc.HomesteadBlock)
	banner += fmt.Sprintf(" - Tangerine Whistle (EIP 150): %-8v (https://github.com/entropy/execution-specs/blob/master/network-upgrades/mainnet-upgrades/tangerine-whistle.md)\n", cc.EIP150Block)
	banner += fmt.Sprintf(" - London:                      %-8v (https://github.com/entropy/execution-specs/blob/master/network-upgrades/mainnet-upgrades/london.md)\n", cc.LondonBlock)
	banner += "\n"

	// Add a special section for the merge as it's non-obvious
	if cc.TerminalTotalDifficulty == nil {
		banner += "Merge not configured!\n"
		banner += " - Hard-fork specification: https://github.com/entropy/execution-specs/blob/master/network-upgrades/mainnet-upgrades/paris.md)"
	} else {
		banner += "Merge configured:\n"
		banner += " - Hard-fork specification:   https://github.com/entropy/execution-specs/blob/master/network-upgrades/mainnet-upgrades/paris.md)\n"
		banner += fmt.Sprintf(" - Total terminal difficulty: %v\n", cc.TerminalTotalDifficulty)
	}
	return banner
}

// IsHomestead returns whether num is either equal to the homestead block or greater.
func (cc *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(cc.HomesteadBlock, num)
}

// IsEIP150 returns whether num is either equal to the EIP150 fork block or greater.
func (cc *ChainConfig) IsEIP150(num *big.Int) bool {
	return isForked(cc.EIP150Block, num)
}

// IsLondon returns whether num is either equal to the London fork block or greater.
func (cc *ChainConfig) IsLondon(num *big.Int) bool {
	return isForked(cc.LondonBlock, num)
}

// IsTerminalPoWBlock returns whether the given block is the last block of PoW stage.
func (cc *ChainConfig) IsTerminalPoWBlock(parentTotalDiff *big.Int, totalDiff *big.Int) bool {
	if cc.TerminalTotalDifficulty == nil {
		return false
	}
	return parentTotalDiff.Cmp(cc.TerminalTotalDifficulty) < 0 && totalDiff.Cmp(cc.TerminalTotalDifficulty) >= 0
}

// CheckCompatible checks whether scheduled fork transitions have been imported
// with a mismatching chain configuration.
func (cc *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *CompatError {
	bhead := new(big.Int).SetUint64(height)

	// Iterate checkCompatible to find the lowest conflict.
	var lasterr *CompatError
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

// CheckConfigForkOrder checks that we don't "skip" any forks, geth isn't pluggable enough
// to guarantee that forks can be implemented in a different order than on official networks
func (cc *ChainConfig) CheckConfigForkOrder() error {
	type fork struct {
		name     string
		block    *big.Int
		optional bool // if true, the fork may be nil and next fork is still allowed
	}
	var lastFork fork
	for _, cur := range []fork{
		{name: "homesteadBlock", block: cc.HomesteadBlock},
		{name: "eip150Block", block: cc.EIP150Block},
		{name: "londonBlock", block: cc.LondonBlock},
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
		// If it was optional and not set, then ignore it
		if !cur.optional || cur.block != nil {
			lastFork = cur
		}
	}
	return nil
}

func (cc *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *CompatError {
	if isForkIncompatible(cc.HomesteadBlock, newcfg.HomesteadBlock, head) {
		return newCompatError("Homestead fork block", cc.HomesteadBlock, newcfg.HomesteadBlock)
	}
	if isForkIncompatible(cc.EIP150Block, newcfg.EIP150Block, head) {
		return newCompatError("EIP150 fork block", cc.EIP150Block, newcfg.EIP150Block)
	}
	if isForkIncompatible(cc.LondonBlock, newcfg.LondonBlock, head) {
		return newCompatError("London fork block", cc.LondonBlock, newcfg.LondonBlock)
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
		return false
	}
	return x.Cmp(y) == 0
}

// ConfigCompatError is raised if the locally-stored blockchain is initialised with a
// ChainConfig that would alter the past.
type CompatError struct {
	What string
	// block numbers of the stored and new configurations
	StoredConfig, NewConfig *big.Int
	// the block number to which the local chain must be rewound to correct the error
	RewindTo uint64
}

func newCompatError(what string, storedBlock, newBlock *big.Int) *CompatError {
	var rew *big.Int
	switch {
	case storedBlock == nil:
		rew = newBlock
	case newBlock == nil || storedBlock.Cmp(newBlock) < 0:
		rew = storedBlock
	default:
		rew = newBlock
	}
	err := &CompatError{what, storedBlock, newBlock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *CompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

// Rules wraps ChainConfig and is merely syntactic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainID                         *big.Int
	IsHomestead, IsEIP150, IsLondon bool
	IsMerge                         bool
}

// Rules ensures c's ChainID is not nil.
func (cc *ChainConfig) Rules(num *big.Int, isMerge bool) Rules {
	chainID := cc.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{
		ChainID:     new(big.Int).Set(chainID),
		IsHomestead: cc.IsHomestead(num),
		IsEIP150:    cc.IsEIP150(num),
		IsLondon:    cc.IsLondon(num),
		IsMerge:     isMerge,
	}
}
