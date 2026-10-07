package services

import (
	"sync"
	"time"
)

// GuardOutcome is the result of sessionNudgeGuard.TryBegin.
type GuardOutcome int

const (
	// GuardOK means the caller owns the session and must call the returned release.
	GuardOK GuardOutcome = iota
	// GuardBusy means another delivery is in flight for the session.
	GuardBusy
	// GuardDuplicate means the same signature was delivered (or just failed) recently.
	GuardDuplicate
)

const (
	nudgeDuplicateWindow = 60 * time.Second
	// A failed write may have partially reached the PTY; a short cooldown stops an instant retry.
	nudgeFailureCooldown = 10 * time.Second
	nudgeRecordTTL       = 10 * time.Minute
	nudgeMaxRecords      = 1024
)

type nudgeRecord struct {
	sig    string
	at     time.Time
	window time.Duration
}

// sessionNudgeGuard serializes PTY nudges per session and suppresses repeats
// of the same reason signature, shared by the manual nudge RPC and PR-fix
// auto-steer. State is in memory only: after a server restart the duplicate
// window is empty, so a nudge delivered just before a restart can repeat once
// (accepted for a single-user tool).
//
// A mutex rather than atomics: the in-flight check, duplicate-window check and
// in-flight mark must be one atomic step across two maps, and the critical
// section is a few map operations with no I/O. The zero value is ready to use.
type sessionNudgeGuard struct {
	mu       sync.Mutex
	inflight map[string]struct{}
	last     map[string]nudgeRecord
	now      func() time.Time // injected in tests; nil means time.Now
}

func (g *sessionNudgeGuard) clock() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

// TryBegin atomically claims id for a delivery of sig. On GuardOK the caller
// must call release exactly once: release(true) records the delivery for the
// 60s duplicate window, release(false) records only a 10s failure cooldown.
// For GuardBusy and GuardDuplicate, release is a safe no-op. id is
// Instance.GetStableID() on every call path so the manual and automatic
// callers share one key.
func (g *sessionNudgeGuard) TryBegin(id, sig string) (release func(success bool), outcome GuardOutcome) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.clock()
	g.evictLocked(now)

	if _, busy := g.inflight[id]; busy {
		return func(bool) {}, GuardBusy
	}
	if rec, ok := g.last[id]; ok && rec.sig == sig && now.Sub(rec.at) < rec.window {
		return func(bool) {}, GuardDuplicate
	}

	if g.inflight == nil {
		g.inflight = make(map[string]struct{})
	}
	g.inflight[id] = struct{}{}

	var once sync.Once
	return func(success bool) {
		once.Do(func() { g.finish(id, sig, success) })
	}, GuardOK
}

// abandon releases id's in-flight claim without recording anything, for a
// claim that never reached the PTY (not ready). The holder must not also call release.
func (g *sessionNudgeGuard) abandon(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inflight, id)
}

func (g *sessionNudgeGuard) finish(id, sig string, success bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.inflight, id)
	window := nudgeFailureCooldown
	if success {
		window = nudgeDuplicateWindow
	}
	if g.last == nil {
		g.last = make(map[string]nudgeRecord)
	}
	g.last[id] = nudgeRecord{sig: sig, at: g.clock(), window: window}
}

// evictLocked drops records past their TTL, then the oldest records beyond the hard cap.
func (g *sessionNudgeGuard) evictLocked(now time.Time) {
	for id, rec := range g.last {
		if now.Sub(rec.at) > nudgeRecordTTL {
			delete(g.last, id)
		}
	}
	for len(g.last) > nudgeMaxRecords {
		var oldestID string
		var oldest time.Time
		for id, rec := range g.last {
			if oldestID == "" || rec.at.Before(oldest) {
				oldestID, oldest = id, rec.at
			}
		}
		delete(g.last, oldestID)
	}
}
