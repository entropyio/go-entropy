package downloader

import (
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/entropy/protocols/ent"
	"time"
)

// headerQueue implements typedQueue and is a type adapter between the generic
// concurrent fetcher and the downloader.
type headerQueue Downloader

// waker returns a notification channel that gets pinged in case more header
// fetches have been queued up, so the fetcher might assign it to idle peers.
func (q *headerQueue) waker() chan bool {
	return q.queue.headerContCh
}

// pending returns the number of headers that are currently queued for fetching
// by the concurrent downloader.
func (q *headerQueue) pending() int {
	return q.queue.PendingHeaders()
}

// capacity is responsible for calculating how many headers a particular peer is
// estimated to be able to retrieve within the alloted round trip time.
func (q *headerQueue) capacity(peer *peerConnection, rtt time.Duration) int {
	return peer.HeaderCapacity(rtt)
}

// updateCapacity is responsible for updating how many headers a particular peer
// is estimated to be able to retrieve in a unit time.
func (q *headerQueue) updateCapacity(peer *peerConnection, items int, span time.Duration) {
	peer.UpdateHeaderRate(items, span)
}

// reserve is responsible for allocating a requested number of pending headers
// from the download queue to the specified peer.
func (q *headerQueue) reserve(peer *peerConnection, items int) (*fetchRequest, bool, bool) {
	return q.queue.ReserveHeaders(peer, items), false, false
}

// unreserve is resposible for removing the current header retrieval allocation
// assigned to a specific peer and placing it back into the pool to allow
// reassigning to some other peer.
func (q *headerQueue) unreserve(peer string) int {
	fails := q.queue.ExpireHeaders(peer)
	if fails > 2 {
		log.Debug("Header delivery timed out", "peer", peer)
	} else {
		log.Debug("Header delivery stalling", "peer", peer)
	}
	return fails
}

// request is responsible for converting a generic fetch request into a header
// one and sending it to the remote peer for fulfillment.
func (q *headerQueue) request(peer *peerConnection, req *fetchRequest, resCh chan *ent.Response) (*ent.Request, error) {
	log.Debug("Requesting new batch of headers", "from", req.From)
	return peer.peer.RequestHeadersByNumber(req.From, MaxHeaderFetch, 0, false, resCh)
}

// deliver is responsible for taking a generic response packet from the concurrent
// fetcher, unpacking the header data and delivering it to the downloader's queue.
func (q *headerQueue) deliver(peer *peerConnection, packet *ent.Response) (int, error) {
	headers := *packet.Res.(*ent.BlockHeadersPacket)
	hashes := packet.Meta.([]common.Hash)

	accepted, err := q.queue.DeliverHeaders(peer.id, headers, hashes, q.headerProcCh)
	switch {
	case err == nil && len(headers) == 0:
		log.Debug("Requested headers delivered")
	case err == nil:
		log.Debug("Delivered new batch of headers", "count", len(headers), "accepted", accepted)
	default:
		log.Debug("Failed to deliver retrieved headers", "err", err)
	}
	return accepted, err
}
