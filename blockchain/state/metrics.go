package state

import "github.com/entropyio/go-entropy/metrics"

var (
	accountUpdatedMeter   = metrics.NewRegisteredMeter("state/update/account", nil)
	storageUpdatedMeter   = metrics.NewRegisteredMeter("state/update/storage", nil)
	accountDeletedMeter   = metrics.NewRegisteredMeter("state/delete/account", nil)
	storageDeletedMeter   = metrics.NewRegisteredMeter("state/delete/storage", nil)
	accountCommittedMeter = metrics.NewRegisteredMeter("state/commit/account", nil)
	storageCommittedMeter = metrics.NewRegisteredMeter("state/commit/storage", nil)
)
