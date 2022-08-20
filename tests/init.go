package tests

import (
	"fmt"
	"github.com/entropyio/go-entropy/config"
	"math/big"
	"sort"
)

// Forks table defines supported forks and their chain config.
var Forks = map[string]*config.ChainConfig{
	"Frontier": {
		ChainID: big.NewInt(1),
	},
	"Homestead": {
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(0),
	},
	"EIP150": {
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(0),
		EIP150Block:    big.NewInt(0),
	},
	"FrontierToHomesteadAt5": {
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(5),
	},
	"HomesteadToEIP150At5": {
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(0),
		EIP150Block:    big.NewInt(5),
	},
	"HomesteadToDaoAt5": {
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(0),
	},
	"London": {
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(0),
		EIP150Block:    big.NewInt(0),
		LondonBlock:    big.NewInt(0),
	},
	"Merged": {
		ChainID:                 big.NewInt(1),
		HomesteadBlock:          big.NewInt(0),
		EIP150Block:             big.NewInt(0),
		LondonBlock:             big.NewInt(0),
		TerminalTotalDifficulty: big.NewInt(0),
	},
}

// Returns the set of defined fork names
func AvailableForks() []string {
	var availableForks []string
	for k := range Forks {
		availableForks = append(availableForks, k)
	}
	sort.Strings(availableForks)
	return availableForks
}

// UnsupportedForkError is returned when a test requests a fork that isn't implemented.
type UnsupportedForkError struct {
	Name string
}

func (e UnsupportedForkError) Error() string {
	return fmt.Sprintf("unsupported fork %q", e.Name)
}
