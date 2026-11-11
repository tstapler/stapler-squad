package svctest

import (
	"context"
	"fmt"
	"time"
)

// mockSessionStopper implements SessionStopper for tests.
type MockSessionStopper struct {
	LiveUUIDs       map[string]bool
	KilledPaneUUIDs []string
	// KillByTitleAllowed records the allowedOwnerUUIDs of each KillTmuxSessionByTitle call.
	KillByTitleAllowed [][]string
	ArchivedUUIDs      []string
	ArchiveErrForUUID  map[string]error
	// StoppedUUIDs records every UUID passed to StopSessionByUUID.
	StoppedUUIDs []string
	// StopperErr, if non-nil, is returned by StopSessionByUUID (default nil —
	// every existing test that doesn't set it keeps today's always-succeeds
	// behavior).
	StopperErr error
	// StaleFor maps a session UUID to the "time since last meaningful output"
	// TimeSinceLastMeaningfulOutput should report for it. A UUID present in
	// LiveUUIDs but absent here reports (0, true) — live and fresh.
	StaleFor map[string]time.Duration
	// TslmoOverrideNotLive forces TimeSinceLastMeaningfulOutput to report
	// live=false for a UUID even though it's present (true) in LiveUUIDs, so
	// IsSessionLive still reports it as live. Models the real SessionService
	// implementation's rare edge case where a session is deregistered from the
	// live poller between an earlier IsSessionLive check (e.g.
	// tombstoneOrphanWorkSessions' orphan sweep) and a later
	// TimeSinceLastMeaningfulOutput call in the same request — both are backed
	// by the same FindLiveInstance lookup in production, so they normally
	// agree, but a caller enriching an error message with the progress signal
	// must still handle the disagreement gracefully rather than assume it. Nil
	// (the default) preserves the old coupled behavior for every other test.
	TslmoOverrideNotLive map[string]bool
	// OnKillTmuxPaneOnly, if set, is invoked synchronously from
	// KillTmuxPaneOnly before it records the call — lets a test observe
	// storage state at the exact moment the pane would be killed in
	// production (BUG-064: RemediateStaleWorkSession must end the
	// ItemSession row before calling KillTmuxPaneOnly, not after, so that
	// onSessionExited's already-ended guard has something to observe once
	// the (real) tmux kill asynchronously fires the exit event).
	OnKillTmuxPaneOnly func(uuid string)
	// RetryPendingUUIDs maps a session UUID to the IsRetryPending result it
	// should report. Absent (or false) means no retry pending — the default
	// for every existing test, preserving prior behavior.
	RetryPendingUUIDs map[string]bool
	// KillIneffectiveUUIDs marks a UUID whose KillTmuxPaneOnly call must NOT
	// flip LiveUUIDs to false — models a kill attempt that genuinely fails to
	// reach the process (a stuck/zombie pane, or an EndedAt set by something
	// other than a real kill). Every other UUID's default KillTmuxPaneOnly
	// behavior flips it to not-live, matching the real
	// SessionService.KillTmuxPaneOnly: Instance.KillSession's underlying tmux
	// kill-session call is synchronous, so a genuinely successful kill is
	// confirmed dead immediately, not eventually.
	KillIneffectiveUUIDs map[string]bool
}

func (m *MockSessionStopper) IsSessionLive(uuid string) bool {
	return m.LiveUUIDs[uuid]
}

func (m *MockSessionStopper) IsRetryPending(uuid string) bool {
	return m.RetryPendingUUIDs[uuid]
}

func (m *MockSessionStopper) TimeSinceLastMeaningfulOutput(uuid string) (time.Duration, bool) {
	if !m.LiveUUIDs[uuid] || m.TslmoOverrideNotLive[uuid] {
		return 0, false
	}
	return m.StaleFor[uuid], true
}

func (m *MockSessionStopper) StopSessionByUUID(_ context.Context, uuid string) error {
	m.StoppedUUIDs = append(m.StoppedUUIDs, uuid)
	return m.StopperErr
}

func (m *MockSessionStopper) KillTmuxSessionByTitle(_ context.Context, _ string, allowedOwnerUUIDs ...string) error {
	m.KillByTitleAllowed = append(m.KillByTitleAllowed, allowedOwnerUUIDs)
	return nil
}

func (m *MockSessionStopper) KillTmuxPaneOnly(_ context.Context, uuid string) error {
	if m.OnKillTmuxPaneOnly != nil {
		m.OnKillTmuxPaneOnly(uuid)
	}
	m.KilledPaneUUIDs = append(m.KilledPaneUUIDs, uuid)
	if m.KillIneffectiveUUIDs[uuid] {
		return fmt.Errorf("mockSessionStopper: kill did not reach %s (KillIneffectiveUUIDs)", uuid)
	}
	if m.LiveUUIDs != nil {
		m.LiveUUIDs[uuid] = false
	}
	return nil
}

func (m *MockSessionStopper) ArchiveSessionByUUID(_ context.Context, uuid string) error {
	if m.ArchiveErrForUUID != nil {
		if err, ok := m.ArchiveErrForUUID[uuid]; ok {
			return err
		}
	}
	m.ArchivedUUIDs = append(m.ArchivedUUIDs, uuid)
	return nil
}
