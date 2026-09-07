package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// goldenConflictAFixturePath is testdata/golden_conflict_a.txt, real git's own
// byte-for-byte conflict-marker output for this file's base/ours/theirs scenario
// (Epic 5.3, Story 5.3.2). Frozen once, by hand, from a real `git merge` run — see this
// test's doc comment for exactly how, so the fixture can be regenerated identically if
// it's ever lost.
const goldenConflictAFixturePath = "testdata/golden_conflict_a.txt"

// TestNativeMerge_ConflictMarkers_MatchGoldenFixture covers Story 5.3.2's acceptance
// criterion and validation.md's P5 golden-fixture requirement: the native merge
// pipeline's conflict-marker output for a frozen base/ours/theirs scenario must be
// byte-identical to real git's own output for the same scenario.
//
// The fixture (testdata/golden_conflict_a.txt) was generated once via a real `git
// merge` subprocess run, not hand-authored:
//
//	git init -b main && git config user.email/user.name
//	printf 'line1\n' > a.txt && git add a.txt && git commit -m base
//	git checkout -b ours
//	printf 'line1\nours\n' > a.txt && git add a.txt && git commit -m ours
//	git checkout main
//	printf 'line1\ntheirs\n' > a.txt && git add a.txt && git commit -m theirs
//	git update-ref refs/remotes/origin/main main   # simulates a fetched remote-tracking ref
//	git checkout ours
//	git -c merge.conflictStyle=merge merge --no-edit origin/main   # conflicts; a.txt is the fixture
//
// "-c merge.conflictStyle=merge" pins the generation run to git's own default conflict
// style (no `|||||||` base section — see ConflictMarkerStyle's doc comment) regardless
// of this checkout's local .git/config override, since MergeStyleDefault is the only
// style this project targets.
//
// This test reproduces the identical scenario through the real native pipeline —
// ThreeWayFileMerger.Merge (Epic 3.2's diff3 reconciliation) followed by
// assembleConflictedFileContent/renderConflictHunk (Epic 3.3's marker rendering) — using
// the same base/ours/theirs content and the same "HEAD"/"origin/main" labels
// materializeConflictOnAbort passes in production (native_merge.go), rather than
// hand-constructing the hunks the pipeline would produce.
func TestNativeMerge_ConflictMarkers_MatchGoldenFixture(t *testing.T) {
	t.Parallel()

	golden, err := os.ReadFile(filepath.Join("testdata", "golden_conflict_a.txt"))
	require.NoError(t, err)

	const (
		base   = "line1\n"
		ours   = "line1\nours\n"
		theirs = "line1\ntheirs\n"
	)

	var merger ThreeWayFileMerger
	result, err := merger.Merge(base, ours, theirs)
	require.NoError(t, err)
	require.NotEmpty(t, result.Conflicts(), "this scenario must produce a real conflict, matching the golden fixture's generation run")

	// oursLabel/theirsLabel match materializeConflictOnAbort's exact production call
	// (native_merge.go): "HEAD" and "origin/"+mainBranch.
	content, err := assembleConflictedFileContent(result.Hunks, "HEAD", "origin/main")
	require.NoError(t, err)

	require.Equal(t, string(golden), content, "native conflict-marker output must be byte-identical to real git's own output")
}

// goldenConflictEmptyTheirsFixturePath is testdata/golden_conflict_empty_theirs.txt, real
// git's byte-for-byte output for a conflict where one side (theirs) collapses the file to
// empty content — the "delete/modify"-shaped case from PR #730 Gate 2 Blocker 2:
// renderConflictHunk used to unconditionally append "\n" after joining a hunk side, even
// when that side had zero lines, producing an extra blank line between "=======" and
// ">>>>>>> origin/main" that real git never emits.
const goldenConflictEmptyTheirsFixturePath = "testdata/golden_conflict_empty_theirs.txt"

// TestNativeMerge_ConflictMarkers_EmptySide_MatchesGoldenFixture_NoExtraBlankLine covers
// Gate 2 Blocker 2's regression test: a hunk whose Theirs side has zero lines must render
// with no blank line before the closing marker, byte-identical to real git.
//
// The fixture was generated the same way as goldenConflictAFixturePath (see that test's
// doc comment for the general recipe), with this scenario's specific commands:
//
//	git init -b main && git config user.email/user.name
//	printf 'line1\nsecond\n' > a.txt && git add a.txt && git commit -m base
//	git checkout -b feature
//	printf 'line1\nours change\n' > a.txt && git add a.txt && git commit -m ours
//	git checkout main
//	printf '' > a.txt && git add a.txt && git commit -m theirs-empties
//	git update-ref refs/remotes/origin/main main
//	git checkout feature
//	git -c merge.conflictStyle=merge merge --no-edit origin/main   # conflicts; a.txt is the fixture
func TestNativeMerge_ConflictMarkers_EmptySide_MatchesGoldenFixture_NoExtraBlankLine(t *testing.T) {
	t.Parallel()

	golden, err := os.ReadFile(filepath.Join("testdata", "golden_conflict_empty_theirs.txt"))
	require.NoError(t, err)

	const (
		base   = "line1\nsecond\n"
		ours   = "line1\nours change\n"
		theirs = ""
	)

	var merger ThreeWayFileMerger
	result, err := merger.Merge(base, ours, theirs)
	require.NoError(t, err)
	require.NotEmpty(t, result.Conflicts(), "this scenario must produce a real conflict, matching the golden fixture's generation run")
	require.Empty(t, result.Conflicts()[0].Theirs, "test setup: theirs side must be the empty hunk this regression test targets")

	content, err := assembleConflictedFileContent(result.Hunks, "HEAD", "origin/main")
	require.NoError(t, err)

	require.Equal(t, string(golden), content, "native conflict-marker output must be byte-identical to real git's own output, with no extra blank line for the empty side")
}
