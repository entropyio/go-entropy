package ent

import (
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/forkid"
	"github.com/entropyio/go-entropy/common/rlp"
	"github.com/entropyio/go-entropy/server/p2p/enode"
)

// enrEntry is the ENR entry which advertises `eth` protocol on the discovery.
type enrEntry struct {
	ForkID forkid.ID // Fork identifier per EIP-2124

	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}

// ENRKey implements enr.Entry.
func (e enrEntry) ENRKey() string {
	return "eth"
}

// StartENRUpdater starts the `eth` ENR updater loop, which listens for chain
// head events and updates the requested node record whenever a fork is passed.
func StartENRUpdater(chain *blockchain.BlockChain, ln *enode.LocalNode) {
	var newHead = make(chan blockchain.ChainHeadEvent, 10)
	sub := chain.SubscribeChainHeadEvent(newHead)

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-newHead:
				ln.Set(currentENREntry(chain))
			case <-sub.Err():
				// Would be nice to sync with Stop, but there is no
				// good way to do that.
				return
			}
		}
	}()
}

// currentENREntry constructs an `eth` ENR entry based on the current state of the chain.
func currentENREntry(chain *blockchain.BlockChain) *enrEntry {
	return &enrEntry{
		ForkID: forkid.NewID(chain.Config(), chain.Genesis().Hash(), chain.CurrentHeader().Number.Uint64()),
	}
}
