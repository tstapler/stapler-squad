package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// minFuzzBaseLines/maxFuzzBaseLines bound the synthetic base file's line count (derived
// from a single fuzzer byte) — enough range to exercise edits near either end of the file
// without letting a single iteration's git-merge-file subprocess round trip get slow.
const (
	minFuzzBaseLines    = 3
	maxFuzzBaseLineSpan = 8 // base has minFuzzBaseLines..minFuzzBaseLines+maxFuzzBaseLineSpan-1 lines
)

// maxFuzzEditContentBytes caps how much of an edit op's raw payload becomes a single
// replacement/inserted line's text.
const maxFuzzEditContentBytes = 64

// FuzzNativeMerge fuzzes ThreeWayFileMerger.Merge — the pure line-diff3 core
// nativeThreeWayMerge's pipeline (native_merge.go) delegates every path's content
// resolution to — against real `git merge-file`'s own three-way text merge as the oracle
// (Story 5.2.1). Epic 5.1's DifferentialMergeHarness (native_merge_differential_test.go)
// was not present in the tree when this was written (checked via `ls`/`git log` per
// plan.md's fallback instruction), so this fuzzer builds its own minimal subprocess-git
// oracle instead of reimplementing a full repo/commit differential harness —
// `git merge-file` is the right-sized real-git primitive for a single file's three-way
// text merge, avoiding the cost of a full clone+commit+merge cycle per fuzz iteration.
//
// ours/theirs are derived from base via applyFuzzEdit rather than fuzzed as three fully
// independent byte strings — see applyFuzzEdit's doc comment for why: independently random
// line content routinely produced base files with coincidentally repeated/reordered line
// values, which gives line-diff algorithms multiple equally-minimal alignments to choose
// between, and this project's diffmatchpatch-based diff and real git's own xdiff do not
// always pick the same one (verified directly during this fuzzer's own development — see
// the two non-adjacency corpus entries this design change made unreachable). That
// divergence is a property of which diff library ran on ambiguous input, not a bug in this
// package's hunk reconciliation, and reproducing git's exact diff heuristics is the
// "Rabbit Holes" scope build-vs-buy.md §3 explicitly declined to take on. Deriving each
// side from base with exactly one tagged, guaranteed-unique-content edit keeps every
// base-vs-side diff structurally unambiguous (every base line value is globally unique, and
// each side's sole new content is prefixed distinctly from the other side's), so this
// fuzzer stays targeted at genuine reconciliation bugs in how *two sides'* edits interact —
// exactly the adjacency gap it did find (see the seed corpus and testdata/fuzz entries).
func FuzzNativeMerge(f *testing.F) {
	// Same-line: both sides edit the identical line differently — a real conflict.
	f.Add(byte(1), []byte{0, 1}, []byte{0, 1})
	// Adjacent-but-separated edits: sides edit lines with one unchanged line between them —
	// auto-resolves.
	f.Add(byte(2), []byte{0, 1}, []byte{0, 3})
	// Touching edits: sides edit immediately adjacent lines with no unchanged line between
	// them — a real conflict (this exact shape is FuzzNativeMerge's own found regression;
	// see testdata/fuzz/FuzzNativeMerge/adjacent_line_edits_conflict).
	f.Add(byte(1), []byte{0, 1}, []byte{0, 2})
	// Add/add: both sides insert new, differently-tagged content at the same position — a
	// same-region insertion conflict.
	f.Add(byte(1), []byte{1, 2}, []byte{1, 2})
	// Delete/modify: one side deletes a line the other side modified at the same
	// position — a real conflict.
	f.Add(byte(1), []byte{2, 1}, []byte{0, 1})

	f.Fuzz(func(t *testing.T, baseSeed byte, oursOp, theirsOp []byte) {
		base := fuzzBaseLines(baseSeed)
		ours := applyFuzzEdit(base, "OURS", oursOp)
		theirs := applyFuzzEdit(base, "THEIRS", theirsOp)

		baseContent := joinFuzzLines(base)
		oursContent := joinFuzzLines(ours)
		theirsContent := joinFuzzLines(theirs)

		var merger ThreeWayFileMerger
		nativeResult, nativeErr := merger.Merge(baseContent, oursContent, theirsContent)
		require.NoError(t, nativeErr, "synthetic fuzz content is always valid UTF-8, Merge must not error on it")
		require.NotNil(t, nativeResult)

		oracleContent, oracleConflicted, oracleErr := realGitMergeFileOracle(t, baseContent, oursContent, theirsContent)
		require.NoError(t, oracleErr, "real git merge-file oracle invocation itself must not fail")

		require.Equalf(t, oracleConflicted, nativeResult.Conflicted,
			"conflict outcome disagrees with real git merge-file oracle\nbase=%q\nours=%q\ntheirs=%q\nnative content=%q\noracle output=%q",
			baseContent, oursContent, theirsContent, nativeResult.Content, oracleContent)
	})
}

// fuzzBaseLines builds a synthetic base file from a single fuzzer byte: every line is a
// distinct "L<i>" token, so no line value in base ever repeats — the property applyFuzzEdit
// and FuzzNativeMerge's doc comment rely on to keep every base-vs-side diff unambiguous.
func fuzzBaseLines(seed byte) []string {
	n := minFuzzBaseLines + int(seed)%maxFuzzBaseLineSpan
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i)
	}
	return lines
}

// applyFuzzEdit derives one side's line content from base by applying at most one edit —
// replace, insert-before, or delete, at a fuzzer-chosen position — with any newly
// introduced content prefixed with tag ("OURS"/"THEIRS") so it can never collide with a
// base line (all "L<i>", never containing ':') or with the other side's own inserted
// content (a different tag). op's first two bytes select the edit kind and position; any
// remaining bytes become the replacement/inserted line's text (sanitized to a single,
// valid-UTF8, newline-free line). An empty/too-short op is treated as "no edit" (ours or
// theirs identical to base), which is itself a useful degenerate case for the fuzzer to
// explore.
func applyFuzzEdit(base []string, tag string, op []byte) []string {
	if len(op) < 2 {
		return append([]string(nil), base...)
	}
	kind := op[0] % 3 // 0 = replace, 1 = insert-before, 2 = delete
	pos := 0
	if len(base) > 0 {
		pos = int(op[1]) % (len(base) + 1)
	}
	content := fuzzEditLine(tag, op[2:])

	out := append([]string(nil), base[:pos]...)
	switch {
	case kind == 0 && pos < len(base): // replace the line at pos
		out = append(out, content)
		out = append(out, base[pos+1:]...)
	case kind == 1: // insert before pos
		out = append(out, content)
		out = append(out, base[pos:]...)
	case kind == 2 && pos < len(base): // delete the line at pos
		out = append(out, base[pos+1:]...)
	default: // replace/delete requested past the end of base: no-op past pos
		out = append(out, base[pos:]...)
	}
	return out
}

// fuzzEditLine turns an edit op's raw payload into a single, valid-UTF8, newline-free line
// of text, prefixed with tag+":" — a prefix no "L<i>" base token or the other side's
// differently-tagged content can ever equal (see applyFuzzEdit's doc comment).
func fuzzEditLine(tag string, raw []byte) string {
	if len(raw) > maxFuzzEditContentBytes {
		raw = raw[:maxFuzzEditContentBytes]
	}
	text := strings.ToValidUTF8(string(raw), "�")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == 0 {
			return -1
		}
		return r
	}, text)
	return tag + ":" + text
}

// joinFuzzLines assembles lines into newline-terminated file content, matching this
// package's own splitLines/hasTrailingNewline convention (every real file this fuzzer
// builds ends with a trailing newline).
func joinFuzzLines(lines []string) string {
	if len(lines) == 0 {
		return "\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

// realGitMergeFileOracle shells to real `git merge-file -p` (Story 5.2.1's fallback
// differential oracle) to three-way merge base/ours/theirs exactly as a real git
// installation would, returning its merged/conflict-marked stdout and whether it reported
// any conflict. `-c merge.conflictStyle=merge` pins the comparison to real git's own
// documented default marker style (see native_merge_conflict.go's ConflictMarkerStyle
// doc comment) regardless of this checkout's local .git/config override, and running in a
// bare scratch directory (not a git repo) means no repo-level config can leak in either.
func realGitMergeFileOracle(t *testing.T, base, ours, theirs string) (content string, conflicted bool, err error) {
	t.Helper()
	dir := t.TempDir()

	baseFile := filepath.Join(dir, "base")
	oursFile := filepath.Join(dir, "ours")
	theirsFile := filepath.Join(dir, "theirs")
	if err := os.WriteFile(baseFile, []byte(base), 0o600); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(oursFile, []byte(ours), 0o600); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(theirsFile, []byte(theirs), 0o600); err != nil {
		return "", false, err
	}

	cmd := safeexec.CommandContext(context.Background(), "git",
		"-c", "merge.conflictStyle=merge",
		"merge-file", "-p",
		"-L", "HEAD", "-L", "base", "-L", "theirs",
		oursFile, baseFile, theirsFile)
	cmd.Dir = dir
	out, runErr := cmd.Output()
	if runErr == nil {
		return string(out), false, nil
	}

	// git merge-file exits with the number of conflicts (>0) on a merge that resolved
	// with conflicts; its stdout (captured via -p) is still the merged, marker-bearing
	// content in that case. Any other failure (git missing, a real execution error) is a
	// genuine oracle error, not a conflict outcome.
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() > 0 {
		return string(out), true, nil
	}
	return "", false, runErr
}
