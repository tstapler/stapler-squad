package deliverygate

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

const (
	// refreshMinInterval is the minimum gap between refresh attempts (success or failure).
	refreshMinInterval = 5 * time.Second
	// refreshTimeout abandons a storage listing that does not return.
	refreshTimeout = 3 * time.Second
	// maxMissedKeys bounds the set used to detect "unresolved but exists" index defects.
	maxMissedKeys = 1024
)

// InstanceDataLister is the refresh-only storage port (session.InstanceStore subset).
type InstanceDataLister interface {
	ListInstanceData() ([]session.InstanceData, error)
}

// Resolution is the resolver's answer for one event or session identity.
type Resolution struct {
	Visibility Visibility
	Kind       HiddenKind
	UUID       string
	Title      string
}

// Resolver reads the VisibilityIndex. It never performs synchronous I/O: a miss
// returns Unresolved and schedules a bounded, singleflight, rate-limited
// background refresh from storage.
type Resolver struct {
	index   *VisibilityIndex
	metrics *Metrics
	now     Clock

	lister  atomic.Pointer[listerHolder]
	timeout func() <-chan time.Time // injectable; nil = real timer

	refreshing  atomic.Bool
	lastAttempt atomic.Int64 // unix nanos; 0 = never
	wg          sync.WaitGroup
	stopped     atomic.Bool

	missMu sync.Mutex // guards missed only; calls nothing while held
	missed map[string]struct{}
}

type listerHolder struct{ l InstanceDataLister }

func newResolver(index *VisibilityIndex, m *Metrics, now Clock) *Resolver {
	return &Resolver{index: index, metrics: m, now: now, missed: map[string]struct{}{}}
}

// SetLister wires the refresh backstop (late-bound: storage exists after the bus).
func (r *Resolver) SetLister(l InstanceDataLister) { r.lister.Store(&listerHolder{l: l}) }

// Resolve answers for an event's session slot and metadata. Order: SessionID
// against the index, then metadata item_id against the index (a PermanentlyFailed
// notification is published via Notify(inst.UUID, ...), which stamps item_id with
// the session UUID), then the positive NotASession rules.
func (r *Resolver) Resolve(sessionID string, metadata map[string]string) Resolution {
	if res, ok := r.fromIndex(sessionID); ok {
		return res
	}
	itemID := metadata[events.MetadataKeyItemID]
	if res, ok := r.fromIndex(itemID); ok {
		return res
	}
	if sessionID == "" || itemID != "" || IsSystemID(sessionID) {
		return Resolution{Visibility: VisibilityNotASession}
	}
	r.noteMiss(sessionID)
	return Resolution{Visibility: VisibilityUnresolved, Kind: KindUnresolved}
}

func (r *Resolver) fromIndex(key string) (Resolution, bool) {
	e, ok := r.index.Lookup(key)
	if !ok {
		return Resolution{}, false
	}
	if e.Hidden {
		return Resolution{Visibility: VisibilityHidden, Kind: e.Kind, UUID: e.UUID, Title: e.Title}, true
	}
	return Resolution{Visibility: VisibilityVisible, UUID: e.UUID, Title: e.Title}, true
}

func (r *Resolver) noteMiss(key string) {
	r.metrics.Add(CounterIndexMiss)
	r.missMu.Lock()
	if len(r.missed) < maxMissedKeys {
		r.missed[key] = struct{}{}
	}
	r.missMu.Unlock()
	r.maybeRefresh()
}

func (r *Resolver) maybeRefresh() {
	h := r.lister.Load()
	if h == nil || h.l == nil || r.stopped.Load() {
		return
	}
	if !r.refreshing.CompareAndSwap(false, true) {
		return // one refresh in flight (singleflight)
	}
	now := r.now().UnixNano()
	if last := r.lastAttempt.Load(); last != 0 && now-last < int64(refreshMinInterval) {
		r.refreshing.Store(false)
		r.metrics.Add(CounterIndexRefresh, RefreshSkippedMinInterval)
		return
	}
	r.lastAttempt.Store(now)
	r.metrics.Add(CounterIndexRefresh, RefreshStarted)
	r.wg.Add(1)
	go r.runRefresh(h.l)
}

type listResult struct {
	data []session.InstanceData
	err  error
}

func (r *Resolver) runRefresh(l InstanceDataLister) {
	defer r.wg.Done()
	defer r.refreshing.Store(false)

	out := make(chan listResult, 1) // buffered: an abandoned lister never blocks on send
	go func() {
		d, err := l.ListInstanceData()
		out <- listResult{d, err}
	}()

	var timeout <-chan time.Time
	if r.timeout != nil {
		timeout = r.timeout()
	} else {
		t := time.NewTimer(refreshTimeout)
		defer t.Stop()
		timeout = t.C
	}
	select {
	case res := <-out:
		if res.err != nil {
			r.metrics.Add(CounterIndexRefresh, RefreshError)
			return
		}
		r.apply(res.data)
		r.metrics.Add(CounterIndexRefresh, RefreshOK)
	case <-timeout:
		r.metrics.Add(CounterIndexRefresh, RefreshTimeout)
	}
}

// apply merges listed sessions into the index and flags any key that earlier
// missed but exists in storage: that is an index defect, not an accepted leak.
func (r *Resolver) apply(data []session.InstanceData) {
	entries := make([]Entry, 0, len(data))
	r.missMu.Lock()
	for _, d := range data {
		e := entryFromData(d)
		entries = append(entries, e)
		for _, k := range []string{e.UUID, e.Title, e.TmuxName} {
			if _, ok := r.missed[k]; !ok || k == "" {
				continue
			}
			delete(r.missed, k)
			if _, known := r.index.Lookup(k); !known {
				r.metrics.Add(CounterResolvedLater)
			}
		}
	}
	r.missMu.Unlock()
	r.index.UpsertMany(entries)
}

// Wait joins in-flight refresh goroutines and stops new ones (shutdown, tests).
func (r *Resolver) Wait() {
	r.stopped.Store(true)
	r.wg.Wait()
}
