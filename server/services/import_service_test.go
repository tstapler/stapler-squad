package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/interceptors"
	"github.com/tstapler/stapler-squad/session"
)

// fakeProcessFileInspector is a minimal test double for
// session.ProcessFileInspector, scoped to what HistoryFileDetector needs.
type fakeProcessFileInspector struct {
	openFiles []string
	openErr   error
}

func (f *fakeProcessFileInspector) OpenFiles(pid int32) ([]string, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return f.openFiles, nil
}

func (f *fakeProcessFileInspector) IsAlive(pid int32, expectedCreateTimeMs int64) bool {
	return true
}

// fakeCreateTimeReader is a minimal test double for ProcessCreateTimeReader.
type fakeCreateTimeReader struct {
	createTimeMs int64
	err          error
}

func (f *fakeCreateTimeReader) CreateTime(pid int32) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.createTimeMs, nil
}

func writeSampleTranscript(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := `{"type":"user","message":{"role":"user","content":"hello"},"uuid":"u1","timestamp":"2026-07-16T10:00:00Z","sessionId":"s1","cwd":"/tmp"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"hi there"},"uuid":"u2","timestamp":"2026-07-16T10:00:01Z","sessionId":"s1","cwd":"/tmp"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestImportService_PreviewImportExternalSession_ReturnsResolved_When_PIDExactMatch(t *testing.T) {
	tmpHome := t.TempDir()
	uuid := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	projectPath := "/Users/alice/myproject"
	projectDir := session.ClaudeProjectDirName(projectPath)
	historyPath := filepath.Join(tmpHome, ".claude", "projects", projectDir, uuid+".jsonl")
	writeSampleTranscript(t, historyPath)

	inspector := &fakeProcessFileInspector{openFiles: []string{historyPath}}
	detector := session.NewHistoryFileDetectorWithHomeDir(inspector, tmpHome)
	svc := NewImportService(detector, &fakeCreateTimeReader{createTimeMs: 12345})

	req := connect.NewRequest(&sessionv1.PreviewImportExternalSessionRequest{
		Candidate: &sessionv1.ExternalSessionCandidateRef{
			SourceKind: sessionv1.ImportSourceKind_IMPORT_SOURCE_KIND_MUX_DISCOVERED,
			Path:       projectPath,
			Program:    "claude",
			Pid:        1234,
		},
	})

	resp, err := svc.PreviewImportExternalSession(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, sessionv1.CorrelationKind_CORRELATION_KIND_RESOLVED, resp.Msg.Correlation.Kind)
	assert.Equal(t, sessionv1.CorrelationConfidence_CORRELATION_CONFIDENCE_PID_EXACT, resp.Msg.Correlation.Confidence)
	assert.Equal(t, uuid, resp.Msg.Correlation.Uuid)
	assert.Equal(t, int32(2), resp.Msg.TurnCount)
	assert.Equal(t, "hi there", resp.Msg.LastMessageExcerpt)
	require.NotNil(t, resp.Msg.PidIdentity)
	assert.Equal(t, int32(1234), resp.Msg.PidIdentity.Pid)
	assert.Equal(t, int64(12345), resp.Msg.PidIdentity.CreateTimeMs)
}

func TestImportService_PreviewImportExternalSession_ReturnsAmbiguous_When_MultipleHistoryFiles(t *testing.T) {
	tmpHome := t.TempDir()
	projectPath := "/Users/alice/ambiguousproject"
	projectDir := session.ClaudeProjectDirName(projectPath)
	dir := filepath.Join(tmpHome, ".claude", "projects", projectDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	uuid1 := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	uuid2 := "11111111-2222-3333-4444-555555555555"
	require.NoError(t, os.WriteFile(filepath.Join(dir, uuid1+".jsonl"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, uuid2+".jsonl"), []byte("{}"), 0o644))

	inspector := &fakeProcessFileInspector{openFiles: nil}
	detector := session.NewHistoryFileDetectorWithHomeDir(inspector, tmpHome)
	svc := NewImportService(detector, &fakeCreateTimeReader{})

	req := connect.NewRequest(&sessionv1.PreviewImportExternalSessionRequest{
		Candidate: &sessionv1.ExternalSessionCandidateRef{
			SourceKind: sessionv1.ImportSourceKind_IMPORT_SOURCE_KIND_MUX_DISCOVERED,
			Path:       projectPath,
			Program:    "claude",
		},
	})

	resp, err := svc.PreviewImportExternalSession(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, sessionv1.CorrelationKind_CORRELATION_KIND_AMBIGUOUS, resp.Msg.Correlation.Kind)
	assert.Empty(t, resp.Msg.Correlation.Uuid)
	require.Len(t, resp.Msg.Correlation.Candidates, 2)
	// Ambiguous case never populates turn_count/excerpt -- the caller must
	// disambiguate first (via CommitImportExternalSession's
	// disambiguation_choice field).
	assert.Zero(t, resp.Msg.TurnCount)
	assert.Empty(t, resp.Msg.LastMessageExcerpt)
	assert.Nil(t, resp.Msg.PidIdentity)
}

func TestImportService_PreviewImportExternalSession_ReturnsNotFound_When_NoHistoryFileExists(t *testing.T) {
	tmpHome := t.TempDir()
	inspector := &fakeProcessFileInspector{openFiles: nil}
	detector := session.NewHistoryFileDetectorWithHomeDir(inspector, tmpHome)
	svc := NewImportService(detector, &fakeCreateTimeReader{})

	req := connect.NewRequest(&sessionv1.PreviewImportExternalSessionRequest{
		Candidate: &sessionv1.ExternalSessionCandidateRef{
			SourceKind: sessionv1.ImportSourceKind_IMPORT_SOURCE_KIND_MUX_DISCOVERED,
			Path:       "/Users/alice/nohistory",
			Program:    "claude",
		},
	})

	resp, err := svc.PreviewImportExternalSession(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, sessionv1.CorrelationKind_CORRELATION_KIND_NOT_FOUND, resp.Msg.Correlation.Kind)
	assert.Zero(t, resp.Msg.TurnCount)
	assert.Nil(t, resp.Msg.PidIdentity)
}

func TestImportService_PreviewImportExternalSession_ReturnsInvalidArgument_When_CandidateNil(t *testing.T) {
	tmpHome := t.TempDir()
	detector := session.NewHistoryFileDetectorWithHomeDir(&fakeProcessFileInspector{}, tmpHome)
	svc := NewImportService(detector, &fakeCreateTimeReader{})

	req := connect.NewRequest(&sessionv1.PreviewImportExternalSessionRequest{})

	_, err := svc.PreviewImportExternalSession(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestImportService_PreviewImportExternalSession_OmitsPidIdentity_When_ProcessDied(t *testing.T) {
	tmpHome := t.TempDir()
	inspector := &fakeProcessFileInspector{openFiles: nil}
	detector := session.NewHistoryFileDetectorWithHomeDir(inspector, tmpHome)
	svc := NewImportService(detector, &fakeCreateTimeReader{err: os.ErrNotExist})

	req := connect.NewRequest(&sessionv1.PreviewImportExternalSessionRequest{
		Candidate: &sessionv1.ExternalSessionCandidateRef{
			SourceKind: sessionv1.ImportSourceKind_IMPORT_SOURCE_KIND_MUX_DISCOVERED,
			Path:       "/Users/alice/nohistory",
			Program:    "claude",
			Pid:        999,
		},
	})

	resp, err := svc.PreviewImportExternalSession(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.PidIdentity)
}

func TestImportService_CommitImportExternalSession_ReturnsUnimplemented(t *testing.T) {
	svc := NewImportService(session.NewHistoryFileDetectorWithHomeDir(&fakeProcessFileInspector{}, t.TempDir()), &fakeCreateTimeReader{})
	_, err := svc.CommitImportExternalSession(context.Background(), connect.NewRequest(&sessionv1.CommitImportExternalSessionRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestImportService_ConfirmKillExternalSession_ReturnsUnimplemented(t *testing.T) {
	svc := NewImportService(session.NewHistoryFileDetectorWithHomeDir(&fakeProcessFileInspector{}, t.TempDir()), &fakeCreateTimeReader{})
	_, err := svc.ConfirmKillExternalSession(context.Background(), connect.NewRequest(&sessionv1.ConfirmKillExternalSessionRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestImportService_CancelPendingKill_ReturnsUnimplemented(t *testing.T) {
	svc := NewImportService(session.NewHistoryFileDetectorWithHomeDir(&fakeProcessFileInspector{}, t.TempDir()), &fakeCreateTimeReader{})
	_, err := svc.CancelPendingKill(context.Background(), connect.NewRequest(&sessionv1.CancelPendingKillRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

// newGatedImportTestServer wires ImportService behind the same
// interceptors.NewScopedFeatureFlagInterceptor used in production
// (server/server.go), gating only the three mutating RPCs, so these tests
// exercise the real Task 0 gating path end-to-end rather than just the
// handler stubs above.
func newGatedImportTestServer(t *testing.T, flagEnabled func() bool) sessionv1connect.ImportServiceClient {
	t.Helper()
	svc := NewImportService(session.NewHistoryFileDetectorWithHomeDir(&fakeProcessFileInspector{}, t.TempDir()), &fakeCreateTimeReader{})
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewImportServiceHandler(
		svc,
		connect.WithInterceptors(interceptors.NewScopedFeatureFlagInterceptor(
			"session_import",
			flagEnabled,
			"CommitImportExternalSession",
			"ConfirmKillExternalSession",
			"CancelPendingKill",
		)),
	)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return sessionv1connect.NewImportServiceClient(srv.Client(), srv.URL)
}

// TestImportService_ThreeMutatingRPCs_ReturnUnimplemented_When_FeatureFlagUnset
// covers Task 0's fix to the feature-flag interceptor registered in
// server/server.go: only CommitImportExternalSession,
// ConfirmKillExternalSession, and CancelPendingKill are gated behind
// STAPLER_SQUAD_ENABLE_SESSION_IMPORT. This replaces validation.md's stale
// "TestImportService_AllThreeRPCs_ReturnUnimplemented_When_FeatureFlagUnset"
// (written before Task 0 identified that the interceptor was incorrectly
// gating all four RPCs, including the read-only Preview) -- the name/intent
// is preserved (three mutating RPCs return Unimplemented with the flag off)
// but clarified to make explicit that "three" refers to the mutating set,
// not "all" RPCs.
func TestImportService_ThreeMutatingRPCs_ReturnUnimplemented_When_FeatureFlagUnset(t *testing.T) {
	client := newGatedImportTestServer(t, func() bool { return false })

	_, err := client.CommitImportExternalSession(context.Background(), connect.NewRequest(&sessionv1.CommitImportExternalSessionRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))

	_, err = client.ConfirmKillExternalSession(context.Background(), connect.NewRequest(&sessionv1.ConfirmKillExternalSessionRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))

	_, err = client.CancelPendingKill(context.Background(), connect.NewRequest(&sessionv1.CancelPendingKillRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

// TestImportService_PreviewImportExternalSession_AlwaysExecutes_When_FeatureFlagUnset
// is the companion assertion Task 0 exists to guarantee: the read-only
// preview RPC must remain reachable even while the flag is off and the three
// mutating RPCs are gated -- it must never be swept up by the same
// interceptor.
func TestImportService_PreviewImportExternalSession_AlwaysExecutes_When_FeatureFlagUnset(t *testing.T) {
	client := newGatedImportTestServer(t, func() bool { return false })

	_, err := client.PreviewImportExternalSession(context.Background(), connect.NewRequest(&sessionv1.PreviewImportExternalSessionRequest{
		Candidate: &sessionv1.ExternalSessionCandidateRef{
			SourceKind: sessionv1.ImportSourceKind_IMPORT_SOURCE_KIND_MUX_DISCOVERED,
			Path:       "/Users/alice/nohistory",
			Program:    "claude",
		},
	}))
	require.NoError(t, err, "PreviewImportExternalSession must always execute regardless of feature flag state")
}

// TestImportService_AllFourRPCs_ExecuteNormally_When_FeatureFlagTrue covers
// validation.md's "TestImportService_AllThreeRPCs_ExecuteNormally_When_FeatureFlagTrue"
// happy path -- renamed to "AllFourRPCs" since, with the flag on, gating is a
// no-op for every RPC (mutating or not), which is the scenario worth
// asserting now that Preview is unconditionally ungated.
func TestImportService_AllFourRPCs_ExecuteNormally_When_FeatureFlagTrue(t *testing.T) {
	client := newGatedImportTestServer(t, func() bool { return true })

	_, err := client.PreviewImportExternalSession(context.Background(), connect.NewRequest(&sessionv1.PreviewImportExternalSessionRequest{
		Candidate: &sessionv1.ExternalSessionCandidateRef{
			SourceKind: sessionv1.ImportSourceKind_IMPORT_SOURCE_KIND_MUX_DISCOVERED,
			Path:       "/Users/alice/nohistory",
			Program:    "claude",
		},
	}))
	require.NoError(t, err)

	// The three mutating RPCs are still unimplemented stubs (Stories 1.2.1 /
	// 1.3.1 / 1.3.3 land in later tasks), so with the flag on they must fail
	// with CodeUnimplemented from the handler itself, never from the
	// interceptor -- i.e. the interceptor must be a complete no-op here.
	_, err = client.CommitImportExternalSession(context.Background(), connect.NewRequest(&sessionv1.CommitImportExternalSessionRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))

	_, err = client.ConfirmKillExternalSession(context.Background(), connect.NewRequest(&sessionv1.ConfirmKillExternalSessionRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))

	_, err = client.CancelPendingKill(context.Background(), connect.NewRequest(&sessionv1.CancelPendingKillRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
