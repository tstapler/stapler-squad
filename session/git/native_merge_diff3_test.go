package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
)

// --- Story 3.2.1: line-level diff ---

func TestLineDiff_SingleLineEdit(t *testing.T) {
	t.Parallel()

	diffs := lineDiff("a\nb\nc\n", "a\nB\nc\n")

	var deletes, inserts []string
	for _, d := range diffs {
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			deletes = append(deletes, d.Text)
		case diffmatchpatch.DiffInsert:
			inserts = append(inserts, d.Text)
		}
	}
	require.Equal(t, []string{"b\n"}, deletes, "line 2 (\"b\") is the sole deleted line")
	require.Equal(t, []string{"B\n"}, inserts, "line 2 (\"B\") is the sole inserted line")
}

// --- Story 3.2.2: MergeRegionKind classification ---

func TestThreeWayFileMerger_OneSidedEdit_AutoResolves(t *testing.T) {
	t.Parallel()
	var merger ThreeWayFileMerger

	result, err := merger.Merge("a\nb\nc\n", "a\nB\nc\n", "a\nb\nc\n")
	require.NoError(t, err)
	require.Empty(t, result.Conflicts())
	require.Equal(t, "a\nB\nc\n", result.Content)
}

func TestThreeWayFileMerger_TwoSidedEdit_Conflicts(t *testing.T) {
	t.Parallel()
	var merger ThreeWayFileMerger

	result, err := merger.Merge("a\nb\nc\n", "a\nB\nc\n", "a\nX\nc\n")
	require.NoError(t, err)

	conflicts := result.Conflicts()
	require.Len(t, conflicts, 1)
	require.Equal(t, RegionConflict, conflicts[0].Kind)
	require.Equal(t, []string{"B"}, conflicts[0].Ours)
	require.Equal(t, []string{"X"}, conflicts[0].Theirs)
}

func TestThreeWayFileMerger_IdenticalEdits_AutoResolves(t *testing.T) {
	t.Parallel()
	var merger ThreeWayFileMerger

	result, err := merger.Merge("a\nb\nc\n", "a\nB\nc\n", "a\nB\nc\n")
	require.NoError(t, err)
	require.Empty(t, result.Conflicts(), "both sides making the identical edit must not conflict")
	require.Equal(t, "a\nB\nc\n", result.Content)
}

func TestThreeWayFileMerger_should_ReturnError_When_ContentIsNotValidUTF8AndNotFlaggedBinary(t *testing.T) {
	t.Parallel()
	var merger ThreeWayFileMerger

	// A lone continuation byte (0x80) is invalid UTF-8 but contains no NUL
	// byte, so MergeFile's binary heuristic would not have caught it either
	// — Merge itself must still refuse to silently split/truncate it.
	invalid := "a\n\x80b\n"

	_, err := merger.Merge("a\nb\n", invalid, "a\nb\n")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not valid UTF-8")
}

// TestThreeWayFileMerger_RenameModifyCollision_TreatedAsIndependentChanges_NotConflict
// builds a real base/ours/theirs tree triple via go-git (base has a.txt; ours renames
// it to b.txt with unchanged content; theirs modifies a.txt in place) and pins the
// "Rename+modify merge collision" Pattern Decision: this project does not correlate
// the two sides' changes into a detected rename+modify conflict, it reconciles
// whatever go-git's own TreeDiffPair independently reports per path.
func TestThreeWayFileMerger_RenameModifyCollision_TreatedAsIndependentChanges_NotConflict(t *testing.T) {
	t.Parallel()
	f := newMergeBaseFixture(t)
	sig := &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()}

	baseCommit := f.commitFile("a.txt", "line1\n", "base")

	wt, err := f.repo.Worktree()
	require.NoError(t, err)

	// ours: rename a.txt -> b.txt, content unchanged.
	require.NoError(t, wt.Checkout(&git.CheckoutOptions{
		Hash: baseCommit.Hash, Branch: plumbing.NewBranchReferenceName("ours"), Create: true,
	}))
	_, err = wt.Remove("a.txt")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "b.txt"), []byte("line1\n"), 0o600))
	_, err = wt.Add("b.txt")
	require.NoError(t, err)
	oursHash, err := wt.Commit("ours: rename a.txt -> b.txt", &git.CommitOptions{Author: sig})
	require.NoError(t, err)
	oursCommit, err := f.repo.CommitObject(oursHash)
	require.NoError(t, err)

	// theirs: modify a.txt in place, from the same base.
	require.NoError(t, wt.Checkout(&git.CheckoutOptions{
		Hash: baseCommit.Hash, Branch: plumbing.NewBranchReferenceName("theirs"), Create: true, Force: true,
	}))
	theirsCommit := f.commitFile("a.txt", "line1\nline2\n", "theirs: modify a.txt")

	baseTree, err := baseCommit.Tree()
	require.NoError(t, err)
	oursTree, err := oursCommit.Tree()
	require.NoError(t, err)
	theirsTree, err := theirsCommit.Tree()
	require.NoError(t, err)

	baseToOurs, baseToTheirs, err := TreeDiffPair(baseTree, oursTree, theirsTree)
	require.NoError(t, err)

	byPath, err := GroupChangesByPath(baseToOurs, baseToTheirs)
	require.NoError(t, err)
	require.Contains(t, byPath, "a.txt")
	require.Contains(t, byPath, "b.txt")

	var merger ThreeWayFileMerger

	aResult, err := merger.ReconcilePathChange(byPath["a.txt"])
	require.NoError(t, err)
	require.Empty(t, aResult.Conflicts(), "a.txt must not become a rename+modify conflict")
	require.Equal(t, "line1\nline2\n", aResult.Content, "a.txt resolves to theirs' modification")

	bResult, err := merger.ReconcilePathChange(byPath["b.txt"])
	require.NoError(t, err)
	require.Empty(t, bResult.Conflicts(), "b.txt must not become a rename+modify conflict")
	require.Equal(t, "line1\n", bResult.Content, "b.txt resolves to ours' Add")
}

// --- Story 3.2.3: mode-only / binary / gitlink conflict classification ---

func TestThreeWayFileMerger_ModeOnlyConflict(t *testing.T) {
	t.Parallel()
	var merger ThreeWayFileMerger

	content := []byte("#!/bin/sh\necho hi\n")
	outcome, err := merger.MergeFile(FileMergeInput{
		BaseMode: filemode.Regular, OursMode: filemode.Executable, TheirsMode: filemode.Regular,
		BaseContent: content, OursContent: content, TheirsContent: content,
	})
	require.NoError(t, err)
	require.Equal(t, ReasonModeConflict, outcome.Reason)
	require.Nil(t, outcome.Result, "no content merge must be attempted for a mode conflict")
}

func TestThreeWayFileMerger_BinaryConflict_SkipsLineDiff(t *testing.T) {
	t.Parallel()
	var merger ThreeWayFileMerger

	base := []byte("\x00base-binary")
	ours := []byte("\x00ours-binary")
	theirs := []byte("\x00theirs-binary")

	outcome, err := merger.MergeFile(FileMergeInput{
		BaseMode: filemode.Regular, OursMode: filemode.Regular, TheirsMode: filemode.Regular,
		BaseContent: base, OursContent: ours, TheirsContent: theirs,
	})
	require.NoError(t, err)
	require.Equal(t, ReasonBinaryConflict, outcome.Reason)
	require.Nil(t, outcome.Result, "renderConflictHunk/line-diff must never run for a binary conflict")
}

func TestThreeWayFileMerger_GitlinkConflict_NeverContentMerged(t *testing.T) {
	t.Parallel()
	var merger ThreeWayFileMerger

	outcome, err := merger.MergeFile(FileMergeInput{
		BaseMode: filemode.Regular, OursMode: filemode.Submodule, TheirsMode: filemode.Regular,
		BaseContent: []byte("blob content\n"), OursContent: []byte("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"), TheirsContent: []byte("blob content\n"),
	})
	require.NoError(t, err)
	require.Equal(t, ReasonGitlinkConflict, outcome.Reason)
	require.Nil(t, outcome.Result, "a gitlink's commit-SHA entry must never be fed into line-diff or mode/binary diffing")
}
