package entropy

import (
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/forkid"
	"github.com/entropyio/go-entropy/common/rlputil"
	"github.com/entropyio/go-entropy/server/p2p/enode"
)

// ethEntry is the "eth" ENR entry which advertises eth protocol
// on the discovery network.
type ethEntry struct {
	ForkID forkid.ID // Fork identifier per EIP-2124

	// Ignore additional fields (for forward compatibility).
	Rest []rlputil.RawValue `rlp:"tail"`
}

// ENRKey implements enr.Entry.
func (e ethEntry) ENRKey() string {
	return "entropy"
}

func (s *Entropy) startEthEntryUpdate(ln *enode.LocalNode) {
	var newHead = make(chan blockchain.ChainHeadEvent, 10)
	sub := s.blockchain.SubscribeChainHeadEvent(newHead)

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-newHead:
				ln.Set(s.currentEthEntry())
			case <-sub.Err():
				// Would be nice to sync with eth.Stop, but there is no
				// good way to do that.
				return
			}
		}
	}()
}

func (s *Entropy) currentEthEntry() *ethEntry {
	return &ethEntry{ForkID: forkid.NewID(s.blockchain)}
}
