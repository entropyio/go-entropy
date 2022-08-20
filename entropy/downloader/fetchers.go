package downloader

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/entropy/protocols/ent"
	"time"
)

// fetchHeadersByHash is a blocking version of Peer.RequestHeadersByHash which
// handles all the cancellation, interruption and timeout mechanisms of a data
// retrieval to allow blocking API calls.
func (d *Downloader) fetchHeadersByHash(p *peerConnection, hash common.Hash, amount int, skip int, reverse bool) ([]*model.Header, []common.Hash, error) {
	// Create the response sink and send the network request
	start := time.Now()
	resCh := make(chan *ent.Response)

	req, err := p.peer.RequestHeadersByHash(hash, amount, skip, reverse, resCh)
	if err != nil {
		return nil, nil, err
	}
	defer req.Close()

	// Wait until the response arrives, the request is cancelled or times out
	ttl := d.peers.rates.TargetTimeout()

	timeoutTimer := time.NewTimer(ttl)
	defer timeoutTimer.Stop()

	select {
	case <-d.cancelCh:
		return nil, nil, errCanceled

	case <-timeoutTimer.C:
		// Header retrieval timed out, update the metrics
		log.Debug("Header request timed out", "elapsed", ttl)
		headerTimeoutMeter.Mark(1)

		return nil, nil, errTimeout

	case res := <-resCh:
		// Headers successfully retrieved, update the metrics
		headerReqTimer.Update(time.Since(start))
		headerInMeter.Mark(int64(len(*res.Res.(*ent.BlockHeadersPacket))))

		// Don't reject the packet even if it turns out to be bad, downloader will
		// disconnect the peer on its own terms. Simply delivery the headers to
		// be processed by the caller
		res.Done <- nil

		return *res.Res.(*ent.BlockHeadersPacket), res.Meta.([]common.Hash), nil
	}
}

// fetchHeadersByNumber is a blocking version of Peer.RequestHeadersByNumber which
// handles all the cancellation, interruption and timeout mechanisms of a data
// retrieval to allow blocking API calls.
func (d *Downloader) fetchHeadersByNumber(p *peerConnection, number uint64, amount int, skip int, reverse bool) ([]*model.Header, []common.Hash, error) {
	// Create the response sink and send the network request
	start := time.Now()
	resCh := make(chan *ent.Response)

	req, err := p.peer.RequestHeadersByNumber(number, amount, skip, reverse, resCh)
	if err != nil {
		return nil, nil, err
	}
	defer req.Close()

	// Wait until the response arrives, the request is cancelled or times out
	ttl := d.peers.rates.TargetTimeout()

	timeoutTimer := time.NewTimer(ttl)
	defer timeoutTimer.Stop()

	select {
	case <-d.cancelCh:
		return nil, nil, errCanceled

	case <-timeoutTimer.C:
		// Header retrieval timed out, update the metrics
		log.Debug("Header request timed out", "elapsed", ttl)
		headerTimeoutMeter.Mark(1)

		return nil, nil, errTimeout

	case res := <-resCh:
		// Headers successfully retrieved, update the metrics
		headerReqTimer.Update(time.Since(start))
		headerInMeter.Mark(int64(len(*res.Res.(*ent.BlockHeadersPacket))))

		// Don't reject the packet even if it turns out to be bad, downloader will
		// disconnect the peer on its own terms. Simply delivery the headers to
		// be processed by the caller
		res.Done <- nil

		return *res.Res.(*ent.BlockHeadersPacket), res.Meta.([]common.Hash), nil
	}
}
