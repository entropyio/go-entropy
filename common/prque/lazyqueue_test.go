package prque

import (
	"github.com/entropyio/go-entropy/common/mclock"
	"math/rand"
	"sync"
	"testing"
	"time"
)

const (
	testItems        = 1000
	testPriorityStep = 100
	testSteps        = 1000000
	testStepPeriod   = time.Millisecond
	testQueueRefresh = time.Second
	testAvgRate      = float64(testPriorityStep) / float64(testItems) / float64(testStepPeriod)
)

type lazyItem struct {
	p, maxp int64
	last    mclock.AbsTime
	index   int
}

func testPriority(a interface{}) int64 {
	return a.(*lazyItem).p
}

func testMaxPriority(a interface{}, until mclock.AbsTime) int64 {
	i := a.(*lazyItem)
	dt := until - i.last
	i.maxp = i.p + int64(float64(dt)*testAvgRate)
	return i.maxp
}

func testSetIndex(a interface{}, i int) {
	a.(*lazyItem).index = i
}

func TestLazyQueue(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	clock := &mclock.Simulated{}
	q := NewLazyQueue(testSetIndex, testPriority, testMaxPriority, clock, testQueueRefresh)

	var (
		items  [testItems]lazyItem
		maxPri int64
	)

	for i := range items[:] {
		items[i].p = rand.Int63n(testPriorityStep * 10)
		if items[i].p > maxPri {
			maxPri = items[i].p
		}
		items[i].index = -1
		q.Push(&items[i])
	}

	var (
		lock   sync.Mutex
		wg     sync.WaitGroup
		stopCh = make(chan chan struct{})
	)
	defer wg.Wait()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-clock.After(testQueueRefresh):
				lock.Lock()
				q.Refresh()
				lock.Unlock()
			case <-stopCh:
				return
			}
		}
	}()

	for c := 0; c < testSteps; c++ {
		i := rand.Intn(testItems)
		lock.Lock()
		items[i].p += rand.Int63n(testPriorityStep*2-1) + 1
		if items[i].p > maxPri {
			maxPri = items[i].p
		}
		items[i].last = clock.Now()
		if items[i].p > items[i].maxp {
			q.Update(items[i].index)
		}
		if rand.Intn(100) == 0 {
			p := q.PopItem().(*lazyItem)
			if p.p != maxPri {
				lock.Unlock()
				close(stopCh)
				t.Fatalf("incorrect item (best known priority %d, popped %d)", maxPri, p.p)
			}
			q.Push(p)
		}
		lock.Unlock()
		clock.Run(testStepPeriod)
		clock.WaitForTimers(1)
	}

	close(stopCh)
}
