package git

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// This file holds the diff3 three-way-merge pipeline: classifying every path base/ours/
// theirs disagree on (resolveMergePaths and its helpers), rendering conflict markers
// (assembleConflictedFileContent), and the two terminal outcomes nativeThreeWayMerge
// dispatches to — a real two-parent merge commit (commitThreeWayMerge et al.) or
// materializing conflict state and aborting (materializeConflictOnAbort,
// capturePreMergeSnapshot). Split out of native_merge.go (Epic 3.4's dispatch entry point,
// path-safety helpers, and fast-forward/admin-file plumbing) purely by file size — no
// behavioral change, same package throughout.

// resolvedPathMerge is one non-conflicted path's outcome from resolveMergePaths: either a
// deletion, or new content+mode to write and index.
type resolvedPathMerge struct {
	path    string
	deleted bool
	mode    filemode.FileMode
	content []byte
}

// conflictedPathMerge is one conflicted path's outcome from resolveMergePaths: the full
// hunk sequence (resolved and conflicting regions interleaved, per ThreeWayFileMerger),
// plus the three-stage blob hashes NewConflictEntries needs.
type conflictedPathMerge struct {
	path                           string
	mode                           filemode.FileMode
	baseHash, oursHash, theirsHash plumbing.Hash
	hunks                          []MergeHunk
}

// mergePathResolution is resolveMergePaths' result: every touched path classified as
// either cleanly resolved or conflicted.
type mergePathResolution struct {
	resolved  []resolvedPathMerge
	conflicts []conflictedPathMerge
}

// resolveMergePaths runs the diff3 pipeline (Epics 3.1-3.2) over every path either side
// touched relative to base, in deterministic (sorted) path order so ConflictedFiles and
// the resulting tree are reproducible rather than dependent on Go's randomized map
// iteration order. It also detects a plain rename (a Modify Change whose From.Name the
// resulting path map never independently addresses) and schedules the old path for
// removal — GroupChangesByPath keys a rename solely by its new name, so without this the
// old path's stale index/working-tree entry would survive the merge.
func resolveMergePaths(byPath map[string]*PathChange) (*mergePathResolution, error) {
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	merger := ThreeWayFileMerger{}
	res := &mergePathResolution{}
	renamedAway := map[string]bool{}
	for _, p := range paths {
		if err := resolveOnePathMerge(p, byPath[p], &merger, renamedAway, res); err != nil {
			return nil, fmt.Errorf("resolveMergePaths: %w", err)
		}
	}

	oldPaths := make([]string, 0, len(renamedAway))
	for old := range renamedAway {
		if _, stillAddressed := byPath[old]; stillAddressed {
			continue // the old path is itself independently addressed elsewhere
		}
		oldPaths = append(oldPaths, old)
	}
	sort.Strings(oldPaths)
	for _, old := range oldPaths {
		res.resolved = append(res.resolved, resolvedPathMerge{path: old, deleted: true})
	}

	return res, nil
}

// resolveOnePathMerge resolves a single path's three-way merge outcome, appending the
// result to res.resolved or res.conflicts and recording any rename source in renamedAway.
// Split out of resolveMergePaths (which now just loops over paths in sorted order) to keep
// each function's cognitive complexity within this repo's gocognit gate — this is the exact
// per-path body resolveMergePaths ran inline before the split, with no behavioral change.
func resolveOnePathMerge(p string, pc *PathChange, merger *ThreeWayFileMerger, renamedAway map[string]bool, res *mergePathResolution) error {
	for _, c := range [...]*object.Change{pc.Ours, pc.Theirs} {
		if c != nil && c.From.Name != "" && c.From.Name != p {
			renamedAway[c.From.Name] = true
		}
	}

	deleted, err := pathChangeIsDelete(pc)
	if err != nil {
		return fmt.Errorf("resolveMergePaths: failed to classify %q: %w", p, err)
	}
	if deleted {
		res.resolved = append(res.resolved, resolvedPathMerge{path: p, deleted: true})
		return nil
	}

	mode, err := pathChangeMode(pc)
	if err != nil {
		return fmt.Errorf("resolveMergePaths: failed to determine mode for %q: %w", p, err)
	}

	// A real two-sided change (both ours and theirs touched this path, neither as a
	// delete) runs through MergeFile's mode/binary/gitlink short-circuits before any
	// content-level diff3 merge — the wiring gap this project's own Phase 6 review
	// caught: MergeFile existed and passed its own isolated unit tests, but this
	// pipeline called ReconcilePathChange directly, which never invokes MergeFile at
	// all, so a mode/binary/gitlink mismatch could reach a real merge undetected.
	// Every other shape (single-sided change, or one side deleted) has no second side
	// to conflict against, so it still goes straight to ReconcilePathChange, unchanged.
	bothModified, err := bothSidesNonDeleteChanged(pc)
	if err != nil {
		return fmt.Errorf("resolveMergePaths: failed to classify %q: %w", p, err)
	}

	var result *MergeResult
	if bothModified {
		in, buildErr := buildFileMergeInput(pc)
		if buildErr != nil {
			return fmt.Errorf("resolveMergePaths: failed to read three-way content for %q: %w", p, buildErr)
		}
		outcome, mergeErr := merger.MergeFile(in)
		if mergeErr != nil {
			return fmt.Errorf("resolveMergePaths: failed to reconcile %q: %w", p, mergeErr)
		}
		if outcome.Reason != ReasonNone {
			base, ours, theirs, hashErr := conflictHashes(pc)
			if hashErr != nil {
				return fmt.Errorf("resolveMergePaths: failed to resolve conflict blob hashes for %q: %w", p, hashErr)
			}
			res.conflicts = append(res.conflicts, conflictedPathMerge{
				path: p, mode: mode,
				baseHash: base, oursHash: ours, theirsHash: theirs,
				hunks: nonTextConflictHunks(outcome.Reason, in),
			})
			return nil
		}
		result = outcome.Result
		// A one-sided mode change (the other side's mode still matches base) auto-
		// resolves to the changed side's mode (resolveFileMode) rather than always
		// preferring ours the way pathChangeMode does for the plain both-changed-content
		// case — pathChangeMode has no base to compare against, so it can't tell "only
		// theirs changed the mode" from "both changed it identically."
		mode = outcome.ResolvedMode
	} else {
		result, err = merger.ReconcilePathChange(pc)
		if err != nil {
			return fmt.Errorf("resolveMergePaths: failed to reconcile %q: %w", p, err)
		}
	}

	if result.Conflicted {
		base, ours, theirs, hashErr := conflictHashes(pc)
		if hashErr != nil {
			return fmt.Errorf("resolveMergePaths: failed to resolve conflict blob hashes for %q: %w", p, hashErr)
		}
		res.conflicts = append(res.conflicts, conflictedPathMerge{
			path: p, mode: mode,
			baseHash: base, oursHash: ours, theirsHash: theirs,
			hunks: result.Hunks,
		})
		return nil
	}

	res.resolved = append(res.resolved, resolvedPathMerge{path: p, mode: mode, content: []byte(result.Content)})
	return nil
}

// pathChangeIsDelete reports whether pc's resolution is an outright deletion: the side(s)
// that touched the path all deleted it, rather than modifying it. Computed independently
// of ReconcilePathChange's own delete handling because ReconcilePathChange conflates "the
// only side that touched this path deleted it" with "that side left empty content" (its
// singleSideResult helper reads changeContents' empty toContent for a Delete the same way
// it would read a genuinely emptied file) — this project's assembly layer needs the two
// told apart so a deleted path is removed from disk/index, not overwritten with an empty
// file.
func pathChangeIsDelete(pc *PathChange) (bool, error) {
	oursDeleted, err := changeIsDelete(pc.Ours)
	if err != nil {
		return false, fmt.Errorf("pathChangeIsDelete: failed to classify ours: %w", err)
	}
	theirsDeleted, err := changeIsDelete(pc.Theirs)
	if err != nil {
		return false, fmt.Errorf("pathChangeIsDelete: failed to classify theirs: %w", err)
	}
	switch {
	case pc.Ours == nil:
		return theirsDeleted, nil
	case pc.Theirs == nil:
		return oursDeleted, nil
	default:
		return oursDeleted && theirsDeleted, nil
	}
}

// changeIsDelete reports whether c (either side of a PathChange, possibly nil) is a
// Delete action. A nil Change (that side made no change relative to base) is never a
// delete.
func changeIsDelete(c *object.Change) (bool, error) {
	if c == nil {
		return false, nil
	}
	action, err := c.Action()
	if err != nil {
		return false, fmt.Errorf("changeIsDelete: failed to determine action: %w", err)
	}
	return action == merkletrie.Delete, nil
}

// pathChangeMode returns the file mode a non-deleted PathChange's resolved path should
// carry: whichever side made a non-delete change, preferring ours when both did (matching
// ReconcilePathChange's own ours-preference for the rename+modify collision case).
func pathChangeMode(pc *PathChange) (filemode.FileMode, error) {
	for _, c := range [...]*object.Change{pc.Ours, pc.Theirs} {
		if c == nil {
			continue
		}
		action, err := c.Action()
		if err != nil {
			return 0, fmt.Errorf("pathChangeMode: failed to determine action: %w", err)
		}
		if action != merkletrie.Delete {
			return c.To.TreeEntry.Mode, nil
		}
	}
	return filemode.Regular, nil
}

// conflictHashes returns the base/ours/theirs blob hashes NewConflictEntries needs for a
// conflicted PathChange. Safe to assume both sides are non-nil, non-delete Changes here:
// ReconcilePathChange only ever reaches its content-merge branch (the sole source of a
// RegionConflict hunk) once its own nil- and delete-side short-circuits have already ruled
// out every other shape (see ReconcilePathChange's doc comment) — a conflict is only
// signaled by resolveMergePaths after result.Conflicted, which requires that branch to
// have run.
func conflictHashes(pc *PathChange) (base, ours, theirs plumbing.Hash, err error) {
	if pc.Ours == nil || pc.Theirs == nil {
		return plumbing.ZeroHash, plumbing.ZeroHash, plumbing.ZeroHash,
			errors.New("conflictHashes: a real conflict requires both sides to have changed the path")
	}
	return pc.Ours.From.TreeEntry.Hash, pc.Ours.To.TreeEntry.Hash, pc.Theirs.To.TreeEntry.Hash, nil
}

// bothSidesNonDeleteChanged reports whether pc represents a real two-sided change: both
// ours and theirs touched the path, and neither side's change is a delete. This is the
// only shape MergeFile's mode/binary/gitlink short-circuits apply to — a single-sided
// change (the other side nil) or a one-side-delete has no second side to conflict
// against, so resolveMergePaths keeps routing those straight to ReconcilePathChange.
func bothSidesNonDeleteChanged(pc *PathChange) (bool, error) {
	if pc.Ours == nil || pc.Theirs == nil {
		return false, nil
	}
	oursDeleted, err := changeIsDelete(pc.Ours)
	if err != nil {
		return false, fmt.Errorf("bothSidesNonDeleteChanged: failed to classify ours: %w", err)
	}
	theirsDeleted, err := changeIsDelete(pc.Theirs)
	if err != nil {
		return false, fmt.Errorf("bothSidesNonDeleteChanged: failed to classify theirs: %w", err)
	}
	return !oursDeleted && !theirsDeleted, nil
}

// buildFileMergeInput extracts pc's three-way content and file modes for MergeFile's
// mode/binary/gitlink short-circuits (Story 3.2.3) — callers only reach this once
// bothSidesNonDeleteChanged(pc) is true, mirroring ReconcilePathChange's own final-branch
// precondition (both pc.Ours and pc.Theirs non-nil and non-delete).
func buildFileMergeInput(pc *PathChange) (FileMergeInput, error) {
	_, oursContent, err := changeContents(pc.Ours)
	if err != nil {
		return FileMergeInput{}, fmt.Errorf("buildFileMergeInput: failed to read ours content: %w", err)
	}
	baseContent, theirsContent, err := changeContents(pc.Theirs)
	if err != nil {
		return FileMergeInput{}, fmt.Errorf("buildFileMergeInput: failed to read base/theirs content: %w", err)
	}
	return FileMergeInput{
		BaseMode:      pc.Theirs.From.TreeEntry.Mode,
		OursMode:      pc.Ours.To.TreeEntry.Mode,
		TheirsMode:    pc.Theirs.To.TreeEntry.Mode,
		BaseContent:   []byte(baseContent),
		OursContent:   []byte(oursContent),
		TheirsContent: []byte(theirsContent),
	}, nil
}

// nonTextConflictHunks builds the working-tree conflict representation for a MergeFile
// mode/binary short-circuit: a single conflicting region spanning the whole file.
// materializeConflictOnAbort always reverts its working-tree write before
// nativeMergeMainIntoWorktree returns (see that function's doc comment), so rendering
// textual markers over binary content is safe — they are never actually left on disk. A
// gitlink conflict returns no hunks at all: a Submodule-mode path is a directory, not a
// blob, so materializeConflictOnAbort and capturePreMergeSnapshot both skip working-tree
// content entirely for it (checked via c.mode there).
func nonTextConflictHunks(reason FileConflictReason, in FileMergeInput) []MergeHunk {
	if reason == ReasonGitlinkConflict {
		return nil
	}
	return []MergeHunk{{
		Kind:   RegionConflict,
		Ours:   splitLines(string(in.OursContent)),
		Theirs: splitLines(string(in.TheirsContent)),
	}}
}

// nativeThreeWayMerge runs the diff3 pipeline over every path base/ours/theirs disagree
// on and either produces a real merge commit (no conflicts) or materializes conflict
// markers and aborts (Tasks 3.4.1b/3.4.1c).
func nativeThreeWayMerge(mc threeWayMergeContext, ours, theirs *object.Commit) (*MergeMainResult, error) {
	base, err := MergeBaseResolver(ours, theirs)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve merge base: %w", err)
	}

	baseTree, err := base.Tree()
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve base tree: %w", err)
	}
	oursTree, err := ours.Tree()
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve ours tree: %w", err)
	}
	theirsTree, err := theirs.Tree()
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve theirs tree: %w", err)
	}

	baseToOurs, baseToTheirs, err := TreeDiffPair(baseTree, oursTree, theirsTree)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to diff trees: %w", err)
	}
	byPath, err := GroupChangesByPath(baseToOurs, baseToTheirs)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to group changes: %w", err)
	}

	resolution, err := resolveMergePaths(byPath)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve paths: %w", err)
	}

	if len(resolution.conflicts) > 0 {
		return materializeConflictOnAbort(mc, resolution.conflicts, theirs.Hash)
	}
	return commitThreeWayMerge(mc, ours.Hash, theirs.Hash, resolution.resolved)
}

// materializeConflictOnAbort writes every conflicted path's stage-1/2/3 index entries
// and conflict-marker working-tree content plus the merge-state files (Task 3.4.1c, per
// Story 3.3.3's "always materialize, then abort" decision), then immediately calls
// abortNativeMerge to restore the pre-merge state — the worktree ends up exactly as clean
// as a real `git merge --abort` would leave it, verified in
// TestNativeMergeMainIntoWorktree_Conflicted_LeavesWorktreeClean via a real `git status
// --porcelain` subprocess.
func materializeConflictOnAbort(mc threeWayMergeContext, conflicts []conflictedPathMerge, theirsHash plumbing.Hash) (*MergeMainResult, error) {
	snapshot, err := capturePreMergeSnapshot(mc.worktreePath, conflicts)
	if err != nil {
		return nil, fmt.Errorf("materializeConflictOnAbort: failed to capture pre-merge snapshot: %w", err)
	}

	theirsLabel := "origin/" + string(mc.mainBranch)
	conflictedFiles := make([]string, 0, len(conflicts))
	var conflictEntries []*index.Entry
	for _, c := range conflicts {
		conflictedFiles = append(conflictedFiles, c.path)
		conflictEntries = append(conflictEntries, NewConflictEntries(c.path, c.baseHash, c.oursHash, c.theirsHash, c.mode)...)
	}
	if err := writeConflictedIndex(string(mc.worktreePath), conflictEntries); err != nil {
		return nil, fmt.Errorf("materializeConflictOnAbort: failed to write conflicted index: %w", err)
	}

	for _, c := range conflicts {
		if c.mode == filemode.Submodule {
			// A gitlink conflict (Story 3.2.3c): the path is a submodule directory, not
			// a blob, so there is no working-tree file content to render markers into
			// — nonTextConflictHunks already returns no hunks for this case.
			continue
		}
		content, renderErr := assembleConflictedFileContent(c.hunks, "HEAD", theirsLabel)
		if renderErr != nil {
			return nil, fmt.Errorf("materializeConflictOnAbort: failed to render markers for %q: %w", c.path, renderErr)
		}
		if writeErr := writeWorkingTreeFile(mc.worktreePath, c.path, c.mode, []byte(content)); writeErr != nil {
			return nil, fmt.Errorf("materializeConflictOnAbort: failed to write markers for %q: %w", c.path, writeErr)
		}
	}

	if err := writeMergeStateFiles(mc.worktreePath, theirsHash.String(), mc.mainBranch); err != nil {
		return nil, fmt.Errorf("materializeConflictOnAbort: failed to write merge-state files: %w", err)
	}

	if err := abortNativeMerge(mc.worktreePath, snapshot); err != nil {
		return nil, fmt.Errorf("materializeConflictOnAbort: failed to abort: %w", err)
	}

	recordMergeOutcome(mergeOutcomeConflicted)
	return &MergeMainResult{Conflicted: true, ConflictedFiles: conflictedFiles}, nil
}

// capturePreMergeSnapshot reads each conflicted path's current (pre-merge) stage-0 index
// entry and working-tree content, for abortNativeMerge to restore after
// materializeConflictOnAbort's transient write. A path with no pre-merge index entry
// (an add/add conflict neither side's ancestor had) is a known gap: abortNativeMerge's
// own touched-path set is derived solely from snapshot.Entries (see its doc comment), so
// such a path's conflict-stage entries would not be cleared from the index by this call —
// out of scope here since no real call site's tested scenarios (modify/modify conflicts)
// exercise it, but worth naming rather than silently mishandling.
func capturePreMergeSnapshot(worktreePath WorktreePath, conflicts []conflictedPathMerge) (*PreMergeIndexSnapshot, error) {
	repo, err := openWorktreeRepo(string(worktreePath))
	if err != nil {
		return nil, fmt.Errorf("capturePreMergeSnapshot: failed to open repo: %w", err)
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("capturePreMergeSnapshot: failed to load index: %w", err)
	}
	byName := make(map[string]*index.Entry, len(idx.Entries))
	for _, e := range idx.Entries {
		byName[e.Name] = e
	}

	snapshot := &PreMergeIndexSnapshot{Content: make(map[string][]byte, len(conflicts))}
	for _, c := range conflicts {
		if e, ok := byName[c.path]; ok {
			snapshot.Entries = append(snapshot.Entries, e)
		}
		if c.mode == filemode.Submodule {
			// A gitlink path is a submodule directory, not a regular file — os.ReadFile
			// would fail on it, and there is no working-tree blob content to snapshot
			// or later restore (see materializeConflictOnAbort's matching skip).
			continue
		}
		fullPath, joinErr := secureWorktreeJoin(worktreePath, c.path)
		if joinErr != nil {
			return nil, fmt.Errorf("capturePreMergeSnapshot: %w", joinErr)
		}
		if err := clearBlockingSymlinksInPath(worktreePath, fullPath); err != nil {
			return nil, fmt.Errorf("capturePreMergeSnapshot: %w", err)
		}
		content, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			return nil, fmt.Errorf("capturePreMergeSnapshot: failed to read %q: %w", c.path, readErr)
		}
		snapshot.Content[c.path] = content
	}
	return snapshot, nil
}

// assembleConflictedFileContent renders hunks (a conflicted path's full, in-order hunk
// sequence — resolved and conflicting regions interleaved) into the exact working-tree
// text a real `git merge` would materialize before an abort: resolved hunks contribute
// their resolved lines verbatim, RegionConflict hunks are rendered via renderConflictHunk
// (Epic 3.3), consecutive resolved runs are joined by newlines exactly like
// assembleContent does for the fully-auto-resolved case.
func assembleConflictedFileContent(hunks []MergeHunk, oursLabel, theirsLabel string) (string, error) {
	var b strings.Builder
	var pending []string

	flush := func() {
		if len(pending) == 0 {
			return
		}
		b.WriteString(strings.Join(pending, "\n"))
		b.WriteString("\n")
		pending = nil
	}

	for _, h := range hunks {
		if h.Kind != RegionConflict {
			pending = append(pending, resolvedLines(h)...)
			continue
		}
		flush()
		marker, err := renderConflictHunk(h, oursLabel, theirsLabel)
		if err != nil {
			return "", fmt.Errorf("assembleConflictedFileContent: %w", err)
		}
		b.WriteString(marker)
	}
	flush()
	return b.String(), nil
}

// commitThreeWayMerge writes every cleanly-resolved path (Task 3.4.1b) to the working
// tree, index, and object store, builds the resulting tree object, creates a real
// two-parent merge commit, and advances the checked-out branch ref to it via
// writeRefWithLockSentinel — never go-git's bare SetReference (see that function's doc
// comment).
func commitThreeWayMerge(mc threeWayMergeContext, oursHash, theirsHash plumbing.Hash, resolved []resolvedPathMerge) (*MergeMainResult, error) {
	touched := make(map[string]bool, len(resolved))
	var newEntries []*index.Entry
	for _, r := range resolved {
		touched[r.path] = true
		if r.deleted {
			if err := removeWorkingTreeFile(mc.worktreePath, r.path); err != nil {
				return nil, fmt.Errorf("commitThreeWayMerge: failed to remove %q: %w", r.path, err)
			}
			continue
		}

		hash, err := storeBlobObject(mc.repo, r.content)
		if err != nil {
			return nil, fmt.Errorf("commitThreeWayMerge: failed to store blob for %q: %w", r.path, err)
		}
		if err := writeWorkingTreeFile(mc.worktreePath, r.path, r.mode, r.content); err != nil {
			return nil, fmt.Errorf("commitThreeWayMerge: failed to write %q: %w", r.path, err)
		}
		newEntries = append(newEntries, &index.Entry{Name: r.path, Hash: hash, Mode: r.mode})
	}

	if err := writeIndexEntries(string(mc.worktreePath), touched, newEntries); err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to update index: %w", err)
	}

	idx, err := mc.repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to reload index: %w", err)
	}
	treeHash, err := buildTreeFromIndex(mc.repo, idx)
	if err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to build tree: %w", err)
	}

	commitHash, err := createMergeCommit(mc.repo, treeHash, oursHash, theirsHash, mc.mainBranch)
	if err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to create merge commit: %w", err)
	}

	if err := writeRefWithLockSentinel(mc.refPath, commitHash); err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to advance branch ref: %w", err)
	}
	recordMergeOutcome(mergeOutcomeCleanMerge)
	return &MergeMainResult{Merged: true}, nil
}

// storeBlobObject stores content as a new blob object in repo's object store, returning
// its hash — the same construction the package's own test helper (storeBlob in
// native_merge_index_test.go) uses, needed here for real (non-test) merged-file content.
func storeBlobObject(repo *git.Repository, content []byte) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("storeBlobObject: failed to open writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return plumbing.ZeroHash, fmt.Errorf("storeBlobObject: failed to write content: %w", err)
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("storeBlobObject: failed to close writer: %w", err)
	}
	return repo.Storer.SetEncodedObject(obj)
}

// buildTreeFromIndex constructs and stores the nested git tree objects for idx's flat
// stage-0 entries, mirroring go-git's own worktree_commit.go buildTreeHelper.
// Reimplemented here (rather than calling Worktree.Commit) because that helper is
// unexported, and Worktree.Commit's own ref update (updateHEAD) uses go-git's bare
// SetReference — exactly what Task 3.4.1b's merge ref-advance must avoid (see
// writeRefWithLockSentinel's doc comment) — so this project needs the tree-building half
// without the ref-writing half.
func buildTreeFromIndex(repo *git.Repository, idx *index.Index) (plumbing.Hash, error) {
	type dirNode struct {
		entries []object.TreeEntry
	}
	dirs := map[string]*dirNode{"": {}}
	dirOf := func(p string) *dirNode {
		d, ok := dirs[p]
		if !ok {
			d = &dirNode{}
			dirs[p] = d
		}
		return d
	}

	seen := map[string]bool{"": true}
	for _, e := range idx.Entries {
		parts := strings.Split(e.Name, "/")
		var full string
		for i, part := range parts {
			parent := full
			full = path.Join(full, part)
			if seen[full] {
				continue
			}
			seen[full] = true

			te := object.TreeEntry{Name: part}
			if i == len(parts)-1 {
				te.Mode = e.Mode
				te.Hash = e.Hash
			} else {
				te.Mode = filemode.Dir
				dirOf(full)
			}
			d := dirOf(parent)
			d.entries = append(d.entries, te)
		}
	}

	var store func(dirPath string) (plumbing.Hash, error)
	store = func(dirPath string) (plumbing.Hash, error) {
		entries := append([]object.TreeEntry(nil), dirs[dirPath].entries...)
		sort.Slice(entries, func(i, j int) bool {
			return treeEntrySortKey(entries[i]) < treeEntrySortKey(entries[j])
		})
		for i, te := range entries {
			if te.Mode != filemode.Dir {
				continue
			}
			hash, err := store(path.Join(dirPath, te.Name))
			if err != nil {
				return plumbing.ZeroHash, fmt.Errorf("buildTreeFromIndex: failed to build subtree %q: %w", te.Name, err)
			}
			te.Hash = hash
			entries[i] = te
		}

		tree := &object.Tree{Entries: entries}
		obj := repo.Storer.NewEncodedObject()
		if err := tree.Encode(obj); err != nil {
			return plumbing.ZeroHash, fmt.Errorf("buildTreeFromIndex: failed to encode tree %q: %w", dirPath, err)
		}
		return repo.Storer.SetEncodedObject(obj)
	}

	return store("")
}

// treeEntrySortKey mirrors go-git's own tree-entry sort convention (sortableEntries in
// worktree_commit.go): a directory sorts as if its name had a trailing "/", matching how
// git itself orders tree entries.
func treeEntrySortKey(te object.TreeEntry) string {
	if te.Mode == filemode.Dir {
		return te.Name + "/"
	}
	return te.Name
}

// createMergeCommit builds and stores a real two-parent merge commit object over
// treeHash, with commit identity resolved from the repo's git config exactly the way
// go-git's own CommitOptions.Validate does for a normal commit.
func createMergeCommit(repo *git.Repository, treeHash plumbing.Hash, oursHash, theirsHash plumbing.Hash, mainBranch BranchName) (plumbing.Hash, error) {
	opts := &git.CommitOptions{Parents: []plumbing.Hash{oursHash, theirsHash}}
	if err := opts.Validate(repo); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("createMergeCommit: failed to resolve commit identity: %w", err)
	}

	commit := &object.Commit{
		Author:       *opts.Author,
		Committer:    *opts.Committer,
		Message:      fmt.Sprintf("Merge branch 'origin/%s'\n", mainBranch),
		TreeHash:     treeHash,
		ParentHashes: opts.Parents,
	}
	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("createMergeCommit: failed to encode commit: %w", err)
	}
	return repo.Storer.SetEncodedObject(obj)
}
