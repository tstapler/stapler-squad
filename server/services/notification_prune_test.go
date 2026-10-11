package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/notifications"
)

const (
	pruneTypeInfo         = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO)
	pruneTypeTaskComplete = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE)
	pruneTypeWarning      = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING)
)

type pruneRow struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Type      int32  `json:"notification_type"`
	IsRead    bool   `json:"is_read"`
	CreatedAt string `json:"created_at"`
}

type pruneHarness struct {
	t         *testing.T
	ns        *NotificationService
	store     *notifications.NotificationHistoryStore
	gate      *deliverygate.Gate
	mfs       *memFS
	storePath string
	srv       *httptest.Server
	client    sessionv1connect.SessionServiceClient
}

// newPruneHarness serves the real SessionService delegate over httptest behind
// the request-record chain, so the handler sees a peer address and headers.
func newPruneHarness(t *testing.T, rows []pruneRow, hidden, visible []string) *pruneHarness {
	t.Helper()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "notifications.json")
	for i := range rows {
		rows[i].CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.Marshal(map[string]any{"version": 1, "notifications": rows})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(storePath, data, 0o600))
	store, err := notifications.NewNotificationHistoryStore(storePath)
	require.NoError(t, err)

	gate := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) { return deliverygate.FlagSettings{}, nil }))
	entries := make([]deliverygate.Entry, 0, len(hidden)+len(visible))
	for _, id := range hidden {
		entries = append(entries, deliverygate.Entry{UUID: id, Title: "title-" + id, Hidden: true, Kind: deliverygate.KindReview})
	}
	for _, id := range visible {
		entries = append(entries, deliverygate.Entry{UUID: id, Title: "title-" + id})
	}
	mfs := newMemFS()
	ns := &NotificationService{notificationStore: store, deliveryGate: gate}
	ns.SetAuditSink(NewAuditSink(func() (string, error) { return "/cfg", nil }, WithAuditFS(mfs)))
	ns.pruneNow = func() time.Time { return time.Date(2026, 10, 9, 18, 30, 45, 0, time.UTC) }

	h := &pruneHarness{t: t, ns: ns, store: store, gate: gate, mfs: mfs, storePath: storePath}
	h.seed(entries)
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(&SessionService{notificationSvc: ns})
	mux.Handle(path, handler)
	h.srv = httptest.NewServer(WithRequestRecord(ListenerLocal, false)(mux))
	t.Cleanup(h.srv.Close)
	h.client = sessionv1connect.NewSessionServiceClient(h.srv.Client(), h.srv.URL)
	return h
}

func (h *pruneHarness) seed(entries []deliverygate.Entry) {
	h.gate.Index().Replace(entries)
}

func (h *pruneHarness) prune(apply, includeUnread bool) (*sessionv1.PruneHiddenSessionNotificationsResponse, error) {
	req := connect.NewRequest(&sessionv1.PruneHiddenSessionNotificationsRequest{Apply: apply, IncludeUnreadActionable: includeUnread})
	req.Header().Set("User-Agent", "prune-test")
	resp, err := h.client.PruneHiddenSessionNotifications(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (h *pruneHarness) auditLines() []AuditLine {
	b, ok := h.mfs.get("/cfg/audit/hidden-session-replies.jsonl")
	if !ok {
		return nil
	}
	var out []AuditLine
	for _, l := range lines(b) {
		var al AuditLine
		require.NoError(h.t, json.Unmarshal(l, &al))
		out = append(out, al)
	}
	return out
}

func storyRows() []pruneRow {
	return []pruneRow{
		{ID: "h1-warn", SessionID: "h1", Type: pruneTypeWarning, IsRead: true},
		{ID: "h1-info", SessionID: "h1", Type: pruneTypeInfo},
		{ID: "h1-done", SessionID: "h1", Type: pruneTypeTaskComplete},
		{ID: "h2-warn", SessionID: "h2", Type: pruneTypeWarning},
		{ID: "d1-warn", SessionID: "d1", Type: pruneTypeWarning},
		{ID: "v1-warn", SessionID: "v1", Type: pruneTypeWarning},
	}
}

func remainingIDs(h *pruneHarness) []string {
	resp, _, err := h.ns.GetNotificationStore().List(notifications.ListOptions{Limit: 1000})
	require.NoError(h.t, err)
	ids := make([]string, 0, len(resp))
	for _, r := range resp {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestPruneRPC_ShouldReturnFailedPrecondition_WhenIndexUnseeded(t *testing.T) {
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})
	unseeded := &NotificationService{notificationStore: h.store, deliveryGate: deliverygate.NewGate()}
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(&SessionService{notificationSvc: unseeded})
	mux.Handle(path, handler)
	srv := httptest.NewServer(WithRequestRecord(ListenerLocal, false)(mux))
	defer srv.Close()
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	for _, apply := range []bool{false, true} {
		_, err := client.PruneHiddenSessionNotifications(context.Background(),
			connect.NewRequest(&sessionv1.PruneHiddenSessionNotificationsRequest{Apply: apply}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	}
	assert.Len(t, remainingIDs(h), 6)
}

func TestPruneRPC_ShouldReturnDryRunPlanWithoutAuditLineOrBackup_WhenApplyUnset(t *testing.T) {
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})

	resp, err := h.prune(false, false)
	require.NoError(t, err)

	assert.False(t, resp.Applied)
	assert.ElementsMatch(t, []string{"h1-warn", "h1-info", "h1-done"}, resp.NotificationIds)
	assert.Equal(t, int32(1), resp.KeptUnreadActionable)
	assert.Equal(t, int32(1), resp.Undeterminable)
	assert.Empty(t, resp.BackupPath)
	assert.Empty(t, h.auditLines(), "a dry run writes no audit line")
	assert.Len(t, remainingIDs(h), 6)
	matches, _ := filepath.Glob(h.storePath + ".pre-prune-*")
	assert.Empty(t, matches)
}

func TestPruneApply_ShouldAppendKindPruneAuditLineBeforeDeleteAndRejectRebindingHostWhenGuardedAndShouldNotAuditDryRun(t *testing.T) {
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})

	resp, err := h.prune(true, false)
	require.NoError(t, err)
	assert.True(t, resp.Applied)
	assert.ElementsMatch(t, []string{"h1-warn", "h1-info", "h1-done"}, resp.NotificationIds)
	assert.Equal(t, h.storePath+".pre-prune-20261009T183045Z.bak", resp.BackupPath)
	assert.FileExists(t, resp.BackupPath)
	assert.ElementsMatch(t, []string{"h2-warn", "d1-warn", "v1-warn"}, remainingIDs(h))

	got := h.auditLines()
	require.Len(t, got, 2)
	req, result := got[0], got[1]
	assert.Equal(t, auditKindPrune, req.Kind)
	assert.Equal(t, auditPhaseRequest, req.Phase)
	require.NotNil(t, req.MatchedCount)
	assert.Equal(t, 3, *req.MatchedCount)
	require.NotNil(t, req.IncludeUnreadActionable)
	assert.False(t, *req.IncludeUnreadActionable)
	assert.Equal(t, map[string]int{
		notifications.PruneReasonRoutine: 3, pruneCountKeptUnread: 1, pruneCountUndeterminable: 1, pruneCountVisible: 1,
	}, req.Counts)
	assert.Equal(t, ListenerLocal, req.Listener)
	assert.Equal(t, AuthModeNone, req.AuthMode)
	assert.Equal(t, "prune-test", req.UserAgent)
	assert.True(t, strings.HasPrefix(req.PeerAddr, "127.0.0.1:"), req.PeerAddr)
	require.NotNil(t, req.PeerLoopback)
	assert.True(t, *req.PeerLoopback)
	assert.Equal(t, auditPhaseResult, result.Phase)
	assert.Equal(t, pruneOutcomeApplied, result.Outcome)
	assert.Equal(t, req.ChangeID, result.ChangeID)

	raw, _ := h.mfs.get("/cfg/audit/hidden-session-replies.jsonl")
	for _, secret := range []string{"h1-warn", "h1-info", "title-h1", "\"h1\"", "session_id"} {
		assert.NotContains(t, string(raw), secret, "the audit line carries counts only")
	}
}

func TestPruneApply_ShouldReturnInternalWithNoDeleteAndNoBackup_WhenAuditAppendFails(t *testing.T) {
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})
	h.mfs.failWrite = errDiskFull

	_, err := h.prune(true, false)

	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Len(t, remainingIDs(h), 6, "failed append means no delete")
	matches, _ := filepath.Glob(h.storePath + ".pre-prune-*")
	assert.Empty(t, matches, "no backup is written when the audit append fails")

	// A flaw in the sink never blocks the dry run.
	resp, err := h.prune(false, false)
	require.NoError(t, err)
	assert.Len(t, resp.NotificationIds, 3)
}

func TestPruneApply_ShouldRefuse_WhenNoAuditSinkIsConfigured(t *testing.T) {
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})
	h.ns.SetAuditSink(nil)

	_, err := h.prune(true, false)

	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Len(t, remainingIDs(h), 6)
}

func TestPruneApply_ShouldRemoveUnreadActionableOnlyWhenIncluded_AndKeepUndeterminableInEveryMode(t *testing.T) {
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})

	resp, err := h.prune(true, true)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"h1-warn", "h1-info", "h1-done", "h2-warn"}, resp.NotificationIds)
	assert.Equal(t, int32(0), resp.KeptUnreadActionable)
	assert.Equal(t, int32(1), resp.Undeterminable)
	assert.ElementsMatch(t, []string{"d1-warn", "v1-warn"}, remainingIDs(h))

	again, err := h.prune(true, true)
	require.NoError(t, err)
	assert.Empty(t, again.NotificationIds, "re-runnable: a second apply removes nothing more")
	assert.False(t, again.Applied)
}

// T-PR-15: rows matched by UUID, title or an alias, and a visible session that
// reused a hidden alias key, group by resolved title and kind with the id form.
func TestPruneDryRun_ShouldGroupByResolvedTitleAndKindTagIDFormAndFlagAliasAndKeepAmbiguousRows_WhenUuidTitleAndAliasFormsMix(t *testing.T) {
	rows := []pruneRow{
		{ID: "by-uuid", SessionID: "h1", Type: pruneTypeInfo, IsRead: true},
		{ID: "by-title", SessionID: "title-h1", Type: pruneTypeTaskComplete, IsRead: true},
		{ID: "by-alias", SessionID: "old-name", Type: pruneTypeInfo, IsRead: true},
		{ID: "reused", SessionID: "reused-name", Type: pruneTypeInfo, IsRead: true},
	}
	h := newPruneHarness(t, rows, []string{"h1"}, nil)
	// h1 was renamed from old-name; a visible session then took reused-name,
	// which had also been an alias of h1.
	idx := h.gate.Index()
	idx.Upsert(deliverygate.Entry{UUID: "h1", Title: "old-name", Hidden: true, Kind: deliverygate.KindReview})
	idx.Upsert(deliverygate.Entry{UUID: "h1", Title: "reused-name", Hidden: true, Kind: deliverygate.KindReview})
	idx.Upsert(deliverygate.Entry{UUID: "h1", Title: "title-h1", Hidden: true, Kind: deliverygate.KindReview})
	idx.Upsert(deliverygate.Entry{UUID: "v9", Title: "reused-name"})

	var plan notifications.PrunePlan
	classify := func(r *notifications.NotificationRecord) notifications.PruneDecision {
		return pruneDecision(h.gate.Resolver().ClassifyStored(r.SessionID, r.Metadata), r.SessionID)
	}
	plan, err := h.store.PruneByPredicate(classify, notifications.PruneOptions{})
	require.NoError(t, err)

	byID := map[string]notifications.PrunePlanRow{}
	for _, r := range plan.Remove {
		byID[r.ID] = r
	}
	assert.Equal(t, "title-h1 [review]", byID["by-uuid"].Group)
	assert.Equal(t, "uuid", byID["by-uuid"].Form)
	assert.Equal(t, "title", byID["by-title"].Form)
	assert.Equal(t, "alias", byID["by-alias"].Form)
	assert.True(t, byID["by-alias"].ByAlias)
	assert.NotContains(t, byID, "reused", "a visible session that reused an alias key is never pruned")
	assert.Equal(t, 1, plan.Visible)
}

// T-PR-07: auth rejects on the remote chain, the handler records the caller's
// address, and the RPC has no MCP tool.
func TestPruneRPC_ShouldRejectUnauthenticatedAndLogRemoteAddrAndHaveNoMCPTool_WhenAuthEnabled(t *testing.T) {
	h := newPruneHarness(t, storyRows(), []string{"h1", "h2"}, []string{"v1"})

	// The remote chain (auth required) refuses before the handler is reached.
	reached := false
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(&SessionService{notificationSvc: h.ns})
	mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		handler.ServeHTTP(w, r)
	}))
	rejectAll := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(WithRequestRecord(ListenerRemote, true)(rejectAll))
	defer srv.Close()
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
	_, err := client.PruneHiddenSessionNotifications(context.Background(),
		connect.NewRequest(&sessionv1.PruneHiddenSessionNotificationsRequest{Apply: true}))
	require.Error(t, err)
	assert.False(t, reached)
	assert.Len(t, remainingIDs(h), 6)

	// An authenticated apply records the caller's address, listener and auth mode.
	authed := connect.NewRequest(&sessionv1.PruneHiddenSessionNotificationsRequest{Apply: true})
	authed.Header().Set("Cookie", "session=ok")
	_, err = client.PruneHiddenSessionNotifications(context.Background(), authed)
	require.NoError(t, err)
	line := h.auditLines()[0]
	assert.Equal(t, ListenerRemote, line.Listener)
	assert.Equal(t, AuthModeRequired, line.AuthMode)
	assert.NotEmpty(t, line.PeerAddr)

	assertNoMCPToolFor(t, "PruneHiddenSessionNotifications", "prune_hidden_session_notifications")
}

func assertNoMCPToolFor(t *testing.T, names ...string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "mcp", "*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, n := range names {
			assert.NotContains(t, strings.ToLower(string(b)), strings.ToLower(n), "%s must not expose the prune RPC", f)
		}
	}
}
