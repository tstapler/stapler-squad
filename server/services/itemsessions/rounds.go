// Package itemsessions picks among the sessions a backlog item owns.
package itemsessions

import (
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// FindSupersededSessions returns every tmux-backed (work or review role)
// ItemSession in sessions that is NOT the current round for its role — i.e.
// a strictly newer ItemSession of the same role exists for this item. This is
// the single source of truth for "which round is current," reusing
// FindMostRecentSessions' exact per-role tie-break (latest CreatedAt wins) —
// the same helper TriggerReReview itself already calls to find the prior
// review round to archive — rather than defining a second, competing notion
// of "current." findActiveWorkSession/findConfirmedLiveWorkSession answer a
// different question ("is there a still-open/still-live work session at
// all," by EndedAt/IsSessionLive, not CreatedAt) for spawnSessionAfterGates'
// concurrency guard, not "which round is current," so this deliberately does
// not reuse their tie-break. The two can disagree in theory (e.g. the
// newest-by-CreatedAt round already ended while an older one is still open) —
// archiveIfNotLive's IsSessionLive check is what actually protects a
// genuinely-attached session from being archived on such a disagreement, not
// this function agreeing with those two on "current."
// Pure — no I/O, no locking — so it's table-testable without tmux/storage.
func FindSupersededSessions(sessions []backlogdto.ItemSessionSummary) []backlogdto.ItemSessionSummary {
	currentReview, currentWork := FindMostRecentSessions(sessions)

	var superseded []backlogdto.ItemSessionSummary
	for i := range sessions {
		is := &sessions[i]
		if !instance.IsTmuxBackedSessionRole(is.Role) {
			continue
		}
		if is == currentReview || is == currentWork {
			continue
		}
		superseded = append(superseded, *is)
	}
	return superseded
}

// FindMostRecentSessions returns the most recently created review and work ItemSessions
// from the provided list. Either return value may be nil if no session of that role exists.
func FindMostRecentSessions(sessions []backlogdto.ItemSessionSummary) (reviewSession, workSession *backlogdto.ItemSessionSummary) {
	for i := range sessions {
		is := &sessions[i]
		switch is.Role {
		case instance.SessionRoleReview:
			if reviewSession == nil || is.CreatedAt.After(reviewSession.CreatedAt) {
				reviewSession = is
			}
		case instance.SessionRoleWork:
			if workSession == nil || is.CreatedAt.After(workSession.CreatedAt) {
				workSession = is
			}
		}
	}
	return
}
