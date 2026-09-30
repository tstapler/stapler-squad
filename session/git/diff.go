package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/binary"
	gogitdiff "github.com/go-git/go-git/v5/utils/diff"
	dmp "github.com/sergi/go-diff/diffmatchpatch"
)

// DiffStats holds statistics about the changes in a diff
type DiffStats struct {
	// Content is the full diff content
	Content string
	// Added is the number of added lines
	Added int
	// Removed is the number of removed lines
	Removed int
	// Error holds any error that occurred during diff computation
	// This allows propagating setup errors (like missing base commit) without breaking the flow
	Error error
}

func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && d.Content == ""
}

// resolveBaseCommitSHA finds a base commit SHA for diff by looking for the merge-base
// with common default branches. Used as a fallback when baseCommitSHA was not set at
// worktree creation time (e.g., sessions created with setupFromExistingBranch before the fix).
//
// Deliberately still a `git merge-base` subprocess rather than go-git's repo.Head() +
// Commit.MergeBase, despite the rest of this file having moved to go-git: go-git's
// direct HEAD-ref-file read has a documented torn-read race against a concurrent
// writer to the SAME worktree's HEAD (see getHeadCommitSHA's doc comment,
// session/git/util.go) — reproduced in production as a syntactically-valid but
// entirely nonexistent commit hash, not just a stale one. getHeadCommitSHA already
// hardens against that (retry + verify against the object store + CLI fallback) and
// could be reused here, but this specific call additionally resolves a second,
// less-instrumented ref (the candidate default branch, e.g. "main") shared across
// every worktree of the repo, which the git CLI's atomic-rename ref reads simply
// aren't exposed to. Given this file's brief was the working-tree-vs-commit diff
// content, not ref resolution, converting this call is left for a future pass that
// can add the same worktree-specific race coverage getHeadCommitSHA already has.
func (g *GitWorktree) resolveBaseCommitSHA() string {
	for _, branch := range CandidateDefaultBranches {
		output, err := g.runGitCommand(g.worktreePath, "merge-base", "HEAD", branch)
		if err == nil {
			if sha := strings.TrimSpace(output); sha != "" {
				return sha
			}
		}
	}
	return ""
}

// Diff returns the git diff between the worktree and the base branch along with
// statistics. The diff content itself (this function and buildWorkingTreePatch)
// is computed via go-git by comparing the base commit's tree directly against the
// worktree's current on-disk state — tracked-file modifications, deletions, and
// untracked-but-not-gitignored new files alike — rather than shelling out to
// `git add -N .` + `git --no-pager diff <baseSHA>` as before. That -N staging
// trick only existed to coax the `git diff` CLI into including untracked files;
// this implementation includes them directly by construction, with no need to
// mutate the git index. ops.go's DiffContentBetween (commit-to-commit) claimed no
// go-git equivalent existed for this working-tree case; this is that equivalent,
// built from go-git's lower-level tree/gitignore/line-diff primitives rather than
// a single library call. See resolveBaseCommitSHA's doc comment for the one piece
// of this feature that deliberately stays on a subprocess (ref resolution, not
// diff content — a real go-git race documented elsewhere in this package).
func (g *GitWorktree) Diff() *DiffStats {
	stats := &DiffStats{}

	// Check if the worktree path exists
	if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
		stats.Error = fmt.Errorf("worktree path does not exist: %s", g.worktreePath)
		return stats
	}

	// Check if the directory is actually a git repository
	gitDir := filepath.Join(g.worktreePath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// This isn't a git repository - this is common when sessions are created
		// in non-git directories. Return empty stats without error to avoid spam
		return stats
	}

	repo, err := OpenRepo(g.worktreePath)
	if err != nil {
		// A ".git" entry exists but doesn't open as a usable repository — treated
		// the same as "not a git repository" above rather than surfaced as an
		// error, matching the subprocess implementation's "not a git repository"
		// stderr-matching branches (every one of which returned empty stats, no
		// error).
		return stats
	}

	// Check if base commit SHA is set (required for diff operations)
	baseCommitSHA := g.GetBaseCommitSHA()
	if baseCommitSHA == "" {
		// Base commit not set (e.g., sessions created before this was tracked).
		// Try to resolve it from merge-base so existing worktrees still get diffs.
		if resolved := g.resolveBaseCommitSHA(); resolved != "" {
			g.baseCommitSHA = resolved // cache for future calls
			baseCommitSHA = resolved
		} else {
			return stats
		}
	}

	baseCommit, err := repo.CommitObject(plumbing.NewHash(baseCommitSHA))
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			// Stored SHA is no longer readable (e.g., after repo rename, rebase,
			// or gc) — go-git's equivalent of the subprocess's "unable to read"
			// stderr match. Clear it so the next call re-resolves via
			// resolveBaseCommitSHA().
			g.baseCommitSHA = ""
			return stats
		}
		stats.Error = fmt.Errorf("failed to resolve base commit %s: %w", baseCommitSHA, err)
		return stats
	}

	patch, err := g.buildWorkingTreePatch(baseCommit)
	if err != nil {
		stats.Error = err
		return stats
	}

	var buf bytes.Buffer
	if err := fdiff.NewUnifiedEncoder(&buf, fdiff.DefaultContextLines).Encode(patch); err != nil {
		stats.Error = fmt.Errorf("failed to encode diff: %w", err)
		return stats
	}

	for _, fp := range patch.FilePatches() {
		for _, chunk := range fp.Chunks() {
			lines := countChunkLines(chunk.Content())
			switch chunk.Type() {
			case fdiff.Add:
				stats.Added += lines
			case fdiff.Delete:
				stats.Removed += lines
			}
		}
	}

	// git diff output is raw bytes and can contain byte sequences that aren't valid
	// UTF-8 (e.g. a tracked file encoded as Latin-1). Content ultimately crosses into
	// a proto3 string field (DiffStats.content), which rejects invalid UTF-8 at
	// marshal time, so sanitize here at the source.
	stats.Content = strings.ToValidUTF8(buf.String(), "�")

	return stats
}

// buildWorkingTreePatch computes the diff between baseCommit's tree and the
// current on-disk state of g.worktreePath, as an fdiff.Patch ready for
// fdiff.NewUnifiedEncoder. Every path that exists in either the base tree or the
// (non-gitignored) worktree is considered; paths whose content is byte-identical
// on both sides are skipped rather than emitted as a no-op FilePatch.
func (g *GitWorktree) buildWorkingTreePatch(baseCommit *object.Commit) (fdiff.Patch, error) {
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to read base commit tree: %w", err)
	}

	baseFiles := make(map[string]*object.File)
	fileIter := baseTree.Files()
	defer fileIter.Close()
	if err := fileIter.ForEach(func(f *object.File) error {
		baseFiles[f.Name] = f
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to walk base commit tree: %w", err)
	}

	untrackedPaths, err := g.untrackedWorktreePaths(baseFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate untracked files: %w", err)
	}

	paths := make(map[string]struct{}, len(baseFiles)+len(untrackedPaths))
	for p := range baseFiles {
		paths[p] = struct{}{}
	}
	for _, p := range untrackedPaths {
		paths[p] = struct{}{}
	}
	sortedPaths := make([]string, 0, len(paths))
	for p := range paths {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	var filePatches []fdiff.FilePatch
	for _, path := range sortedPaths {
		fp, changed, err := diffOneFile(g.worktreePath, path, baseFiles[path])
		if err != nil {
			return nil, err
		}
		if changed {
			filePatches = append(filePatches, fp)
		}
	}

	return &workingTreePatch{filePatches: filePatches}, nil
}

// untrackedWorktreePaths walks g.worktreePath (skipping .git and anything
// matched by the worktree's .gitignore/.git/info/exclude patterns, mirroring
// what `git add .` would stage) and returns the slash-separated relative paths
// of every regular file or symlink not already present in baseFiles. Reads
// gitignore patterns directly (github.com/go-git/go-git/v5/plumbing/format/gitignore.ReadPatterns,
// the same call session/unfinished's GoGitVCSReader uses) rather than go-git's
// Worktree.Status(), which hashes every untracked file's full content just to
// classify it — unnecessary work here since only the path is needed.
func (g *GitWorktree) untrackedWorktreePaths(baseFiles map[string]*object.File) ([]string, error) {
	fsRoot := osfs.New(g.worktreePath)
	patterns, err := gitignore.ReadPatterns(fsRoot, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read gitignore patterns: %w", err)
	}
	matcher := gitignore.NewMatcher(patterns)

	var paths []string
	walkErr := filepath.WalkDir(g.worktreePath, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(g.worktreePath, fullPath)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		segments := strings.Split(relSlash, "/")

		if d.IsDir() {
			if segments[0] == ".git" {
				return filepath.SkipDir
			}
			if matcher.Match(segments, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if segments[0] == ".git" {
			return nil
		}
		if _, tracked := baseFiles[relSlash]; tracked {
			return nil
		}
		if matcher.Match(segments, false) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		// Only regular files and symlinks are diffable content; skip anything
		// else (sockets, devices) defensively.
		if info.Mode()&os.ModeSymlink == 0 && !info.Mode().IsRegular() {
			return nil
		}

		paths = append(paths, relSlash)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return paths, nil
}

// diffOneFile builds path's fdiff.FilePatch given its optional base-tree entry.
// changed is false (with a nil fp and error) when the file's content is
// identical on both sides, or absent from both — the latter should not occur
// given how callers derive path, but is handled the same way regardless.
// worktreePath is g.worktreePath, passed explicitly since this has no other
// dependency on *GitWorktree.
func diffOneFile(worktreePath, path string, baseFile *object.File) (fp fdiff.FilePatch, changed bool, err error) {
	var baseContent []byte
	baseExists := baseFile != nil
	if baseExists {
		r, readerErr := baseFile.Reader()
		if readerErr != nil {
			return nil, false, fmt.Errorf("failed to read base content of %s: %w", path, readerErr)
		}
		content, readErr := io.ReadAll(r)
		_ = r.Close()
		if readErr != nil {
			return nil, false, fmt.Errorf("failed to read base content of %s: %w", path, readErr)
		}
		baseContent = content
	}

	fullPath := filepath.Join(worktreePath, filepath.FromSlash(path))
	diskContent, diskMode, diskExists, err := readWorktreeFile(fullPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read worktree file %s: %w", path, err)
	}

	if !baseExists && !diskExists {
		return nil, false, nil
	}
	if baseExists && diskExists && bytes.Equal(baseContent, diskContent) && baseFile.Mode == diskMode {
		// Content AND mode both unchanged. Mode is checked too: a mode-only
		// change (e.g. `chmod +x` on an otherwise-untouched tracked file) is a
		// real diff a byte-content-only comparison would otherwise miss.
		return nil, false, nil
	}

	var from, to *diffFile
	if baseExists {
		from = &diffFile{path: path, hash: baseFile.Hash, mode: baseFile.Mode}
	}
	if diskExists {
		// The on-disk content was never hashed into a real git object, so there
		// is no genuine blob hash to report here — plumbing.ZeroHash is purely
		// cosmetic for the unified diff's "index a..b" header line, mirroring
		// what a real `git diff` shows for an intent-to-add (`git add -N`) file.
		to = &diffFile{path: path, hash: plumbing.ZeroHash, mode: diskMode}
	}

	if isBinaryContent(baseContent) || isBinaryContent(diskContent) {
		return &diffFilePatch{from: from, to: to, binary: true}, true, nil
	}

	diffs := gogitdiff.Do(string(baseContent), string(diskContent))
	chunks := make([]fdiff.Chunk, 0, len(diffs))
	for _, d := range diffs {
		var op fdiff.Operation
		switch d.Type {
		case dmp.DiffDelete:
			op = fdiff.Delete
		case dmp.DiffInsert:
			op = fdiff.Add
		default:
			op = fdiff.Equal
		}
		chunks = append(chunks, &diffChunk{content: d.Text, op: op})
	}

	return &diffFilePatch{from: from, to: to, chunks: chunks}, true, nil
}

// readWorktreeFile reads path's current on-disk content and mode. exists is
// false (with a nil error) when the path is simply absent — the normal
// "deleted" case, not a failure.
func readWorktreeFile(path string) (content []byte, mode filemode.FileMode, exists bool, err error) {
	info, statErr := os.Lstat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, 0, false, nil
		}
		return nil, 0, false, statErr
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return nil, 0, false, readErr
		}
		return []byte(target), filemode.Symlink, true, nil
	case info.IsDir():
		// A tree entry that's become a directory on disk (or vice versa) is a
		// type change this simple content-comparison doesn't model; treat it as
		// absent on the disk side rather than erroring.
		return nil, 0, false, nil
	case info.Mode()&0o111 != 0:
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, 0, false, readErr
		}
		return data, filemode.Executable, true, nil
	default:
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, 0, false, readErr
		}
		return data, filemode.Regular, true, nil
	}
}

// isBinaryContent reports whether content looks binary, using the same
// NUL-byte-sniffing heuristic git itself uses (github.com/go-git/go-git/v5/utils/binary,
// also what object.File.IsBinary() uses internally for commit-to-commit diffs
// in ops.go). A nil/empty content is never binary.
func isBinaryContent(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	isBin, err := binary.IsBinary(bytes.NewReader(content))
	if err != nil {
		return false
	}
	return isBin
}

// diffFile implements fdiff.File for one side of a diffOneFile comparison.
type diffFile struct {
	path string
	hash plumbing.Hash
	mode filemode.FileMode
}

func (f *diffFile) Hash() plumbing.Hash     { return f.hash }
func (f *diffFile) Mode() filemode.FileMode { return f.mode }
func (f *diffFile) Path() string            { return f.path }

// diffFilePatch implements fdiff.FilePatch for one file's change in Diff()'s
// working-tree-vs-base-commit patch.
type diffFilePatch struct {
	from, to *diffFile
	binary   bool
	chunks   []fdiff.Chunk
}

func (p *diffFilePatch) IsBinary() bool { return p.binary }

func (p *diffFilePatch) Files() (from, to fdiff.File) {
	if p.from != nil {
		from = p.from
	}
	if p.to != nil {
		to = p.to
	}
	return
}

func (p *diffFilePatch) Chunks() []fdiff.Chunk { return p.chunks }

// diffChunk implements fdiff.Chunk. Equivalent to go-git's own unexported
// object.textChunk (plumbing/object/patch.go), reimplemented here because that
// type isn't exported.
type diffChunk struct {
	content string
	op      fdiff.Operation
}

func (c *diffChunk) Content() string       { return c.content }
func (c *diffChunk) Type() fdiff.Operation { return c.op }

// workingTreePatch implements fdiff.Patch for Diff()'s working-tree-vs-base-
// commit output. Equivalent to go-git's own unexported object.Patch, reimplemented
// here because that type's constructor isn't exported for a non-commit-to-commit diff.
type workingTreePatch struct {
	filePatches []fdiff.FilePatch
}

func (p *workingTreePatch) FilePatches() []fdiff.FilePatch { return p.filePatches }
func (p *workingTreePatch) Message() string                { return "" }
