package downloader

import (
	"github.com/entropyio/go-entropy/metrics"
)

var (
	headerInMeter      = metrics.NewRegisteredMeter("entropy/downloader/headers/in", nil)
	headerReqTimer     = metrics.NewRegisteredTimer("entropy/downloader/headers/req", nil)
	headerDropMeter    = metrics.NewRegisteredMeter("entropy/downloader/headers/drop", nil)
	headerTimeoutMeter = metrics.NewRegisteredMeter("entropy/downloader/headers/timeout", nil)

	bodyInMeter      = metrics.NewRegisteredMeter("entropy/downloader/bodies/in", nil)
	bodyReqTimer     = metrics.NewRegisteredTimer("entropy/downloader/bodies/req", nil)
	bodyDropMeter    = metrics.NewRegisteredMeter("entropy/downloader/bodies/drop", nil)
	bodyTimeoutMeter = metrics.NewRegisteredMeter("entropy/downloader/bodies/timeout", nil)

	receiptInMeter      = metrics.NewRegisteredMeter("entropy/downloader/receipts/in", nil)
	receiptReqTimer     = metrics.NewRegisteredTimer("entropy/downloader/receipts/req", nil)
	receiptDropMeter    = metrics.NewRegisteredMeter("entropy/downloader/receipts/drop", nil)
	receiptTimeoutMeter = metrics.NewRegisteredMeter("entropy/downloader/receipts/timeout", nil)

	stateInMeter   = metrics.NewRegisteredMeter("entropy/downloader/states/in", nil)
	stateDropMeter = metrics.NewRegisteredMeter("entropy/downloader/states/drop", nil)
)
