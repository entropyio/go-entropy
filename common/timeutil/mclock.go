package timeutil

import (
	"time"

	"github.com/aristanetworks/goarista/monotime"
)

// AbsTime represents absolute monotonic time.
type AbsTime time.Duration

// Now returns the current absolute monotonic time.
func Now() AbsTime {
	return AbsTime(monotime.Now())
}

// Add returns t + d.
func (t AbsTime) Add(d time.Duration) AbsTime {
	return t + AbsTime(d)
}

// Clock interface makes it possible to replace the monotonic system clock with
// a simulated clock.
type Clock interface {
	Now() AbsTime
	Sleep(time.Duration)
	After(time.Duration) <-chan time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer represents a cancellable event returned by AfterFunc
type Timer interface {
	Stop() bool
}

// System implements Clock using the system clock.
type System struct{}

// Now returns the current monotonic time.
func (System) Now() AbsTime {
	return AbsTime(monotime.Now())
}

// Sleep blocks for the given duration.
func (System) Sleep(d time.Duration) {
	time.Sleep(d)
}

// After returns a channel which receives the current time after d has elapsed.
func (System) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// AfterFunc runs f on a new goroutine after the duration has elapsed.
func (System) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}
