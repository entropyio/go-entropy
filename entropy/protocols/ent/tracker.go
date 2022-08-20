package ent

import (
	"github.com/entropyio/go-entropy/server/p2p/tracker"
	"time"
)

// requestTracker is a singleton tracker for eth/66 and newer request times.
var requestTracker = tracker.New(ProtocolName, 5*time.Minute)
