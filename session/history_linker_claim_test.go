package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	claimSibling  = "cccccccc-0000-0000-0000-000000000000"
	claimOther    = "dddddddd-0000-0000-0000-000000000000"
	claimOldStart = "eeeeeeee-0000-0000-0000-000000000000"
	claimOwnStart = "ffffffff-0000-0000-0000-000000000000"
)

type claimFixture struct {
	linker *HistoryLinker
	dir    string
	now    time.Time
}

func newClaimFixture(t *testing.T, repoPath string) *claimFixture {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", ClaudeProjectDirName(repoPath))
	require.NoError(t, os.MkdirAll(dir, 0755))
	detector := NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, home)
	return &claimFixture{linker: NewHistoryLinker(detector, nil), dir: dir, now: time.Now()}
}

// write creates <uuid>.jsonl with the given content and mtime = now - age.
func (f *claimFixture) write(t *testing.T, uuid, content string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(f.dir, uuid+".jsonl")
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	mt := f.now.Add(-age)
	require.NoError(t, os.Chtimes(p, mt, mt))
	return p
}

func stampedEntry(ts time.Time) string {
	return `{"type":"queue-operation","timestamp":"` + ts.UTC().Format(time.RFC3339Nano) + `"}` + "\n"
}

func TestHistoryLinker_SkipsConversationOwnedBySibling(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/shared")
	f.write(t, claimOther, "{}", 2*time.Second)
	siblingPath := f.write(t, claimSibling, "{}", time.Second) // newest

	sibling := &Instance{Title: "sibling", Path: "/repo/shared", Status: Running}
	sibling.SetHistoryInfo(claimSibling, siblingPath)
	fresh := &Instance{Title: "fresh", Path: "/repo/shared", Status: Running}
	f.linker.SetInstances([]*Instance{sibling, fresh})

	f.linker.correlateSession(fresh, false)

	require.True(t, fresh.HasClaudeSession())
	assert.Equal(t, claimOther, fresh.GetClaudeSession().ConversationUUID)
}

func TestHistoryLinker_AllConversationsClaimed_StaysUnlinked(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/shared")
	siblingPath := f.write(t, claimSibling, "{}", time.Second)

	sibling := &Instance{Title: "sibling", Path: "/repo/shared", Status: Running}
	sibling.SetHistoryInfo(claimSibling, siblingPath)
	lonely := &Instance{Title: "lonely", Path: "/repo/shared", Status: Running}
	f.linker.SetInstances([]*Instance{sibling, lonely})

	f.linker.correlateSession(lonely, false)

	assert.False(t, lonely.HasClaudeSession())
}

func TestHistoryLinker_LinkedSession_KeepsOwnConversation(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/shared")
	ownPath := f.write(t, claimOther, "{}", time.Second) // newest

	inst := &Instance{Title: "self", Path: "/repo/shared", Status: Running}
	inst.SetHistoryInfo(claimOther, ownPath)
	f.linker.SetInstances([]*Instance{inst})

	f.linker.correlateSession(inst, true)

	assert.Equal(t, claimOther, inst.GetClaudeSession().ConversationUUID, "own file must not count as claimed")
}

func TestHistoryLinker_LinkedSession_ClearMovesToUnclaimedNotSiblingFile(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/shared")
	oldPath := f.write(t, claimOldStart, "{}", time.Hour)
	f.write(t, claimOther, "{}", 2*time.Second)                // post-/clear conversation
	siblingPath := f.write(t, claimSibling, "{}", time.Second) // newest, owned by sibling

	sibling := &Instance{Title: "sibling", Path: "/repo/shared", Status: Running}
	sibling.SetHistoryInfo(claimSibling, siblingPath)
	inst := &Instance{Title: "cleared", Path: "/repo/shared", Status: Running, EverHadConversationHistory: true}
	inst.SetHistoryInfo(claimOldStart, oldPath)
	f.linker.SetInstances([]*Instance{sibling, inst})

	f.linker.correlateSession(inst, true)

	assert.Equal(t, claimOther, inst.GetClaudeSession().ConversationUUID)
}

// Original incident: the newest file is an unclaimed conversation that began
// before the new session existed.
func TestHistoryLinker_NeverLinked_SkipsConversationPredatingSession(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/predate")
	created := f.now.Add(-time.Minute)
	f.write(t, claimOwnStart, stampedEntry(created.Add(10*time.Second)), 5*time.Second)
	f.write(t, claimOldStart, stampedEntry(created.Add(-time.Hour)), time.Second) // newest mtime

	fresh := &Instance{Title: "fresh", Path: "/repo/predate", Status: Running, CreatedAt: created}
	f.linker.SetInstances([]*Instance{fresh})

	f.linker.correlateSession(fresh, false)

	require.True(t, fresh.HasClaudeSession())
	assert.Equal(t, claimOwnStart, fresh.GetClaudeSession().ConversationUUID)
}

func TestHistoryLinker_PredatingConversation_AcceptedWhenNotFreshlyUnlinked(t *testing.T) {
	t.Parallel()
	created := time.Now().Add(-time.Minute)
	oldEntry := stampedEntry(created.Add(-time.Hour))

	tests := []struct {
		name string
		inst *Instance
		prep func(*Instance, string)
	}{
		{
			name: "EverHadConversationHistory",
			inst: &Instance{Title: "veteran", Path: "/repo/p", Status: Running, CreatedAt: created, EverHadConversationHistory: true},
		},
		{
			// resume_id sessions are pre-linked to the older conversation they resume.
			name: "linked via resume_id",
			inst: &Instance{Title: "resumed", Path: "/repo/p", Status: Running, CreatedAt: created},
			prep: func(i *Instance, path string) {
				i.claudeSession = &ClaudeSessionData{ConversationUUID: claimOther}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newClaimFixture(t, "/repo/p")
			path := f.write(t, claimOldStart, oldEntry, time.Second)
			if tt.prep != nil {
				tt.prep(tt.inst, path)
			}
			f.linker.SetInstances([]*Instance{tt.inst})

			f.linker.correlateSession(tt.inst, true)

			assert.Equal(t, claimOldStart, tt.inst.GetClaudeSession().ConversationUUID)
		})
	}
}

func TestHistoryLinker_StartBoundary_EqualToCreatedAtAccepted(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/edge")
	created := f.now.Add(-time.Minute).Truncate(time.Millisecond)
	f.write(t, claimOwnStart, stampedEntry(created), time.Second)

	fresh := &Instance{Title: "fresh", Path: "/repo/edge", Status: Running, CreatedAt: created}
	f.linker.SetInstances([]*Instance{fresh})
	f.linker.correlateSession(fresh, false)
	assert.True(t, fresh.HasClaudeSession())

	f2 := newClaimFixture(t, "/repo/edge")
	f2.write(t, claimOwnStart, stampedEntry(created.Add(-time.Millisecond)), time.Second)
	fresh2 := &Instance{Title: "fresh2", Path: "/repo/edge", Status: Running, CreatedAt: created}
	f2.linker.SetInstances([]*Instance{fresh2})
	f2.linker.correlateSession(fresh2, false)
	assert.False(t, fresh2.HasClaudeSession())
}

func TestJSONLStartTime(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 10, 16, 8, 0, 44, 889000000, time.UTC)
	stamped := `{"timestamp":"2026-10-16T08:00:44.889Z"}` + "\n"

	tests := []struct {
		name    string
		content string
		wantOK  bool
	}{
		{"first entry", stamped, true},
		{"skips untimestamped entries", "{}\n{\"type\":\"x\"}\n" + stamped, true},
		{"skips unparseable timestamp", `{"timestamp":"yesterday"}` + "\n" + stamped, true},
		{"empty file", "", false},
		{"malformed json", "not json\n", false},
		{"no timestamps", "{}\n{}\n", false},
		{"timestamp past entry scan limit", strings.Repeat("{}\n", jsonlStartScanEntries) + stamped, false},
		{"first entry beyond read cap", `{"pad":"` + strings.Repeat("x", jsonlStartScanBytes) + `"}` + "\n" + stamped, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := filepath.Join(t.TempDir(), "c.jsonl")
			require.NoError(t, os.WriteFile(p, []byte(tt.content), 0644))
			got, ok := jsonlStartTime(p)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				assert.True(t, got.Equal(want), "got %s", got)
			}
		})
	}

	_, ok := jsonlStartTime(filepath.Join(t.TempDir(), "missing.jsonl"))
	assert.False(t, ok, "unreadable file fails open")
}

func TestHistoryLinker_UndatableConversation_AdoptedByFreshSession(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/undated")
	f.write(t, claimOwnStart, "{}\n", time.Second)

	fresh := &Instance{Title: "fresh", Path: "/repo/undated", Status: Running, CreatedAt: f.now.Add(-time.Minute)}
	f.linker.SetInstances([]*Instance{fresh})
	f.linker.correlateSession(fresh, false)

	assert.True(t, fresh.HasClaudeSession())
}
