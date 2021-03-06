package p2p

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/metrics"
)

const (
	MetricsInboundTraffic   = "p2p/ingress" // Name for the registered inbound traffic meter
	MetricsOutboundTraffic  = "p2p/egress"  // Name for the registered outbound traffic meter
	MetricsOutboundConnects = "p2p/dials"   // Name for the registered outbound connects meter
	MetricsInboundConnects  = "p2p/serves"  // Name for the registered inbound connects meter

	MeteredPeerLimit = 1024 // This amount of peers are individually metered
)

var (
	ingressConnectMeter = metrics.NewRegisteredMeter(MetricsInboundConnects, nil)  // Meter counting the ingress connections
	ingressTrafficMeter = metrics.NewRegisteredMeter(MetricsInboundTraffic, nil)   // Meter metering the cumulative ingress traffic
	egressConnectMeter  = metrics.NewRegisteredMeter(MetricsOutboundConnects, nil) // Meter counting the egress connections
	egressTrafficMeter  = metrics.NewRegisteredMeter(MetricsOutboundTraffic, nil)  // Meter metering the cumulative egress traffic
	activePeerGauge     = metrics.NewRegisteredGauge("p2p/peers", nil)             // Gauge tracking the current peer count

	PeerIngressRegistry = metrics.NewPrefixedChildRegistry(metrics.EphemeralRegistry, MetricsInboundTraffic+"/")  // Registry containing the peer ingress
	PeerEgressRegistry  = metrics.NewPrefixedChildRegistry(metrics.EphemeralRegistry, MetricsOutboundTraffic+"/") // Registry containing the peer egress

	meteredPeerFeed  event.Feed // Event feed for peer metrics
	meteredPeerCount int32      // Actually stored peer connection count
)

// MeteredPeerEventType is the type of peer events emitted by a metered connection.
type MeteredPeerEventType int

const (
	// PeerHandshakeSucceeded is the type of event
	// emitted when a peer successfully makes the handshake.
	PeerHandshakeSucceeded MeteredPeerEventType = iota

	// PeerHandshakeFailed is the type of event emitted when a peer fails to
	// make the handshake or disconnects before it.
	PeerHandshakeFailed

	// PeerDisconnected is the type of event emitted when a peer disconnects.
	PeerDisconnected
)

// MeteredPeerEvent is an event emitted when peers connect or disconnect.
type MeteredPeerEvent struct {
	Type    MeteredPeerEventType // Type of peer event
	Addr    string               // TCP address of the peer
	Elapsed time.Duration        // Time elapsed between the connection and the handshake/disconnection
	Peer    *Peer                // Connected remote node instance
	Ingress uint64               // Ingress count at the moment of the event
	Egress  uint64               // Egress count at the moment of the event
}

// SubscribeMeteredPeerEvent registers a subscription for peer life-cycle events
// if metrics collection is enabled.
func SubscribeMeteredPeerEvent(ch chan<- MeteredPeerEvent) event.Subscription {
	return meteredPeerFeed.Subscribe(ch)
}

// meteredConn is a wrapper around a net.Conn that meters both the
// inbound and outbound network traffic.
type meteredConn struct {
	net.Conn // Network connection to wrap with metering

	connected time.Time    // Connection time of the peer
	addr      *net.TCPAddr // TCP address of the peer
	peer      *Peer        // Peer instance

	// trafficMetered denotes if the peer is registered in the traffic registries.
	// Its value is true if the metered peer count doesn't reach the limit in the
	// moment of the peer's connection.
	trafficMetered bool
	ingressMeter   metrics.Meter // Meter for the read bytes of the peer
	egressMeter    metrics.Meter // Meter for the written bytes of the peer

	lock sync.RWMutex // Lock protecting the metered connection's internals
}

// newMeteredConn creates a new metered connection, bumps the ingress or egress
// connection meter and also increases the metered peer count. If the metrics
// system is disabled or the IP address is unspecified, this function returns
// the original object.
func newMeteredConn(conn net.Conn, ingress bool, addr *net.TCPAddr) net.Conn {
	// Short circuit if metrics are disabled
	if !metrics.Enabled {
		return conn
	}
	if addr == nil || addr.IP.IsUnspecified() {
		log.Warning("Peer address is unspecified")
		return conn
	}
	// Bump the connection counters and wrap the connection
	if ingress {
		ingressConnectMeter.Mark(1)
	} else {
		egressConnectMeter.Mark(1)
	}
	activePeerGauge.Inc(1)

	return &meteredConn{
		Conn:      conn,
		addr:      addr,
		connected: time.Now(),
	}
}

// Read delegates a network read to the underlying connection, bumping the common
// and the peer ingress traffic meters along the way.
func (c *meteredConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	ingressTrafficMeter.Mark(int64(n))
	c.lock.RLock()
	if c.trafficMetered {
		c.ingressMeter.Mark(int64(n))
	}
	c.lock.RUnlock()
	return n, err
}

// Write delegates a network write to the underlying connection, bumping the common
// and the peer egress traffic meters along the way.
func (c *meteredConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	egressTrafficMeter.Mark(int64(n))
	c.lock.RLock()
	if c.trafficMetered {
		c.egressMeter.Mark(int64(n))
	}
	c.lock.RUnlock()
	return n, err
}

// handshakeDone is called after the connection passes the handshake.
func (c *meteredConn) handshakeDone(peer *Peer) {
	if atomic.AddInt32(&meteredPeerCount, 1) >= MeteredPeerLimit {
		// Don't register the peer in the traffic registries.
		atomic.AddInt32(&meteredPeerCount, -1)
		c.lock.Lock()
		c.peer, c.trafficMetered = peer, false
		c.lock.Unlock()
		log.Warning("Metered peer count reached the limit")
	} else {
		enode := peer.Node().String()
		c.lock.Lock()
		c.peer, c.trafficMetered = peer, true
		c.ingressMeter = metrics.NewRegisteredMeter(enode, PeerIngressRegistry)
		c.egressMeter = metrics.NewRegisteredMeter(enode, PeerEgressRegistry)
		c.lock.Unlock()
	}
	meteredPeerFeed.Send(MeteredPeerEvent{
		Type:    PeerHandshakeSucceeded,
		Addr:    c.addr.String(),
		Peer:    peer,
		Elapsed: time.Since(c.connected),
	})
}

// Close delegates a close operation to the underlying connection, unregisters
// the peer from the traffic registries and emits close event.
func (c *meteredConn) Close() error {
	err := c.Conn.Close()
	c.lock.RLock()
	if c.peer == nil {
		// If the peer disconnects before/during the handshake.
		c.lock.RUnlock()
		meteredPeerFeed.Send(MeteredPeerEvent{
			Type:    PeerHandshakeFailed,
			Addr:    c.addr.String(),
			Elapsed: time.Since(c.connected),
		})
		activePeerGauge.Dec(1)
		return err
	}
	peer := c.peer
	if !c.trafficMetered {
		// If the peer isn't registered in the traffic registries.
		c.lock.RUnlock()
		meteredPeerFeed.Send(MeteredPeerEvent{
			Type: PeerDisconnected,
			Addr: c.addr.String(),
			Peer: peer,
		})
		activePeerGauge.Dec(1)
		return err
	}
	ingress, egress, enode := uint64(c.ingressMeter.Count()), uint64(c.egressMeter.Count()), c.peer.Node().String()
	c.lock.RUnlock()

	// Decrement the metered peer count
	atomic.AddInt32(&meteredPeerCount, -1)

	// Unregister the peer from the traffic registries
	PeerIngressRegistry.Unregister(enode)
	PeerEgressRegistry.Unregister(enode)

	meteredPeerFeed.Send(MeteredPeerEvent{
		Type:    PeerDisconnected,
		Addr:    c.addr.String(),
		Peer:    peer,
		Ingress: ingress,
		Egress:  egress,
	})
	activePeerGauge.Dec(1)
	return err
}
