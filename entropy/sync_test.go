package entropy

import (
	"github.com/entropyio/go-entropy/entropy/downloader"
	"github.com/entropyio/go-entropy/entropy/protocols/ent"
	"github.com/entropyio/go-entropy/entropy/protocols/snap"
	"github.com/entropyio/go-entropy/server/p2p"
	"github.com/entropyio/go-entropy/server/p2p/enode"
	"sync/atomic"
	"testing"
	"time"
)

// Tests that snap sync is disabled after a successful sync cycle.
func TestSnapSyncDisabling66(t *testing.T) { testSnapSyncDisabling(t, ent.ETH66, snap.SNAP1) }
func TestSnapSyncDisabling67(t *testing.T) { testSnapSyncDisabling(t, ent.ETH67, snap.SNAP1) }

// Tests that snap sync gets disabled as soon as a real block is successfully
// imported into the blockchain.
func testSnapSyncDisabling(t *testing.T, ethVer uint, snapVer uint) {
	t.Parallel()

	// Create an empty handler and ensure it's in snap sync mode
	empty := newTestHandler()
	if atomic.LoadUint32(&empty.handler.snapSync) == 0 {
		t.Fatalf("snap sync disabled on pristine blockchain")
	}
	defer empty.close()

	// Create a full handler and ensure snap sync ends up disabled
	full := newTestHandlerWithBlocks(1024)
	if atomic.LoadUint32(&full.handler.snapSync) == 1 {
		t.Fatalf("snap sync not disabled on non-empty blockchain")
	}
	defer full.close()

	// Sync up the two handlers via both `eth` and `snap`
	caps := []p2p.Cap{{Name: "eth", Version: ethVer}, {Name: "snap", Version: snapVer}}

	emptyPipeEth, fullPipeEth := p2p.MsgPipe()
	defer emptyPipeEth.Close()
	defer fullPipeEth.Close()

	emptyPeerEth := ent.NewPeer(ethVer, p2p.NewPeer(enode.ID{1}, "", caps), emptyPipeEth, empty.txpool)
	fullPeerEth := ent.NewPeer(ethVer, p2p.NewPeer(enode.ID{2}, "", caps), fullPipeEth, full.txpool)
	defer emptyPeerEth.Close()
	defer fullPeerEth.Close()

	go empty.handler.runEntropyPeer(emptyPeerEth, func(peer *ent.Peer) error {
		return ent.Handle((*entropyHandler)(empty.handler), peer)
	})
	go full.handler.runEntropyPeer(fullPeerEth, func(peer *ent.Peer) error {
		return ent.Handle((*entropyHandler)(full.handler), peer)
	})

	emptyPipeSnap, fullPipeSnap := p2p.MsgPipe()
	defer emptyPipeSnap.Close()
	defer fullPipeSnap.Close()

	emptyPeerSnap := snap.NewPeer(snapVer, p2p.NewPeer(enode.ID{1}, "", caps), emptyPipeSnap)
	fullPeerSnap := snap.NewPeer(snapVer, p2p.NewPeer(enode.ID{2}, "", caps), fullPipeSnap)

	go empty.handler.runSnapExtension(emptyPeerSnap, func(peer *snap.Peer) error {
		return snap.Handle((*snapHandler)(empty.handler), peer)
	})
	go full.handler.runSnapExtension(fullPeerSnap, func(peer *snap.Peer) error {
		return snap.Handle((*snapHandler)(full.handler), peer)
	})
	// Wait a bit for the above handlers to start
	time.Sleep(250 * time.Millisecond)

	// Check that snap sync was disabled
	op := peerToSyncOp(downloader.SnapSync, empty.handler.peers.peerWithHighestTD())
	if err := empty.handler.doSync(op); err != nil {
		t.Fatal("sync failed:", err)
	}
	if atomic.LoadUint32(&empty.handler.snapSync) == 1 {
		t.Fatalf("snap sync not disabled after successful synchronisation")
	}
}
