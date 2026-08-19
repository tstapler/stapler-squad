package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// createTestAnalyticsClient opens an in-memory SQLite database and runs migrations.
// The database is cleaned up automatically via t.Cleanup.
func createTestAnalyticsClient(t *testing.T) *ent.Client {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	return repo.GetEntClient()
}

// createTestServiceWithAnalytics creates a SessionService wired with an analytics ent client.
func createTestServiceWithAnalytics(t *testing.T) (*SessionService, *ent.Client) {
	t.Helper()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })
	client := createTestAnalyticsClient(t)
	svc.SetAnalyticsClient(client)
	return svc, client
}

// insertEscapeEvent creates a test escape event directly via the ent client.
func insertEscapeEvent(t *testing.T, client *ent.Client, sessionID, stage, seqType string, mangled bool) {
	t.Helper()
	insertEscapeEventAt(t, client, sessionID, stage, seqType, mangled, time.Now())
}

// insertEscapeEventAt is insertEscapeEvent with an explicit wall time, for
// tests that need to exercise the start_time/end_time boundary filters.
func insertEscapeEventAt(t *testing.T, client *ent.Client, sessionID, stage, seqType string, mangled bool, wallTime time.Time) {
	t.Helper()
	mangleType := ""
	if mangled {
		mangleType = "truncated"
	}
	_, err := client.EscapeEvent.Create().
		SetID(uuid.New().String()).
		SetSessionID(sessionID).
		SetStage(stage).
		SetSequenceType(seqType).
		SetByteLength(4).
		SetMangled(mangled).
		SetMangleType(mangleType).
		SetWallTime(wallTime).
		SetSessionSeq(1).
		Save(context.Background())
	require.NoError(t, err, "insertEscapeEventAt failed")
}

// ---------------------------------------------------------------------------
// QueryEscapeAnalytics tests
// ---------------------------------------------------------------------------

func TestQueryEscapeAnalytics_FiltersBySessionID(t *testing.T) {
	t.Parallel()
	svc, client := createTestServiceWithAnalytics(t)

	const wantSession = "session-A"
	const otherSession = "session-B"

	insertEscapeEvent(t, client, wantSession, "pty", "csi", false)
	insertEscapeEvent(t, client, wantSession, "pty", "osc", false)
	insertEscapeEvent(t, client, otherSession, "pty", "csi", false)

	req := connect.NewRequest(&sessionv1.QueryEscapeAnalyticsRequest{
		SessionId: wantSession,
	})
	resp, err := svc.QueryEscapeAnalytics(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Events, 2, "should only return events for session-A")
	for _, ev := range resp.Msg.Events {
		assert.Equal(t, wantSession, ev.SessionId)
	}
}

func TestQueryEscapeAnalytics_MangledOnlyFilter(t *testing.T) {
	t.Parallel()
	svc, client := createTestServiceWithAnalytics(t)

	const sid = "session-mangle"
	insertEscapeEvent(t, client, sid, "pty", "csi", false)
	insertEscapeEvent(t, client, sid, "pty", "csi", true)
	insertEscapeEvent(t, client, sid, "pty", "osc", true)

	req := connect.NewRequest(&sessionv1.QueryEscapeAnalyticsRequest{
		SessionId:   sid,
		MangledOnly: true,
	})
	resp, err := svc.QueryEscapeAnalytics(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Events, 2, "should return only mangled events")
	for _, ev := range resp.Msg.Events {
		assert.True(t, ev.Mangled)
	}
}

func TestQueryEscapeAnalytics_RequiresSessionID(t *testing.T) {
	t.Parallel()
	svc, _ := createTestServiceWithAnalytics(t)

	req := connect.NewRequest(&sessionv1.QueryEscapeAnalyticsRequest{})
	_, err := svc.QueryEscapeAnalytics(context.Background(), req)
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestQueryEscapeAnalytics_NoClientReturnsUnavailable(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })
	// analyticsClient intentionally not set

	req := connect.NewRequest(&sessionv1.QueryEscapeAnalyticsRequest{SessionId: "x"})
	_, err := svc.QueryEscapeAnalytics(context.Background(), req)
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

// ---------------------------------------------------------------------------
// GetEscapeAnalyticsSummary tests
// ---------------------------------------------------------------------------

func TestGetEscapeAnalyticsSummary_ReturnsHistogram(t *testing.T) {
	t.Parallel()
	svc, client := createTestServiceWithAnalytics(t)

	const sid = "session-hist"
	insertEscapeEvent(t, client, sid, "pty", "csi", false)
	insertEscapeEvent(t, client, sid, "pty", "csi", true)
	insertEscapeEvent(t, client, sid, "pty", "osc", false)

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsSummaryRequest{
		SessionId: sid,
	})
	resp, err := svc.GetEscapeAnalyticsSummary(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, int64(3), resp.Msg.TotalSequences)
	assert.Equal(t, int64(1), resp.Msg.TotalMangled)
	assert.InDelta(t, 1.0/3.0, resp.Msg.MangleRate, 1e-9)

	// Build map for easier assertions
	hist := make(map[string]*sessionv1.EscapeSequenceCount)
	for _, c := range resp.Msg.Histogram {
		hist[c.SequenceType] = c
	}
	require.Contains(t, hist, "csi")
	assert.Equal(t, int64(2), hist["csi"].Count)
	assert.Equal(t, int64(1), hist["csi"].MangledCount)

	require.Contains(t, hist, "osc")
	assert.Equal(t, int64(1), hist["osc"].Count)
	assert.Equal(t, int64(0), hist["osc"].MangledCount)
}

func TestGetEscapeAnalyticsSummary_EmptySession(t *testing.T) {
	t.Parallel()
	svc, _ := createTestServiceWithAnalytics(t)

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsSummaryRequest{
		SessionId: "empty-session",
	})
	resp, err := svc.GetEscapeAnalyticsSummary(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.Msg.TotalSequences)
	assert.Equal(t, int64(0), resp.Msg.TotalMangled)
	assert.Equal(t, 0.0, resp.Msg.MangleRate)
	assert.Empty(t, resp.Msg.Histogram)
}

func TestGetEscapeAnalyticsSummary_RequiresSessionID(t *testing.T) {
	t.Parallel()
	svc, _ := createTestServiceWithAnalytics(t)

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsSummaryRequest{})
	_, err := svc.GetEscapeAnalyticsSummary(context.Background(), req)
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// ---------------------------------------------------------------------------
// GetEscapeAnalyticsGlobalSummary tests
// ---------------------------------------------------------------------------

func TestGetEscapeAnalyticsGlobalSummary_should_ReturnUnavailable_When_AnalyticsClientNil(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })
	// analyticsClient intentionally not set

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsGlobalSummaryRequest{})
	_, err := svc.GetEscapeAnalyticsGlobalSummary(context.Background(), req)
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

func TestGetEscapeAnalyticsGlobalSummary_should_AggregateAcrossSessions_When_MultipleSessionsHaveEvents(t *testing.T) {
	t.Parallel()
	svc, client := createTestServiceWithAnalytics(t)

	const sessionA = "global-session-A"
	const sessionB = "global-session-B"
	const sessionC = "global-session-C"

	insertEscapeEvent(t, client, sessionA, "pty", "csi", false)
	insertEscapeEvent(t, client, sessionA, "pty", "csi", true)
	insertEscapeEvent(t, client, sessionB, "pty", "osc", false)
	insertEscapeEvent(t, client, sessionC, "pty", "csi", true)

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsGlobalSummaryRequest{})
	resp, err := svc.GetEscapeAnalyticsGlobalSummary(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, int64(4), resp.Msg.TotalSequences)
	assert.Equal(t, int64(2), resp.Msg.TotalMangled)
	assert.InDelta(t, 2.0/4.0, resp.Msg.MangleRate, 1e-9)

	hist := make(map[string]*sessionv1.EscapeSequenceCount)
	for _, c := range resp.Msg.Histogram {
		hist[c.SequenceType] = c
	}
	require.Contains(t, hist, "csi")
	assert.Equal(t, int64(3), hist["csi"].Count)
	assert.Equal(t, int64(2), hist["csi"].MangledCount)
	require.Contains(t, hist, "osc")
	assert.Equal(t, int64(1), hist["osc"].Count)
	assert.Equal(t, int64(0), hist["osc"].MangledCount)

	perSession := make(map[string]*sessionv1.SessionEscapeSummary)
	for _, s := range resp.Msg.PerSession {
		perSession[s.SessionId] = s
	}
	require.Len(t, perSession, 3)

	require.Contains(t, perSession, sessionA)
	assert.Equal(t, int64(2), perSession[sessionA].TotalSequences)
	assert.Equal(t, int64(1), perSession[sessionA].TotalMangled)
	assert.InDelta(t, 0.5, perSession[sessionA].MangleRate, 1e-9)

	require.Contains(t, perSession, sessionB)
	assert.Equal(t, int64(1), perSession[sessionB].TotalSequences)
	assert.Equal(t, int64(0), perSession[sessionB].TotalMangled)
	assert.Equal(t, 0.0, perSession[sessionB].MangleRate)

	require.Contains(t, perSession, sessionC)
	assert.Equal(t, int64(1), perSession[sessionC].TotalSequences)
	assert.Equal(t, int64(1), perSession[sessionC].TotalMangled)
	assert.InDelta(t, 1.0, perSession[sessionC].MangleRate, 1e-9)
}

func TestGetEscapeAnalyticsGlobalSummary_should_ExcludeEventsOutsideBoundary_When_TimeRangeSet(t *testing.T) {
	t.Parallel()
	svc, client := createTestServiceWithAnalytics(t)

	const sid = "global-session-boundary"
	rangeStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	// Exactly at the inclusive boundaries — must be included.
	insertEscapeEventAt(t, client, sid, "pty", "csi", false, rangeStart)
	insertEscapeEventAt(t, client, sid, "pty", "csi", false, rangeEnd)

	// Just outside each boundary — must be excluded.
	insertEscapeEventAt(t, client, sid, "pty", "csi", false, rangeStart.Add(-time.Second))
	insertEscapeEventAt(t, client, sid, "pty", "csi", false, rangeEnd.Add(time.Second))

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsGlobalSummaryRequest{
		StartTime: timestamppb.New(rangeStart),
		EndTime:   timestamppb.New(rangeEnd),
	})
	resp, err := svc.GetEscapeAnalyticsGlobalSummary(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, int64(2), resp.Msg.TotalSequences, "only the two in-boundary events should count")
	require.Len(t, resp.Msg.PerSession, 1)
	assert.Equal(t, int64(2), resp.Msg.PerSession[0].TotalSequences)
}

func TestGetEscapeAnalyticsGlobalSummary_should_ReturnZeroRate_When_NoEventsMatch(t *testing.T) {
	t.Parallel()
	svc, _ := createTestServiceWithAnalytics(t)

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsGlobalSummaryRequest{})
	resp, err := svc.GetEscapeAnalyticsGlobalSummary(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Msg.TotalSequences)
	assert.Equal(t, int64(0), resp.Msg.TotalMangled)
	assert.Equal(t, 0.0, resp.Msg.MangleRate)
	assert.Empty(t, resp.Msg.Histogram)
	assert.Empty(t, resp.Msg.PerSession)
}

func TestGetEscapeAnalyticsGlobalSummary_should_ReturnExactMangledCount_When_FixtureHasMixedTrueFalseMangled(t *testing.T) {
	t.Parallel()
	svc, client := createTestServiceWithAnalytics(t)

	const sid = "global-session-mixed-mangle"
	// Deliberately mixes true/false mangled rows in the same sequence_type
	// group to catch a SQLite SUM()-over-boolean type mismatch: mangled is
	// stored as 0/1, and Sum(FieldMangled) must add the integers, not choke
	// on or misinterpret the boolean column.
	insertEscapeEvent(t, client, sid, "pty", "csi", true)
	insertEscapeEvent(t, client, sid, "pty", "csi", false)
	insertEscapeEvent(t, client, sid, "pty", "csi", true)
	insertEscapeEvent(t, client, sid, "pty", "csi", false)
	insertEscapeEvent(t, client, sid, "pty", "csi", false)

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsGlobalSummaryRequest{})
	resp, err := svc.GetEscapeAnalyticsGlobalSummary(context.Background(), req)
	require.NoError(t, err)

	require.Len(t, resp.Msg.Histogram, 1)
	assert.Equal(t, int64(5), resp.Msg.Histogram[0].Count)
	assert.Equal(t, int64(2), resp.Msg.Histogram[0].MangledCount, "exactly 2 of 5 rows have mangled=true")

	require.Len(t, resp.Msg.PerSession, 1)
	assert.Equal(t, int64(5), resp.Msg.PerSession[0].TotalSequences)
	assert.Equal(t, int64(2), resp.Msg.PerSession[0].TotalMangled)
	assert.InDelta(t, 2.0/5.0, resp.Msg.PerSession[0].MangleRate, 1e-9)
}

func TestEscapeMangleRate_should_ComputeRatio_When_TotalSequencesPositive(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 0.4, escapeMangleRate(5, 2), 1e-9)
	assert.Equal(t, 1.0, escapeMangleRate(3, 3))
}

func TestEscapeMangleRate_should_ReturnZero_When_TotalSequencesIsZero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0.0, escapeMangleRate(0, 0))
}
