package entropy

import (
	"github.com/entropyio/go-entropy/entropy/protocols/ent"
	"github.com/entropyio/go-entropy/entropy/protocols/snap"
	"math/big"
)

// entropyPeerInfo represents a short summary of the `eth` sub-protocol metadata known
// about a connected peer.
type entropyPeerInfo struct {
	Version    uint     `json:"version"`    // Entropy protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the peer's blockchain
	Head       string   `json:"head"`       // Hex hash of the peer's best owned block
}

// entropyPeer is a wrapper around eth.Peer to maintain a few extra metadata.
type entropyPeer struct {
	*ent.Peer
	snapExt *snapPeer // Satellite `snap` connection
}

// info gathers and returns some `entropy` protocol metadata known about a peer.
func (p *entropyPeer) info() *entropyPeerInfo {
	hash, td := p.Head()

	return &entropyPeerInfo{
		Version:    p.Version(),
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// snapPeerInfo represents a short summary of the `snap` sub-protocol metadata known
// about a connected peer.
type snapPeerInfo struct {
	Version uint `json:"version"` // Snapshot protocol version negotiated
}

// snapPeer is a wrapper around snap.Peer to maintain a few extra metadata.
type snapPeer struct {
	*snap.Peer
}

// info gathers and returns some `snap` protocol metadata known about a peer.
func (p *snapPeer) info() *snapPeerInfo {
	return &snapPeerInfo{
		Version: p.Version(),
	}
}
