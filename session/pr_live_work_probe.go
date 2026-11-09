package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// LiveSnapshotSource returns the lock-free Snapshot() of a session that is live in
// this process; ok is false when it is not tracked live. Implemented by the server
// layer over the live instance registry.
type LiveSnapshotSource interface {
	LiveSnapshot(sessionUUID string) (snap *InstanceSnapshot, ok bool)
}

// InstanceDataReader is the storage read the probe falls back to for a session
// that is not live. *Storage implements it.
type InstanceDataReader interface {
	FindInstanceDataByID(id string) (*InstanceData, error)
}

// SnapshotLiveWorkProbe is the real LiveWorkProbe. Contract (owner-approved):
//   - (false, nil) for an absent, deleted, archived or ended session;
//   - an unreadable session (transient storage failure) is treated as live, but only
//     for liveWorkErrorHoldCap from the first failure; after that it is not live,
//     counted and logged. The probe never returns an error itself, so every
//     consumer sees the same capped answer.
//
// One instance is shared by the lifecycle listener, BacklogService and the gate.
type SnapshotLiveWorkProbe struct {
	live  LiveSnapshotSource
	store InstanceDataReader

	mu       sync.Mutex
	errSince map[string]time.Time
}

// NewSnapshotLiveWorkProbe builds the probe; both sources are required.
func NewSnapshotLiveWorkProbe(live LiveSnapshotSource, store InstanceDataReader) *SnapshotLiveWorkProbe {
	return &SnapshotLiveWorkProbe{live: live, store: store, errSince: map[string]time.Time{}}
}

var _ LiveWorkProbe = (*SnapshotLiveWorkProbe)(nil)

// IsLive implements LiveWorkProbe.
func (p *SnapshotLiveWorkProbe) IsLive(_ context.Context, sessionUUID string, now time.Time) (bool, error) {
	if sessionUUID == "" {
		return false, nil
	}
	if snap, ok := p.live.LiveSnapshot(sessionUUID); ok && snap != nil {
		p.clearErrorHold(sessionUUID)
		live, capped := evaluateLiveWork(LiveWorkFactsFromSnapshot(snap), now, LiveWorkIdleWindow)
		if capped {
			liveWorkMetrics.count(LiveWorkResultCapWorking)
			log.WarningLog().Printf("[LiveWork] session=%s Working with no meaningful change since %s: not live (%s cap)",
				sessionUUID, snap.LastMeaningfulChangeAt.Format(time.RFC3339), liveWorkWorkingCap)
		}
		return live, nil
	}
	data, err := p.store.FindInstanceDataByID(sessionUUID)
	switch {
	case errors.Is(err, ErrInstanceDataNotFound):
		p.clearErrorHold(sessionUUID)
		return false, nil
	case err != nil:
		return p.heldOnError(sessionUUID, now, err), nil
	}
	p.clearErrorHold(sessionUUID)
	return IsLiveWork(liveWorkFactsFromData(data), now, LiveWorkIdleWindow), nil
}

// heldOnError applies the error-hold cap: live until liveWorkErrorHoldCap after the
// first consecutive failure, then not live.
func (p *SnapshotLiveWorkProbe) heldOnError(sessionUUID string, now time.Time, err error) bool {
	p.mu.Lock()
	since, seen := p.errSince[sessionUUID]
	if !seen {
		since = now
		p.errSince[sessionUUID] = now
	}
	p.mu.Unlock()
	if now.Sub(since) >= liveWorkErrorHoldCap {
		liveWorkMetrics.count(LiveWorkResultCapError)
		log.WarningLog().Printf("[LiveWork] session=%s probe failing since %s (%v): not live (%s error-hold cap)",
			sessionUUID, since.Format(time.RFC3339), err, liveWorkErrorHoldCap)
		return false
	}
	liveWorkMetrics.count(LiveWorkResultErrorHeld)
	log.WarningLog().Printf("[LiveWork] session=%s probe error, treated as live: %v", sessionUUID, err)
	return true
}

func (p *SnapshotLiveWorkProbe) clearErrorHold(sessionUUID string) {
	p.mu.Lock()
	delete(p.errSince, sessionUUID)
	p.mu.Unlock()
}

// LiveWorkFactsFromSnapshot maps a Snapshot() to IsLiveWork's inputs. Ended means
// status Paused, Stopped or Hibernated, or ArchivedAt set. Working is an unended
// Creating or Active session. TurnEnded means a turn end was recorded and nothing
// meaningful changed after it: RecordTurnEnd touches LastMeaningfulChangeAt to the
// same instant, so any later output, approval or status change makes it Working again.
func LiveWorkFactsFromSnapshot(s *InstanceSnapshot) LiveWorkFacts {
	unended := s.ArchivedAt == nil && (s.Status == Creating || s.Status == Active)
	lmca := s.LastMeaningfulChangeAt
	if lmca.IsZero() {
		lmca = s.CreatedAt
	}
	turnEnded := !s.TurnEnded.At.IsZero() && !s.LastMeaningfulChangeAt.After(s.TurnEnded.At)
	return LiveWorkFacts{Unended: unended, Working: unended, TurnEnded: turnEnded, LastMeaningfulChangeAt: lmca}
}

// liveWorkFactsFromData is the persisted fallback for a session that is not live
// in this process: not Working (nothing is running to be mid-turn), so only a
// recent output keeps it live.
func liveWorkFactsFromData(d *InstanceData) LiveWorkFacts {
	unended := d.ArchivedAt == nil && (d.Status == Creating || d.Status == Active)
	return LiveWorkFacts{Unended: unended, LastMeaningfulChangeAt: d.LastMeaningfulOutput}
}

// CurrentRoundWorkSessions is the live-work subject of an item: its newest work
// session. (The PR round walk skips sessions with no link or tombstone, which is
// right for attributing PRs and wrong for liveness: the agent still working has
// usually not opened a PR yet.)
func CurrentRoundWorkSessions(sessions []ItemSessionSummary) []ItemSessionSummary {
	var newest *ItemSessionSummary
	for i := range sessions {
		s := sessions[i]
		if s.Role != SessionRoleWork || s.SessionUUID == "" {
			continue
		}
		if newest == nil || workSessionNewer(s, *newest) {
			newest = &sessions[i]
		}
	}
	if newest == nil {
		return nil
	}
	return []ItemSessionSummary{*newest}
}

// NewestWorkSession returns the live-work subject of an item as a slice: its newest
// work session, or none.
func NewestWorkSession(sessions []ItemSessionSummary) []ItemSessionSummary {
	return CurrentRoundWorkSessions(sessions)
}

// SplitLiveCurrentRound separates the sessions terminal-item archival may archive
// and pane-kill from the live current-round work session it must leave alone
// (RULE L). A nil probe, or a flag-off caller passing nil, skips nothing. A probe
// error counts as live. skippedLive tells the sweep to leave the item unsettled
// so it retries every pass.
func SplitLiveCurrentRound(ctx context.Context, probe LiveWorkProbe, now time.Time, sessions []ItemSessionSummary) (archivable []ItemSessionSummary, skippedLive bool) {
	if probe == nil {
		return sessions, false
	}
	live := map[string]bool{}
	for _, s := range CurrentRoundWorkSessions(sessions) {
		isLive, err := probe.IsLive(ctx, s.SessionUUID, now)
		if err != nil {
			log.WarningLog().Printf("[LiveWork] RULE L probe error session=%s, treated as live: %v", s.SessionUUID, err)
			isLive = true
		}
		live[s.SessionUUID] = isLive
	}
	for _, s := range sessions {
		if live[s.SessionUUID] {
			skippedLive = true
			continue
		}
		archivable = append(archivable, s)
	}
	return archivable, skippedLive
}
