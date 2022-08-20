package entropy

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/entropy/protocols/ent"
	"github.com/entropyio/go-entropy/server/p2p/enode"
	"math/big"
	"sync/atomic"
	"time"
)

// entropyHandler implements the entropy.Backend interface to handle the various network
// packets that are sent as replies or broadcasts.
type entropyHandler handler

func (h *entropyHandler) Chain() *blockchain.BlockChain { return h.chain }
func (h *entropyHandler) TxPool() ent.TxPool            { return h.txpool }

// RunPeer is invoked when a peer joins on the `eth` protocol.
func (h *entropyHandler) RunPeer(peer *ent.Peer, hand ent.Handler) error {
	return (*handler)(h).runEntropyPeer(peer, hand)
}

// PeerInfo retrieves all known `eth` information about a peer.
func (h *entropyHandler) PeerInfo(id enode.ID) interface{} {
	if p := h.peers.peer(id.String()); p != nil {
		return p.info()
	}
	return nil
}

// AcceptTxs retrieves whether transaction processing is enabled on the node
// or if inbound transactions should simply be dropped.
func (h *entropyHandler) AcceptTxs() bool {
	return atomic.LoadUint32(&h.acceptTxs) == 1
}

// Handle is invoked from a peer's message handler when it receives a new remote
// message that the handler couldn't consume and serve itself.
func (h *entropyHandler) Handle(peer *ent.Peer, packet ent.Packet) error {
	// Consume any broadcasts and announces, forwarding the rest to the downloader
	switch packet := packet.(type) {
	case *ent.NewBlockHashesPacket:
		hashes, numbers := packet.Unpack()
		return h.handleBlockAnnounces(peer, hashes, numbers)

	case *ent.NewBlockPacket:
		return h.handleBlockBroadcast(peer, packet.Block, packet.TD)

	case *ent.NewPooledTransactionHashesPacket:
		return h.txFetcher.Notify(peer.ID(), *packet)

	case *ent.TransactionsPacket:
		return h.txFetcher.Enqueue(peer.ID(), *packet, false)

	case *ent.PooledTransactionsPacket:
		return h.txFetcher.Enqueue(peer.ID(), *packet, true)

	default:
		return fmt.Errorf("unexpected eth packet type: %T", packet)
	}
}

// handleBlockAnnounces is invoked from a peer's message handler when it transmits a
// batch of block announcements for the local node to process.
func (h *entropyHandler) handleBlockAnnounces(peer *ent.Peer, hashes []common.Hash, numbers []uint64) error {
	// Drop all incoming block announces from the p2p network if
	// the chain already entered the pos stage and disconnect the
	// remote peer.
	if h.merger.PoSFinalized() {
		// TODO (MariusVanDerWijden) drop non-updated peers after the merge
		return nil
		// return errors.New("unexpected block announces")
	}
	// Schedule all the unknown hashes for retrieval
	var (
		unknownHashes  = make([]common.Hash, 0, len(hashes))
		unknownNumbers = make([]uint64, 0, len(numbers))
	)
	for i := 0; i < len(hashes); i++ {
		if !h.chain.HasBlock(hashes[i], numbers[i]) {
			unknownHashes = append(unknownHashes, hashes[i])
			unknownNumbers = append(unknownNumbers, numbers[i])
		}
	}
	for i := 0; i < len(unknownHashes); i++ {
		_ = h.blockFetcher.Notify(peer.ID(), unknownHashes[i], unknownNumbers[i], time.Now(), peer.RequestOneHeader, peer.RequestBodies)
	}
	return nil
}

// handleBlockBroadcast is invoked from a peer's message handler when it transmits a
// block broadcast for the local node to process.
func (h *entropyHandler) handleBlockBroadcast(peer *ent.Peer, block *model.Block, td *big.Int) error {
	// Drop all incoming block announces from the p2p network if
	// the chain already entered the pos stage and disconnect the
	// remote peer.
	if h.merger.PoSFinalized() {
		// TODO (MariusVanDerWijden) drop non-updated peers after the merge
		return nil
		// return errors.New("unexpected block announces")
	}
	// Schedule the block for import
	_ = h.blockFetcher.Enqueue(peer.ID(), block)

	// Assuming the block is importable by the peer, but possibly not yet done so,
	// calculate the head hash and TD that the peer truly must have.
	var (
		trueHead = block.ParentHash()
		trueTD   = new(big.Int).Sub(td, block.Difficulty())
	)
	// Update the peer's total difficulty if better than the previous
	if _, td := peer.Head(); trueTD.Cmp(td) > 0 {
		peer.SetHead(trueHead, trueTD)
		h.chainSync.handlePeerEvent(peer)
	}
	return nil
}
