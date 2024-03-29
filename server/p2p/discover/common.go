package discover

import (
	"crypto/ecdsa"
	"github.com/entropyio/go-entropy/common/mclock"
	"github.com/entropyio/go-entropy/common/netutil"
	"github.com/entropyio/go-entropy/server/p2p/enode"
	"github.com/entropyio/go-entropy/server/p2p/enr"
	"github.com/op/go-logging"
	"net"
)

// UDPConn is a network connection on which discovery can operate.
type UDPConn interface {
	ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error)
	WriteToUDP(b []byte, addr *net.UDPAddr) (n int, err error)
	Close() error
	LocalAddr() net.Addr
}

// Config holds settings for the discovery listener.
type Config struct {
	// These settings are required and configure the UDP listener:
	PrivateKey *ecdsa.PrivateKey

	// These settings are optional:
	NetRestrict  *netutil.Netlist   // list of allowed IP networks
	Bootnodes    []*enode.Node      // list of bootstrap nodes
	Unhandled    chan<- ReadPacket  // unhandled packets are sent on this channel
	Log          *logging.Logger    // if set, log messages go here
	ValidSchemes enr.IdentityScheme // allowed identity schemes
	Clock        mclock.Clock
}

func (cfg Config) withDefaults() Config {
	if cfg.Log == nil {
		cfg.Log = log
	}
	if cfg.ValidSchemes == nil {
		cfg.ValidSchemes = enode.ValidSchemes
	}
	if cfg.Clock == nil {
		cfg.Clock = mclock.System{}
	}
	return cfg
}

// ListenUDP starts listening for discovery packets on the given UDP socket.
func ListenUDP(c UDPConn, ln *enode.LocalNode, cfg Config) (*UDPv4, error) {
	return ListenV4(c, ln, cfg)
}

// ReadPacket is a packet that couldn't be handled. Those packets are sent to the unhandled
// channel if configured.
type ReadPacket struct {
	Data []byte
	Addr *net.UDPAddr
}

func min(x, y int) int {
	if x > y {
		return y
	}
	return x
}
