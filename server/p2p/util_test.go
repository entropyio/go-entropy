package p2p

import (
	"testing"
	"time"
)

func TestExpHeap(t *testing.T) {
	var h expHeap

	var (
		basetime = time.Unix(4000, 0)
		exptimeA = basetime.Add(2 * time.Second)
		exptimeB = basetime.Add(3 * time.Second)
		exptimeC = basetime.Add(4 * time.Second)
	)
	h.add("a", exptimeA)
	h.add("b", exptimeB)
	h.add("c", exptimeC)

	if !h.nextExpiry().Equal(exptimeA) {
		t.Fatal("wrong nextExpiry")
	}
	if !h.contains("a") || !h.contains("b") || !h.contains("c") {
		t.Fatal("heap doesn't contain all live items")
	}

	h.expire(exptimeA.Add(1))
	if !h.nextExpiry().Equal(exptimeB) {
		t.Fatal("wrong nextExpiry")
	}
	if h.contains("a") {
		t.Fatal("heap contains a even though it has already expired")
	}
	if !h.contains("b") || !h.contains("c") {
		t.Fatal("heap doesn't contain all live items")
	}
}
