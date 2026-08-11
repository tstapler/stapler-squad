// Package memorytest provides test doubles for the memory package.
package memorytest

import "sync"

// FakeReader is a test double for memory.Reader that returns configurable values.
type FakeReader struct {
	SystemPct    float64
	SystemPctErr error
	RSSBySession map[string]int64

	mu       sync.Mutex
	sysCalls int
	rssCalls int
}

func (f *FakeReader) SystemMemoryPct() (float64, error) {
	f.mu.Lock()
	f.sysCalls++
	f.mu.Unlock()
	if f.SystemPctErr != nil {
		return 0, f.SystemPctErr
	}
	return f.SystemPct, nil
}

func (f *FakeReader) SessionRSSMB(name string) (int64, error) {
	f.mu.Lock()
	f.rssCalls++
	f.mu.Unlock()
	if f.RSSBySession != nil {
		return f.RSSBySession[name], nil
	}
	return 0, nil
}

func (f *FakeReader) GetSystemMemoryCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sysCalls
}

func (f *FakeReader) GetSessionRSSCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rssCalls
}
