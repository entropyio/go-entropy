package bloombits

import (
	"bytes"
	"math/big"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests that the scheduler can deduplicate and forward retrieval requests to
// underlying fetchers and serve responses back, irrelevant of the concurrency
// of the requesting clients or serving data fetchers.
func TestSchedulerSingleClientSingleFetcher(t *testing.T) { testScheduler(t, 1, 1, 5000) }
func TestSchedulerSingleClientMultiFetcher(t *testing.T)  { testScheduler(t, 1, 10, 5000) }
func TestSchedulerMultiClientSingleFetcher(t *testing.T)  { testScheduler(t, 10, 1, 5000) }
func TestSchedulerMultiClientMultiFetcher(t *testing.T)   { testScheduler(t, 10, 10, 5000) }

func testScheduler(t *testing.T, clients int, fetchers int, requests int) {
	f := newScheduler(0)

	// Create a batch of handler goroutines that respond to bloom bit requests and
	// deliver them to the scheduler.
	var fetchPend sync.WaitGroup
	fetchPend.Add(fetchers)
	defer fetchPend.Wait()

	fetch := make(chan *request, 16)
	defer close(fetch)

	var delivered uint32
	for i := 0; i < fetchers; i++ {
		go func() {
			defer fetchPend.Done()

			for req := range fetch {
				time.Sleep(time.Duration(rand.Intn(int(100 * time.Microsecond))))
				atomic.AddUint32(&delivered, 1)

				f.deliver([]uint64{
					req.section + uint64(requests), // Non-requested data (ensure it doesn't go out of bounds)
					req.section,                    // Requested data
					req.section,                    // Duplicated data (ensure it doesn't double close anything)
				}, [][]byte{
					{},
					new(big.Int).SetUint64(req.section).Bytes(),
					new(big.Int).SetUint64(req.section).Bytes(),
				})
			}
		}()
	}
	// Start a batch of goroutines to concurrently run scheduling tasks
	quit := make(chan struct{})

	var pend sync.WaitGroup
	pend.Add(clients)

	for i := 0; i < clients; i++ {
		go func() {
			defer pend.Done()

			in := make(chan uint64, 16)
			out := make(chan []byte, 16)

			f.run(in, fetch, out, quit, &pend)

			go func() {
				for j := 0; j < requests; j++ {
					in <- uint64(j)
				}
				close(in)
			}()

			for j := 0; j < requests; j++ {
				bits := <-out
				if want := new(big.Int).SetUint64(uint64(j)).Bytes(); !bytes.Equal(bits, want) {
					t.Errorf("vector %d: delivered content mismatch: have %x, want %x", j, bits, want)
				}
			}
		}()
	}
	pend.Wait()

	if have := atomic.LoadUint32(&delivered); int(have) != requests {
		t.Errorf("request count mismatch: have %v, want %v", have, requests)
	}
}
