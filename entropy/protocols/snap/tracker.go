package snap

import (
	"github.com/entropyio/go-entropy/server/p2p/tracker"
	"time"
)

// requestTracker is a singleton tracker for request times.
var requestTracker = tracker.New(ProtocolName, time.Minute)
