// Package memorytest provides test doubles for the memory package.
package memorytest

import (
	"context"
	"sync"
)

// FakeReader is a test double for memory.Reader that returns configurable values.
type FakeReader struct {
	SystemPct    float64
	SystemPctErr error
	RSSBySession map[string]int64
	RSSErr       error

	mu        sync.Mutex
	sysCalls  int
	rssCalls  int
	lastNames []string
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

// SessionsRSSMB returns configured RSS values for each requested name. Every
// call is recorded (count and the requested names) so tests can assert the
// caller batched sessions into a single call rather than one call per session.
func (f *FakeReader) SessionsRSSMB(_ context.Context, names []string) (map[string]int64, error) {
	f.mu.Lock()
	f.rssCalls++
	f.lastNames = append([]string(nil), names...)
	f.mu.Unlock()

	if f.RSSErr != nil {
		return nil, f.RSSErr
	}

	result := make(map[string]int64, len(names))
	for _, name := range names {
		if f.RSSBySession != nil {
			result[name] = f.RSSBySession[name]
			continue
		}
		result[name] = 0
	}
	return result, nil
}

func (f *FakeReader) GetSystemMemoryCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sysCalls
}

// GetSessionRSSCalls returns how many times SessionsRSSMB was called (not how
// many sessions were measured) -- useful for asserting batching behavior.
func (f *FakeReader) GetSessionRSSCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rssCalls
}

// LastRSSNames returns the session names passed to the most recent SessionsRSSMB call.
func (f *FakeReader) LastRSSNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lastNames...)
}
