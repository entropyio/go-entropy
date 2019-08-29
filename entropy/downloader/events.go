package downloader

import "github.com/entropyio/go-entropy/blockchain/model"

type DoneEvent struct {
	Latest *model.Header
}
type StartEvent struct{}
type FailedEvent struct{ Err error }
