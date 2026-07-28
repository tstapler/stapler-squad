package server

import (
	"errors"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// fakeInstanceDataLister is a minimal instanceDataLister for testing
// buildSessionExistenceLookup without a real *session.Storage (and without
// implementing the full session.Repository interface).
type fakeInstanceDataLister struct {
	data []session.InstanceData
	err  error
}

func (f *fakeInstanceDataLister) ListInstanceData() ([]session.InstanceData, error) {
	return f.data, f.err
}

// Test_buildSessionExistenceLookup_should_ReturnNil_When_UptimeGateNotElapsed covers
// case (a): before pruneOrphanedMinUptime has elapsed since startedAt, the lookup
// must return nil (skip this prune pass) regardless of what storage would return —
// confirmed here by using a lister that would otherwise report sessions, to prove
// the gate short-circuits before ListInstanceData is even consulted.
func Test_buildSessionExistenceLookup_should_ReturnNil_When_UptimeGateNotElapsed(t *testing.T) {
	lister := &fakeInstanceDataLister{
		data: []session.InstanceData{{UUID: "abc-123", Title: "some-session"}},
	}
	// startedAt "now" means time.Since(startedAt) is ~0, well under the 5-minute gate.
	lookup := buildSessionExistenceLookup(lister, time.Now())

	got := lookup()
	if got != nil {
		t.Fatalf("expected nil before uptime gate elapses, got %#v", got)
	}
}

// Test_buildSessionExistenceLookup_should_ReturnPopulatedMap_When_GateElapsedAndSessionsExist
// covers case (b): once the gate has elapsed and ListInstanceData returns live
// sessions, the lookup must return a map keyed by both stable ID (UUID when set)
// and Title.
func Test_buildSessionExistenceLookup_should_ReturnPopulatedMap_When_GateElapsedAndSessionsExist(t *testing.T) {
	lister := &fakeInstanceDataLister{
		data: []session.InstanceData{
			{UUID: "uuid-1", Title: "session-one"},
			{Title: "session-two"}, // no UUID: GetStableID() falls back to Title
		},
	}
	startedAt := time.Now().Add(-2 * pruneOrphanedMinUptime)
	lookup := buildSessionExistenceLookup(lister, startedAt)

	got := lookup()
	if got == nil {
		t.Fatal("expected a non-nil map once the uptime gate has elapsed")
	}

	wantKeys := []string{"uuid-1", "session-one", "session-two"}
	for _, k := range wantKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("expected key %q in existence map, got %#v", k, got)
		}
	}
	if len(got) != len(wantKeys) {
		t.Errorf("expected exactly %d keys, got %d: %#v", len(wantKeys), len(got), got)
	}
}

// Test_buildSessionExistenceLookup_should_ReturnNil_When_ListInstanceDataErrors covers
// case (c): once the gate has elapsed, if ListInstanceData errors, the lookup must
// fail closed and return nil (skip this prune pass) rather than an empty map — an
// empty map would tell the pruning sweep "nothing exists, prune everything," which
// would be a data-loss bug for a merely transient storage error.
func Test_buildSessionExistenceLookup_should_ReturnNil_When_ListInstanceDataErrors(t *testing.T) {
	lister := &fakeInstanceDataLister{
		err: errors.New("boom: simulated storage failure"),
	}
	startedAt := time.Now().Add(-2 * pruneOrphanedMinUptime)
	lookup := buildSessionExistenceLookup(lister, startedAt)

	got := lookup()
	if got != nil {
		t.Fatalf("expected nil (fail-closed) when ListInstanceData errors, got %#v", got)
	}
}

// Test_buildSessionExistenceLookup_should_ReturnEmptyNonNilMap_When_GateElapsedAndNoSessions
// covers case (d): once the gate has elapsed, zero live sessions must produce a
// genuinely empty *non-nil* map — distinct from case (a)'s nil "skip" signal. This is
// the safety-critical distinction the whole closure exists to preserve: nil means
// "don't prune this pass," a non-nil empty map means "authoritatively nothing exists,
// prune everything session-scoped."
func Test_buildSessionExistenceLookup_should_ReturnEmptyNonNilMap_When_GateElapsedAndNoSessions(t *testing.T) {
	lister := &fakeInstanceDataLister{
		data: []session.InstanceData{},
	}
	startedAt := time.Now().Add(-2 * pruneOrphanedMinUptime)
	lookup := buildSessionExistenceLookup(lister, startedAt)

	got := lookup()
	if got == nil {
		t.Fatal("expected a non-nil empty map when there are zero live sessions, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected an empty map, got %#v", got)
	}
}
