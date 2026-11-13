package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
)

func TestSnapshotNonNilAfterNewInstance(t *testing.T) {
	t.Parallel()
	inst, err := NewInstance(InstanceOptions{
		Title:   "snapshot-test-new",
		Path:    t.TempDir(),
		Program: "echo",
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	if inst.Snapshot() == nil {
		t.Fatal("Snapshot() is nil after NewInstance")
	}
	if inst.Snapshot().Title != "snapshot-test-new" {
		t.Fatalf("Snapshot().Title = %q, want %q", inst.Snapshot().Title, "snapshot-test-new")
	}
}

func TestBuildSnapshot_ShouldCopyPRsDeeply_WhenPublished(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "pr-snap", Path: t.TempDir(), Program: "echo", Branch: "feat/x"}
	FinishInstanceConstruction(inst)
	ref := fixturetest.MustPRRef(t, "", "acme", "app", 12)
	inst.PRs = []domain.PRView{{StateID: 1, Ref: ref, State: domain.PRStateOpen, HeadBranch: "feat/x"}}
	inst.LegacyPRUnrepresented = true
	inst.HasPRLinks = true

	snap := buildSnapshot(inst)
	inst.PRs[0].State = domain.PRStateMerged // mutate the live set after the build

	if got := snap.GitHub.PRs[0].State; got != domain.PRStateOpen {
		t.Errorf("snapshot PR aliased the live slice: state = %q", got)
	}
	if !snap.GitHub.LegacyPRUnrepresented || !snap.GitHub.HasPRLinks {
		t.Errorf("flags not copied: %+v", snap.GitHub)
	}
	if snap.GitHub.GitHubPRNumber != 12 || snap.GitHub.GitHubPRURL != "https://github.com/acme/app/pull/12" {
		t.Errorf("legacy scalars = %d %q, want 12 + derived URL", snap.GitHub.GitHubPRNumber, snap.GitHub.GitHubPRURL)
	}

	// The flags are carried, never re-derived: no links but a true flag stays true.
	inst.PRs, inst.HasPRLinks = nil, true
	if !buildSnapshot(inst).GitHub.HasPRLinks {
		t.Error("HasPRLinks was re-derived from an empty PRs slice")
	}
}

func TestToInstanceData_ShouldCarryPRFlagsOut_WhenSnapshotHasLinks(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "pr-out", Path: t.TempDir(), Program: "echo"}
	inst.PRs = []domain.PRView{{StateID: 1, Ref: fixturetest.MustPRRef(t, "", "acme", "app", 12), State: domain.PRStateOpen}}
	inst.LegacyPRUnrepresented, inst.HasPRLinks = true, true
	FinishInstanceConstruction(inst)

	data := inst.ToInstanceData()
	if !data.LegacyPRUnrepresented || !data.HasPRLinks || len(data.PRs) != 1 {
		t.Errorf("ToInstanceData dropped PR state: %+v", data)
	}
	if data.GitHubPRNumber != 12 {
		t.Errorf("GitHubPRNumber = %d, want mirror 12", data.GitHubPRNumber)
	}
}
