package approvals

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Fixtures duplicated from server/services' tests, which a leaf cannot import.

// writeSettings writes content to <dir>/.claude/settings.local.json, creating the directory.
// Used by hook injector tests.
func writeSettings(t *testing.T, dir, content string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("writeSettings: mkdir %s: %v", claudeDir, err)
	}
	path := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeSettings: write %s: %v", path, err)
	}
}

// readSettings reads and top-level-parses <dir>/.claude/settings.local.json.
// Used by hook injector tests.
func readSettings(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	path := filepath.Join(dir, ".claude", "settings.local.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readSettings: read %s: %v", path, err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("readSettings: parse JSON: %v", err)
	}
	return result
}

type mirrorCounts struct {
	mu      sync.Mutex
	failed  []string
	cleared []string
}

func (c *mirrorCounts) onFailed(r string) { c.mu.Lock(); c.failed = append(c.failed, r); c.mu.Unlock() }
func (c *mirrorCounts) onCleared(r string) {
	c.mu.Lock()
	c.cleared = append(c.cleared, r)
	c.mu.Unlock()
}
func (c *mirrorCounts) failures() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.failed...)
}

// mirrorFlagOn pins askuserquestion_structured on for tests that probe the mirror itself.
func mirrorFlagOn() bool { return true }

// newRealStoreMirror lifts the 250 ms production bound: a race-instrumented
// SQLite write under a parallel test run can exceed it, which is not what these tests probe.
func newRealStoreMirror(storage *session.Storage, pending *questionkit.PendingQuestionStore, counts *mirrorCounts) *questionkit.GuidanceHookMirror {
	m := questionkit.NewGuidanceHookMirror(storage, pending, counts.onFailed, counts.onCleared, mirrorFlagOn)
	m.SetBound(10 * time.Second)
	return m
}

func guidanceRowOf(t *testing.T, s *session.Storage, id string) *domain.GuidanceRequestData {
	t.Helper()
	row, err := s.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(t, err)
	return row
}

// blockingMirrorStore blocks CreateMirroredGuidanceRequest until released.
type blockingMirrorStore struct {
	questionkit.GuidanceMirrorStore
	release chan struct{}
	err     error
}

func (b *blockingMirrorStore) CreateMirroredGuidanceRequest(ctx context.Context, req domain.GuidanceRequest, capLimit int) (*domain.GuidanceRequestData, error) {
	if b.release != nil {
		<-b.release
	}
	return nil, b.err
}

type stubReplier struct {
	out     *hookipc.HookOutput
	session string
	calls   int
}

func (s *stubReplier) Reply(_ context.Context, sessionUUID string, _ map[string]interface{}) *hookipc.HookOutput {
	s.calls++
	s.session = sessionUUID
	return s.out
}
