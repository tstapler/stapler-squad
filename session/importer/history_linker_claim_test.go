package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

func TestHistoryLinker_SkipsConversationOwnedBySibling(t *testing.T) {
	t.Parallel()
	f := newLinkerFixture(t, "/repo/shared")
	f.write(t, claimOther, "{}", 2*time.Second)
	siblingPath := f.write(t, claimSibling, "{}", time.Second) // newest

	sibling := &session.Instance{Title: "sibling", Path: "/repo/shared", Status: session.Active}
	sibling.SetHistoryInfo(claimSibling, siblingPath)
	fresh := &session.Instance{Title: "fresh", Path: "/repo/shared", Status: session.Active}
	f.linker.SetInstances([]*session.Instance{sibling, fresh})

	f.linker.correlateSession(fresh, false)

	require.True(t, fresh.HasClaudeSession())
	assert.Equal(t, claimOther, fresh.GetClaudeSession().ConversationUUID)
}

func TestHistoryLinker_AllConversationsClaimed_StaysUnlinked(t *testing.T) {
	t.Parallel()
	f := newLinkerFixture(t, "/repo/shared")
	siblingPath := f.write(t, claimSibling, "{}", time.Second)

	sibling := &session.Instance{Title: "sibling", Path: "/repo/shared", Status: session.Active}
	sibling.SetHistoryInfo(claimSibling, siblingPath)
	lonely := &session.Instance{Title: "lonely", Path: "/repo/shared", Status: session.Active}
	f.linker.SetInstances([]*session.Instance{sibling, lonely})

	f.linker.correlateSession(lonely, false)

	assert.False(t, lonely.HasClaudeSession())
}

func TestHistoryLinker_LinkedSession_KeepsOwnConversation(t *testing.T) {
	t.Parallel()
	f := newLinkerFixture(t, "/repo/shared")
	ownPath := f.write(t, claimOther, "{}", time.Second) // newest

	inst := &session.Instance{Title: "self", Path: "/repo/shared", Status: session.Active}
	inst.SetHistoryInfo(claimOther, ownPath)
	f.linker.SetInstances([]*session.Instance{inst})

	f.linker.correlateSession(inst, true)

	assert.Equal(t, claimOther, inst.GetClaudeSession().ConversationUUID, "own file must not count as claimed")
}

func TestHistoryLinker_LinkedSession_ClearMovesToUnclaimedNotSiblingFile(t *testing.T) {
	t.Parallel()
	f := newLinkerFixture(t, "/repo/shared")
	oldPath := f.write(t, claimOldStart, "{}", time.Hour)
	f.write(t, claimOther, "{}", 2*time.Second)                // post-/clear conversation
	siblingPath := f.write(t, claimSibling, "{}", time.Second) // newest, owned by sibling

	sibling := &session.Instance{Title: "sibling", Path: "/repo/shared", Status: session.Active}
	sibling.SetHistoryInfo(claimSibling, siblingPath)
	inst := &session.Instance{Title: "cleared", Path: "/repo/shared", Status: session.Active, EverHadConversationHistory: true}
	inst.SetHistoryInfo(claimOldStart, oldPath)
	f.linker.SetInstances([]*session.Instance{sibling, inst})

	f.linker.correlateSession(inst, true)

	assert.Equal(t, claimOther, inst.GetClaudeSession().ConversationUUID)
}

// Original incident: the newest file is an unclaimed conversation that began
// before the new session existed.
func TestHistoryLinker_NeverLinked_SkipsConversationPredatingSession(t *testing.T) {
	t.Parallel()
	f := newLinkerFixture(t, "/repo/predate")
	created := f.now.Add(-time.Minute)
	f.write(t, claimOwnStart, stampedEntry(created.Add(10*time.Second)), 5*time.Second)
	f.write(t, claimOldStart, stampedEntry(created.Add(-time.Hour)), time.Second) // newest mtime

	fresh := &session.Instance{Title: "fresh", Path: "/repo/predate", Status: session.Active, CreatedAt: created}
	f.linker.SetInstances([]*session.Instance{fresh})

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
		inst *session.Instance
		prep func(*session.Instance, string)
	}{
		{
			name: "EverHadConversationHistory",
			inst: &session.Instance{Title: "veteran", Path: "/repo/p", Status: session.Active, CreatedAt: created, EverHadConversationHistory: true},
		},
		{
			// resume_id sessions are pre-linked to the older conversation they resume.
			name: "linked via resume_id",
			inst: &session.Instance{Title: "resumed", Path: "/repo/p", Status: session.Active, CreatedAt: created},
			prep: func(i *session.Instance, path string) {
				i.SetClaudeSession(&session.ClaudeSessionData{ConversationUUID: claimOther})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newLinkerFixture(t, "/repo/p")
			path := f.write(t, claimOldStart, oldEntry, time.Second)
			if tt.prep != nil {
				tt.prep(tt.inst, path)
			}
			f.linker.SetInstances([]*session.Instance{tt.inst})

			f.linker.correlateSession(tt.inst, true)

			assert.Equal(t, claimOldStart, tt.inst.GetClaudeSession().ConversationUUID)
		})
	}
}

func TestHistoryLinker_StartBoundary_EqualToCreatedAtAccepted(t *testing.T) {
	t.Parallel()
	f := newLinkerFixture(t, "/repo/edge")
	created := f.now.Add(-time.Minute).Truncate(time.Millisecond)
	f.write(t, claimOwnStart, stampedEntry(created), time.Second)

	fresh := &session.Instance{Title: "fresh", Path: "/repo/edge", Status: session.Active, CreatedAt: created}
	f.linker.SetInstances([]*session.Instance{fresh})
	f.linker.correlateSession(fresh, false)
	assert.True(t, fresh.HasClaudeSession())

	f2 := newLinkerFixture(t, "/repo/edge")
	f2.write(t, claimOwnStart, stampedEntry(created.Add(-time.Millisecond)), time.Second)
	fresh2 := &session.Instance{Title: "fresh2", Path: "/repo/edge", Status: session.Active, CreatedAt: created}
	f2.linker.SetInstances([]*session.Instance{fresh2})
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
		{"timestamp past entry scan limit", strings.Repeat("{}\n", session.JsonlStartScanEntries) + stamped, false},
		{"first entry beyond read cap", `{"pad":"` + strings.Repeat("x", session.JsonlStartScanBytes) + `"}` + "\n" + stamped, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := filepath.Join(t.TempDir(), "c.jsonl")
			require.NoError(t, os.WriteFile(p, []byte(tt.content), 0644))
			got, ok := session.JsonlStartTime(p)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				assert.True(t, got.Equal(want), "got %s", got)
			}
		})
	}

	_, ok := session.JsonlStartTime(filepath.Join(t.TempDir(), "missing.jsonl"))
	assert.False(t, ok, "unreadable file fails open")
}

func TestHistoryLinker_UndatableConversation_AdoptedByFreshSession(t *testing.T) {
	t.Parallel()
	f := newLinkerFixture(t, "/repo/undated")
	f.write(t, claimOwnStart, "{}\n", time.Second)

	fresh := &session.Instance{Title: "fresh", Path: "/repo/undated", Status: session.Active, CreatedAt: f.now.Add(-time.Minute)}
	f.linker.SetInstances([]*session.Instance{fresh})
	f.linker.correlateSession(fresh, false)

	assert.True(t, fresh.HasClaudeSession())
}

// linkerFixture adds a HistoryLinker over the claim fixture's detector.
type linkerFixture struct {
	*claimFixture
	linker *HistoryLinker
}

func newLinkerFixture(t *testing.T, repoPath string) *linkerFixture {
	t.Helper()
	f := newClaimFixture(t, repoPath)
	return &linkerFixture{claimFixture: f, linker: NewHistoryLinker(f.detector, nil)}
}
