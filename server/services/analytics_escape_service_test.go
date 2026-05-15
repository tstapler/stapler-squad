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
)

// createTestAnalyticsClient opens an in-process SQLite database and runs migrations.
// The database file is in t.TempDir() and is cleaned up automatically.
func createTestAnalyticsClient(t *testing.T) *ent.Client {
	t.Helper()
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(testDir + "/analytics.db"))
	require.NoError(t, err, "createTestAnalyticsClient: failed to open database")
	t.Cleanup(func() { repo.Close() })
	return repo.GetEntClient()
}

// createTestServiceWithAnalytics creates a SessionService wired with an analytics ent client.
func createTestServiceWithAnalytics(t *testing.T) (*SessionService, *ent.Client) {
	t.Helper()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	client := createTestAnalyticsClient(t)
	svc.SetAnalyticsClient(client)
	return svc, client
}

// insertEscapeEvent creates a test escape event directly via the ent client.
func insertEscapeEvent(t *testing.T, client *ent.Client, sessionID, stage, seqType string, mangled bool) {
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
		SetWallTime(time.Now()).
		SetSessionSeq(1).
		Save(context.Background())
	require.NoError(t, err, "insertEscapeEvent failed")
}

// ---------------------------------------------------------------------------
// QueryEscapeAnalytics tests
// ---------------------------------------------------------------------------

func TestQueryEscapeAnalytics_FiltersBySessionID(t *testing.T) {
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
	svc, _ := createTestServiceWithAnalytics(t)

	req := connect.NewRequest(&sessionv1.QueryEscapeAnalyticsRequest{})
	_, err := svc.QueryEscapeAnalytics(context.Background(), req)
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestQueryEscapeAnalytics_NoClientReturnsUnavailable(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(100))
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
	svc, _ := createTestServiceWithAnalytics(t)

	req := connect.NewRequest(&sessionv1.GetEscapeAnalyticsSummaryRequest{})
	_, err := svc.GetEscapeAnalyticsSummary(context.Background(), req)
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
