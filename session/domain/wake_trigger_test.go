package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wantWakeTransitions spells out ADR-006 rule 2's table independently of the
// production map so a typo in either shows up as a diff.
var wantWakeTransitions = map[[2]WakeStatus]bool{
	{WakeStatusPending, WakeStatusClaimed}:       true,
	{WakeStatusPending, WakeStatusMissed}:        true,
	{WakeStatusPending, WakeStatusCancelled}:     true,
	{WakeStatusPending, WakeStatusSkipped}:       true,
	{WakeStatusClaimed, WakeStatusDelivered}:     true,
	{WakeStatusClaimed, WakeStatusPending}:       true,
	{WakeStatusClaimed, WakeStatusNeedsRevive}:   true,
	{WakeStatusClaimed, WakeStatusMissed}:        true,
	{WakeStatusClaimed, WakeStatusSkipped}:       true,
	{WakeStatusClaimed, WakeStatusCancelled}:     true,
	{WakeStatusNeedsRevive, WakeStatusClaimed}:   true,
	{WakeStatusNeedsRevive, WakeStatusMissed}:    true,
	{WakeStatusNeedsRevive, WakeStatusSkipped}:   true,
	{WakeStatusNeedsRevive, WakeStatusCancelled}: true,
}

func TestWakeStatus_CanTransitionTo_should_MatchADRTable_When_EveryPairIsChecked(t *testing.T) {
	t.Parallel()
	for _, from := range AllWakeStatuses() {
		for _, to := range AllWakeStatuses() {
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, wantWakeTransitions[[2]WakeStatus{from, to}], from.CanTransitionTo(to))
			})
		}
	}
}

func TestWakeStatus_IsTerminal_should_HaveNoOutgoingEdges_When_Terminal(t *testing.T) {
	t.Parallel()
	for _, s := range AllWakeStatuses() {
		hasOut := false
		for _, to := range AllWakeStatuses() {
			hasOut = hasOut || s.CanTransitionTo(to)
		}
		assert.Equal(t, !hasOut, s.IsTerminal(), string(s))
	}
}

func TestWakeStatusesReaching_should_ListSources_When_TargetGiven(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []WakeStatus{WakeStatusPending, WakeStatusClaimed, WakeStatusNeedsRevive}, WakeStatusesReaching(WakeStatusCancelled))
	assert.Equal(t, []WakeStatus{WakeStatusClaimed}, WakeStatusesReaching(WakeStatusDelivered))
	assert.Empty(t, WakeStatusesReaching("bogus"))
}

func TestWakeSpec_RoundTrip_should_PreserveSpec_When_MarshaledAndParsed(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		spec WakeSpec
	}{
		{"at", AtSpec{At: at}},
		{"cron", CronSpec{Expr: "CRON_TZ=America/Chicago */30 * * * *"}},
		{"backlog_event", BacklogEventSpec{ItemID: "i1", Kind: "verdict_recorded"}},
		{"pr_event", PREventSpec{Repo: "o/r", Number: 7, Kind: "merged"}},
		{"ci_event", CIEventSpec{Repo: "o/r", Number: 7, Kind: "checks_passed"}},
		{"session_state", SessionStateSpec{SessionUUID: "u", State: "stopped"}},
		{"guidance_answered", GuidanceAnsweredSpec{GuidanceRequestID: "g1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw, err := MarshalWakeSpec(tt.spec)
			require.NoError(t, err)
			got, err := ParseWakeSpec(tt.spec.TriggerType(), raw)
			require.NoError(t, err)
			assert.Equal(t, tt.spec, got)
		})
	}
}

func TestWakeSpec_Validate_should_Reject_When_FieldsMissing(t *testing.T) {
	t.Parallel()
	for name, spec := range map[string]WakeSpec{
		"at zero":    AtSpec{},
		"cron empty": CronSpec{Expr: "  "},
		"backlog":    BacklogEventSpec{ItemID: "i"},
		"pr number":  PREventSpec{Repo: "r", Kind: "k"},
		"session":    SessionStateSpec{SessionUUID: "u"},
		"guidance":   GuidanceAnsweredSpec{},
	} {
		_, err := MarshalWakeSpec(spec)
		assert.Error(t, err, name)
	}
	_, err := ParseWakeSpec("nope", "{}")
	assert.Error(t, err)
	_, err = ParseWakeSpec(WakeTriggerCron, `{"expr":""}`)
	assert.Error(t, err)
}

func TestNewWakeDedupKey_should_BeStable_When_SameInputsAndDistinct_When_AnyDiffers(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	act, err := NewWakeAction(WakeActionSteerText, "check PR")
	require.NoError(t, err)
	k1, err := NewWakeDedupKey("owner", AtSpec{At: at}, act)
	require.NoError(t, err)
	// Same instant expressed in another zone canonicalizes identically.
	k2, err := NewWakeDedupKey("owner", AtSpec{At: at.In(time.FixedZone("x", 3600))}, act)
	require.NoError(t, err)
	assert.Equal(t, k1, k2)

	other, _ := NewWakeAction(WakeActionSteerText, "other")
	k3, _ := NewWakeDedupKey("owner", AtSpec{At: at}, other)
	k4, _ := NewWakeDedupKey("owner2", AtSpec{At: at}, act)
	assert.NotEqual(t, k1, k3)
	assert.NotEqual(t, k1, k4)

	_, err = NewWakeDedupKey("", AtSpec{At: at}, act)
	assert.Error(t, err)
}

func TestWakeDedupKey_Tombstone_should_AppendID_When_Terminal(t *testing.T) {
	t.Parallel()
	k := WakeDedupKey("abc")
	assert.False(t, k.IsTombstone())
	assert.Equal(t, WakeDedupKey("abc#42"), k.Tombstone("42"))
	assert.True(t, k.Tombstone("42").IsTombstone())
}

func TestNewWakeAction_should_Reject_When_KindOrPayloadInvalid(t *testing.T) {
	t.Parallel()
	_, err := NewWakeAction("bogus", "x")
	assert.Error(t, err)
	_, err = NewWakeAction(WakeActionNotifyOperator, "  ")
	assert.Error(t, err)
	a, err := NewWakeAction(WakeActionNotifyOperator, "remind")
	require.NoError(t, err)
	assert.Equal(t, WakeActionNotifyOperator, a.Kind)
}

func TestWakeTriggerType_should_ClassifyTimeAndRecurring(t *testing.T) {
	t.Parallel()
	assert.True(t, WakeTriggerAt.IsTimeBased())
	assert.True(t, WakeTriggerCron.IsRecurring())
	assert.False(t, WakeTriggerBacklogEvent.IsTimeBased())
	assert.Len(t, AllWakeTriggerTypes(), 7)
	for _, tt := range AllWakeTriggerTypes() {
		assert.True(t, tt.IsValid())
	}
	assert.False(t, WakeTriggerType("x").IsValid())
}
