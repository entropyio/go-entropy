package config

import (
	"fmt"
	"math/big"

	"github.com/entropyio/go-entropy/common"
)

// Genesis hashes to enforce below configs on.
var (
	MainnetGenesisHash = common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")
	TestnetGenesisHash = common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d")
)

const (
	ConsensusTypeEthash = 1
	ConsensusTypeClique = 2
	ConsensusTypeClaude = 3
)

const SelectedConsensusType = ConsensusTypeEthash

var (
	// EthashChainConfig is the chain parameters to run a node on the pow network.
	EthashChainConfig = &ChainConfig{
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(1),
		Ethash:         new(EthashConfig),
		Clique:         &CliqueConfig{Period: 15, Epoch: 30000},
		Claude:         new(ClaudeConfig),
		ConsensusType:  ConsensusTypeEthash,
	}

	// CliqueChainConfig contains the chain parameters to run a node on the pos network.
	CliqueChainConfig = &ChainConfig{
		ChainID:        big.NewInt(2),
		HomesteadBlock: big.NewInt(1),
		Ethash:         new(EthashConfig),
		Clique:         &CliqueConfig{Period: 15, Epoch: 30000},
		Claude:         new(ClaudeConfig),
		ConsensusType:  ConsensusTypeClique,
	}

	// ClaudeChainConfig contains the chain parameters to run a node on the dpos network.
	ClaudeChainConfig = &ChainConfig{
		ChainID:        big.NewInt(4),
		HomesteadBlock: big.NewInt(1),
		Ethash:         new(EthashConfig),
		Clique:         &CliqueConfig{Period: 15, Epoch: 30000},
		Claude:         new(ClaudeConfig),
		ConsensusType:  ConsensusTypeClaude,
	}

	TestChainConfig = &ChainConfig{
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(0),
		Ethash:         new(EthashConfig),
		Clique:         &CliqueConfig{Period: 15, Epoch: 30000},
		Claude:         new(ClaudeConfig),
		ConsensusType:  SelectedConsensusType,
	}
)

func GetSelectedChainConfig() *ChainConfig {
	return GetChainConfig(SelectedConsensusType)
}

func GetChainConfig(consensusType int) *ChainConfig {
	switch consensusType {
	case ConsensusTypeEthash:
		return EthashChainConfig
	case ConsensusTypeClique:
		return CliqueChainConfig
	case ConsensusTypeClaude:
		return ClaudeChainConfig
	default:
		return EthashChainConfig
	}
}

// ChainConfig is the blockchain config which determines the blockchain settings.
//
// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` // Homestead switch block (nil = no fork, 0 = already homestead)

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
	return fmt.Sprintf("{ChainID: %v, Homestead: %v,  Engine: %v}",
		cc.ChainID,
		cc.HomesteadBlock,
		engine,
	)
}

// IsHomestead returns whether num is either equal to the homestead block or greater.
func (cc *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(cc.HomesteadBlock, num)
}

// GasTable returns the gas table corresponding to the current phase (homestead or homestead reprice).
//
// The returned GasTable's fields shouldn't, under any circumstances, be changed.
func (cc *ChainConfig) GasTable(num *big.Int) GasTable {
	return GasTableHomestead
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

// Rules wraps ChainConfig and is merely syntatic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainID     *big.Int
	IsHomestead bool
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{ChainID: new(big.Int).Set(chainID), IsHomestead: c.IsHomestead(num)}
}
