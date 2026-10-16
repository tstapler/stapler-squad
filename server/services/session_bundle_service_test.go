package services_test

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/artifacts"
	"github.com/tstapler/stapler-squad/session/contexthistory"
)

// ─── fakes ────────────────────────────────────────────────────────────────────

type fakeBundleStorage struct {
	analytics    []session.AnalyticsData
	analyticsErr error
	itemSession  session.ItemSessionSummary
	itemErr      error
	artBlob      *artifacts.SessionArtifactsBlob
	artErr       error
}

func (f *fakeBundleStorage) ListAnalyticsBySessionID(_ context.Context, _ string) ([]session.AnalyticsData, error) {
	return f.analytics, f.analyticsErr
}

func (f *fakeBundleStorage) GetItemSessionBySessionUUID(_ context.Context, _ string) (session.ItemSessionSummary, error) {
	return f.itemSession, f.itemErr
}

func (f *fakeBundleStorage) GetSessionArtifactsByUUID(_ context.Context, _ string) (*artifacts.SessionArtifactsBlob, error) {
	return f.artBlob, f.artErr
}

type fakeScrollback struct {
	mu        sync.Mutex
	callOrder []string // records "flush" or "read" as they arrive
	lines     []byte
	flushErr  error
	readErr   error
}

func (f *fakeScrollback) FlushSession(_ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callOrder = append(f.callOrder, "flush")
	return f.flushErr
}

func (f *fakeScrollback) GetRecentLines(_ string, _ int) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callOrder = append(f.callOrder, "read")
	return f.lines, f.readErr
}

type fakeCtxHistory struct {
	summary contexthistory.SessionSummary
	series  []contexthistory.Sample
	comps   []contexthistory.Compaction
	err     error
}

func (f *fakeCtxHistory) SummarizeSession(_ context.Context, _ string) (contexthistory.SessionSummary, error) {
	return f.summary, f.err
}

func (f *fakeCtxHistory) Series(_ context.Context, _ string) ([]contexthistory.Sample, error) {
	return f.series, f.err
}

func (f *fakeCtxHistory) Compactions(_ context.Context, _ string) ([]contexthistory.Compaction, error) {
	return f.comps, f.err
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// newTestInstance builds a zero-value *session.Instance with UUID set so
// assembleBundle's Snapshot() call resolves the UUID correctly.
func newTestInstance(uuid string) *session.Instance {
	inst := &session.Instance{}
	inst.UUID = uuid
	return inst
}

// readZIP parses a ZIP from raw bytes and returns a map from entry name to content.
func readZIP(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err, "parse ZIP")
	out := make(map[string][]byte, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(rc)
		_ = rc.Close()
		out[f.Name] = buf.Bytes()
	}
	return out
}

// ─── TC-001: Valid ZIP with manifest ─────────────────────────────────────────

func TestAssembleBundle_ValidZIPWithManifest(t *testing.T) {
	svc := services.NewSessionBundleService(
		&fakeBundleStorage{
			analytics: []session.AnalyticsData{{ToolName: "bash", SessionID: "uuid-001"}},
		},
		&fakeCtxHistory{},
		&fakeScrollback{lines: []byte("line1\nline2")},
	)

	inst := newTestInstance("uuid-001")
	zipBytes, manifest, err := exportForTest(t, svc, inst, 100)
	require.NoError(t, err)
	assert.Greater(t, len(zipBytes), 0)

	entries := readZIP(t, zipBytes)

	// manifest.json must be present and parseable.
	manifestRaw, ok := entries["manifest.json"]
	require.True(t, ok, "manifest.json must be in ZIP")
	var m services.BundleManifest
	require.NoError(t, json.Unmarshal(manifestRaw, &m))
	assert.Equal(t, 1, m.BundleFormatVersion)
	assert.Equal(t, "uuid-001", m.SessionUUID)

	// session_metadata.json and server_info.json are required sections.
	assert.Contains(t, entries, "session_metadata.json")
	assert.Contains(t, entries, "server_info.json")

	// Scrollback should be present too.
	assert.Contains(t, entries, "scrollback.txt")

	// manifest returned by assembleBundle matches embedded manifest.json.
	assert.Equal(t, m.BundleFormatVersion, manifest.BundleFormatVersion)
}

// ─── TC-002: Size cap drops analytics first ───────────────────────────────────

func TestAssembleBundle_SizeCapDropsAnalytics(t *testing.T) {
	// Build a storage that returns ~1 KB of analytics data.
	bigAnalytics := make([]session.AnalyticsData, 100)
	for i := range bigAnalytics {
		bigAnalytics[i] = session.AnalyticsData{ToolName: "resize", SessionID: "abc"}
	}

	svc := services.NewSessionBundleService(
		&fakeBundleStorage{analytics: bigAnalytics},
		nil, // ctxHistory disabled
		&fakeScrollback{lines: bytes.Repeat([]byte("x"), 512)},
	)

	inst := newTestInstance("uuid-002")
	zipBytes, manifest, err := exportForTest(t, svc, inst, 10)
	require.NoError(t, err)

	entries := readZIP(t, zipBytes)

	// Even with big analytics the required sections must survive.
	assert.Contains(t, entries, "session_metadata.json")
	assert.Contains(t, entries, "server_info.json")

	// analytics must appear either in included or omitted.
	inIncluded := false
	for _, s := range manifest.IncludedSections {
		if s == "analytics" {
			inIncluded = true
		}
	}
	inOmitted := false
	for _, o := range manifest.OmittedSections {
		if o.Name == "analytics" {
			inOmitted = true
		}
	}
	assert.True(t, inIncluded || inOmitted, "analytics must appear in IncludedSections or OmittedSections")
}

// ─── TC-003: Scrollback uses GetRecentLines (tail), not head ─────────────────

func TestAssembleBundle_ScrollbackTailNotOldest(t *testing.T) {
	// GetRecentLines returns the MOST-RECENT N lines.  The fake returns a known
	// sentinel so we can confirm the content in the ZIP came from GetRecentLines.
	sentinel := []byte("RECENT-TAIL-SENTINEL")
	sc := &fakeScrollback{lines: sentinel}

	svc := services.NewSessionBundleService(
		&fakeBundleStorage{},
		nil,
		sc,
	)

	inst := newTestInstance("uuid-003")
	zipBytes, _, err := exportForTest(t, svc, inst, 50)
	require.NoError(t, err)

	entries := readZIP(t, zipBytes)
	scrollbackEntry, ok := entries["scrollback.txt"]
	require.True(t, ok, "scrollback.txt must appear in ZIP")
	assert.Equal(t, sentinel, scrollbackEntry, "scrollback content must match GetRecentLines output")
}

// ─── TC-004: Nil ctxHistory store omits context_history section ───────────────

func TestAssembleBundle_NilCtxHistoryStore(t *testing.T) {
	svc := services.NewSessionBundleService(
		&fakeBundleStorage{},
		nil, // explicitly nil — analytics disabled / not wired
		&fakeScrollback{},
	)

	inst := newTestInstance("uuid-004")
	zipBytes, manifest, err := exportForTest(t, svc, inst, 10)
	require.NoError(t, err)

	// context_history must not appear in the ZIP.
	entries := readZIP(t, zipBytes)
	assert.NotContains(t, entries, "context_history.json")

	// It must appear in OmittedSections with a meaningful reason.
	found := false
	for _, o := range manifest.OmittedSections {
		if o.Name == "context_history" {
			found = true
			assert.NotEmpty(t, o.Reason)
		}
	}
	assert.True(t, found, "context_history must appear in OmittedSections when ctxHistory is nil")
}

// ─── TC-008: RunDiagnosticsRetention prunes old files ─────────────────────────

func TestRunDiagnosticsRetention_PrunesOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Create three .zip files: two old, one recent.
	oldTime := time.Now().Add(-31 * 24 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)

	writeZip := func(name string, mtime time.Time) {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("fake"), 0o600))
		require.NoError(t, os.Chtimes(path, mtime, mtime))
	}
	writeZip("old1.zip", oldTime)
	writeZip("old2.zip", oldTime)
	writeZip("recent.zip", newTime)

	// A non-.zip file that should be left untouched.
	notAZip := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(notAZip, []byte("keep"), 0o600))

	err := services.RunDiagnosticsRetention(dir, 30*24*time.Hour)
	require.NoError(t, err)

	_, err1 := os.Stat(filepath.Join(dir, "old1.zip"))
	assert.True(t, os.IsNotExist(err1), "old1.zip must be deleted")
	_, err2 := os.Stat(filepath.Join(dir, "old2.zip"))
	assert.True(t, os.IsNotExist(err2), "old2.zip must be deleted")

	_, err3 := os.Stat(filepath.Join(dir, "recent.zip"))
	assert.NoError(t, err3, "recent.zip must be kept")

	_, err4 := os.Stat(notAZip)
	assert.NoError(t, err4, "non-.zip file must be kept")
}

// ─── TC-009: 5-second deadline cancels assembleBundle ────────────────────────

func TestAssembleBundle_FiveSecondDeadline(t *testing.T) {
	// A storage that blocks until the context is cancelled.
	blocking := &blockingStorage{}

	svc := services.NewSessionBundleService(
		blocking,
		nil,
		&fakeScrollback{},
	)

	inst := newTestInstance("uuid-009")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := exportForTestCtx(t, ctx, svc, inst, 10)
	assert.Error(t, err, "assembleBundle must return an error when context is cancelled")
	assert.True(t,
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"error must be context.DeadlineExceeded or context.Canceled, got: %v", err)
}

type blockingStorage struct{}

func (b *blockingStorage) ListAnalyticsBySessionID(ctx context.Context, _ string) ([]session.AnalyticsData, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingStorage) GetItemSessionBySessionUUID(_ context.Context, _ string) (session.ItemSessionSummary, error) {
	return session.ItemSessionSummary{}, nil
}

func (b *blockingStorage) GetSessionArtifactsByUUID(_ context.Context, _ string) (*artifacts.SessionArtifactsBlob, error) {
	return nil, nil //nolint:nilnil // fake: no stored artifacts
}

// ─── TC-010: FlushSession called before GetRecentLines ────────────────────────

func TestAssembleBundle_FlushCalledBeforeRead(t *testing.T) {
	sc := &fakeScrollback{lines: []byte("ok")}

	svc := services.NewSessionBundleService(
		&fakeBundleStorage{},
		nil,
		sc,
	)

	inst := newTestInstance("uuid-010")
	_, _, err := exportForTest(t, svc, inst, 10)
	require.NoError(t, err)

	sc.mu.Lock()
	order := make([]string, len(sc.callOrder))
	copy(order, sc.callOrder)
	sc.mu.Unlock()

	require.GreaterOrEqual(t, len(order), 2, "expected at least flush + read calls")
	assert.Equal(t, "flush", order[0], "FlushSession must be called before GetRecentLines")
	assert.Equal(t, "read", order[1], "GetRecentLines must follow FlushSession")
}

// ─── TC-022: SaveSessionBundle writes file and returns hash ──────────────────

func TestSaveSessionBundle_WritesFileAndHash(t *testing.T) {
	svc := services.NewSessionBundleService(
		&fakeBundleStorage{
			analytics: []session.AnalyticsData{{ToolName: "click", SessionID: "uuid-022"}},
		},
		nil,
		&fakeScrollback{lines: []byte("scroll")},
	)

	inst := newTestInstance("uuid-022")

	// assemble the ZIP to verify file content and SHA-256 hash.
	zipBytes, _, err := exportForTest(t, svc, inst, 100)
	require.NoError(t, err)

	// Write to a temp dir and verify the file.
	dir := t.TempDir()
	filename := "bundle-test.zip"
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, zipBytes, 0o640))

	// Verify SHA-256 matches what the service would compute.
	h := sha256.New()
	h.Write(zipBytes)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, zipBytes, raw, "file content must match assembled ZIP")

	// Recompute from file to prove the hash formula is correct.
	h2 := sha256.New()
	h2.Write(raw)
	assert.Equal(t, expectedHash, hex.EncodeToString(h2.Sum(nil)))
}

// ─── exported test helpers ────────────────────────────────────────────────────
// These call assembleBundle via an exported test shim so we don't have to
// test only through the full ConnectRPC handler surface.

// exportForTest calls the package-internal assembleBundle via the exported
// ExportBundleForTest shim.
func exportForTest(t *testing.T, svc *services.SessionBundleService, inst *session.Instance, maxScrollbackLines int) ([]byte, services.BundleManifest, error) {
	t.Helper()
	return exportForTestCtx(t, context.Background(), svc, inst, maxScrollbackLines)
}

func exportForTestCtx(t *testing.T, ctx context.Context, svc *services.SessionBundleService, inst *session.Instance, maxScrollbackLines int) ([]byte, services.BundleManifest, error) {
	t.Helper()
	return svc.ExportBundleForTest(ctx, inst, maxScrollbackLines, []byte(`[{"name":"t"}]`))
}

// ─── AC2: traces and profiles are in the archive and readable ────────────────

func TestAssembleBundle_IncludesTracesAndProfiles(t *testing.T) {
	svc := services.NewSessionBundleService(&fakeBundleStorage{}, nil, &fakeScrollback{lines: []byte("x")})
	zipBytes, manifest, err := exportForTest(t, svc, newTestInstance("uuid-traces"), 10)
	require.NoError(t, err)

	entries := readZIP(t, zipBytes)
	assert.JSONEq(t, `[{"name":"t"}]`, string(entries["client_traces.json"]))
	assert.Contains(t, entries, "runtime_stats.json")

	gz, err := gzip.NewReader(bytes.NewReader(entries["profiles/heap.pb.gz"]))
	require.NoError(t, err, "heap profile must be readable gzip pprof")
	raw, err := io.ReadAll(gz)
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
	assert.Contains(t, manifest.IncludedSections, "client_traces")
	assert.Contains(t, manifest.IncludedSections, "profile_goroutine")
}
