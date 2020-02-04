package p2p

import (
	"container/heap"
	"time"
)

// expHeap tracks strings and their expiry time.
type expHeap []expItem

// expItem is an entry in addrHistory.
type expItem struct {
	item string
	exp  time.Time
}

// nextExpiry returns the next expiry time.
func (h *expHeap) nextExpiry() time.Time {
	return (*h)[0].exp
}

// add adds an item and sets its expiry time.
func (h *expHeap) add(item string, exp time.Time) {
	heap.Push(h, expItem{item, exp})
}

// contains checks whether an item is present.
func (h expHeap) contains(item string) bool {
	for _, v := range h {
		if v.item == item {
			return true
		}
	}
	return false
}

// expire removes items with expiry time before 'now'.
func (h *expHeap) expire(now time.Time) {
	for h.Len() > 0 && h.nextExpiry().Before(now) {
		heap.Pop(h)
	}
}

// heap.Interface boilerplate
func (h expHeap) Len() int            { return len(h) }
func (h expHeap) Less(i, j int) bool  { return h[i].exp.Before(h[j].exp) }
func (h expHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *expHeap) Push(x interface{}) { *h = append(*h, x.(expItem)) }
func (h *expHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
