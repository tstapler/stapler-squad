// Package fsevents is a pure-Go (no cgo) binding to macOS FSEvents, loaded via
// purego. The stream itself is darwin-only; types, event decoding and rescan
// dispatch are platform-neutral so they can be tested anywhere.
package fsevents

import "time"

// EventID is a monotonically increasing FSEvents cursor. A saved ID is only
// valid for the same volume UUID (see VolumeUUID).
type EventID = uint64

// SinceNow starts a stream with no replay (kFSEventStreamEventIdSinceNow).
const SinceNow EventID = 0xFFFFFFFFFFFFFFFF

// EventFlags are the kFSEventStreamEventFlag* bits carried on each event.
type EventFlags uint32

const (
	MustScanSubDirs EventFlags = 0x00000001
	UserDropped     EventFlags = 0x00000002
	KernelDropped   EventFlags = 0x00000004
	EventIdsWrapped EventFlags = 0x00000008
	HistoryDone     EventFlags = 0x00000010
	RootChanged     EventFlags = 0x00000020
	Mount           EventFlags = 0x00000040
	Unmount         EventFlags = 0x00000080
	ItemCreated     EventFlags = 0x00000100
	ItemRemoved     EventFlags = 0x00000200
	ItemRenamed     EventFlags = 0x00000800
	ItemModified    EventFlags = 0x00001000
	ItemIsFile      EventFlags = 0x00010000
	ItemIsDir       EventFlags = 0x00020000
)

// RescanFlags mark events after which incremental state is untrustworthy and
// the caller must do a full rescan.
const RescanFlags = MustScanSubDirs | UserDropped | KernelDropped | EventIdsWrapped | RootChanged

// NeedsRescan reports whether f carries any flag in RescanFlags.
func (f EventFlags) NeedsRescan() bool { return f&RescanFlags != 0 }

// Event is one entry of an FSEvents batch.
type Event struct {
	Path    string
	Flags   EventFlags
	EventID EventID
}

// Config configures a Stream.
type Config struct {
	Paths     []string
	SinceWhen EventID       // SinceNow or a saved EventID to replay from
	Latency   time.Duration // coalesce window passed to FSEvents
	// OnEvents receives every batch. It runs on the run-loop thread and must
	// not block.
	OnEvents func([]Event)
	// OnRescan is called once per event carrying a RescanFlags bit, in
	// addition to OnEvents. Never swallowed.
	OnRescan func(Event)
}

// dispatch delivers a decoded batch to the config's callbacks.
func (c *Config) dispatch(evs []Event) {
	if len(evs) == 0 {
		return
	}
	if c.OnEvents != nil {
		c.OnEvents(evs)
	}
	if c.OnRescan != nil {
		for _, e := range evs {
			if e.Flags.NeedsRescan() {
				c.OnRescan(e)
			}
		}
	}
}
