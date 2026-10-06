package a

import (
	"testing"
	"time"
	. "time"
	tm "time"
)

type napper struct{}

func (napper) Sleep(d time.Duration) {}

func TestSleeps(t *testing.T) {
	time.Sleep(time.Millisecond) // want `time.Sleep in a test`
	tm.Sleep(time.Millisecond)   // want `time.Sleep in a test`
	Sleep(time.Millisecond)      // want `time.Sleep in a test`
	f := time.Sleep              // want `time.Sleep in a test`
	_ = f
	napper{}.Sleep(time.Millisecond) // unrelated Sleep method: ok
}

func TestNolint(t *testing.T) {
	time.Sleep(time.Millisecond) //nolint:notimesleeptest waits on real subprocess exit
	//nolint:notimesleeptest kernel timer granularity under test
	time.Sleep(time.Millisecond)
	time.Sleep(time.Millisecond) /* want `requires a reason` */ //nolint:notimesleeptest
	_ = t
	time.Sleep(time.Millisecond) /* want `time.Sleep in a test` */ //nolint:otherlinter
}
