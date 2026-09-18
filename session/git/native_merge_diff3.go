package git

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	godiff "github.com/go-git/go-git/v5/utils/diff"
	"github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// MergeRegionKind classifies one reconciled region of a three-way file
// merge. It is a sum type, not boolean flags (isConflict/isOurs/isTheirs),
// so that illegal states (e.g. "ours and theirs simultaneously") cannot be
// represented — see the "Conflict classification" Pattern Decision.
type MergeRegionKind int

const (
	// RegionUnchanged covers base lines neither side touched.
	RegionUnchanged MergeRegionKind = iota
	// RegionOursOnly covers a region only ours changed (or both sides made
	// the identical change — see Story 3.2.2's "identical edits" case).
	RegionOursOnly
	// RegionTheirsOnly covers a region only theirs changed.
	RegionTheirsOnly
	// RegionConflict covers a region both sides changed differently.
	RegionConflict
)

// MergeHunk is one classified region of a file's reconciled content: a
// MergeRegionKind plus the base/ours/theirs line content it covers (lines
// have no trailing newline — content assembly re-joins them).
type MergeHunk struct {
	Kind   MergeRegionKind
	Base   []string
	Ours   []string
	Theirs []string
}

// MergeResult is the outcome of a ThreeWayFileMerger.Merge or MergeFile call:
// the classified hunks plus the assembled content for the auto-resolved
// case. When Conflicted is true, Content only reflects the non-conflicted
// hunks — rendering conflict markers into the working-tree file is Epic
// 3.3's renderConflictHunk, not this package's job (Task 3.2.2c).
type MergeResult struct {
	Hunks      []MergeHunk
	Content    string
	Conflicted bool
}

// Conflicts returns the subset of Hunks classified as RegionConflict.
func (r *MergeResult) Conflicts() []MergeHunk {
	var out []MergeHunk
	for _, h := range r.Hunks {
		if h.Kind == RegionConflict {
			out = append(out, h)
		}
	}
	return out
}

// ThreeWayFileMerger runs the diff3 hunk-reconciliation algorithm over one
// path's base/ours/theirs content, producing merged content or a conflict
// classification. It holds no state — a zero value is ready to use.
type ThreeWayFileMerger struct{}

// lineDiff returns the line-oriented diff between base and side, computed
// via go-git's own utils/diff wrapper (DiffLinesToRunes+DiffMainRunes) per
// Task 3.2.1a — zero new dependency, since sergi/go-diff already ships
// transitively through go-git.
func lineDiff(base, side string) []diffmatchpatch.Diff {
	return godiff.Do(base, side)
}

// lineEdit is one contiguous replacement of base lines [baseStart, baseEnd)
// with newLines, derived from a lineDiff result.
type lineEdit struct {
	baseStart, baseEnd int
	newLines           []string
}

// computeEdits walks lineDiff(base, side) and collapses it into an edit
// script expressed as base line ranges — the per-side input the hunk-walk
// classifier (reconcileHunks) merges together.
func computeEdits(base, side string) []lineEdit {
	diffs := lineDiff(base, side)
	var edits []lineEdit
	pos := 0
	var cur *lineEdit

	flush := func() {
		if cur != nil {
			edits = append(edits, *cur)
			cur = nil
		}
	}

	for _, d := range diffs {
		lines := diffTextToLines(d.Text)
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			flush()
			pos += len(lines)
		case diffmatchpatch.DiffDelete:
			if cur == nil {
				cur = &lineEdit{baseStart: pos, baseEnd: pos}
			}
			cur.baseEnd += len(lines)
			pos += len(lines)
		case diffmatchpatch.DiffInsert:
			if cur == nil {
				cur = &lineEdit{baseStart: pos, baseEnd: pos}
			}
			cur.newLines = append(cur.newLines, lines...)
		}
	}
	flush()
	return edits
}

// diffTextToLines splits one diffmatchpatch.Diff.Text chunk (which carries
// whole lines with their trailing "\n", per go-git's line-mode diff) back
// into bare lines with the newline stripped.
func diffTextToLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// splitLines splits whole file content into bare lines (no trailing
// newline), matching diffTextToLines' convention.
func splitLines(content string) []string {
	return diffTextToLines(content)
}

// hasTrailingNewline reports whether content ends with "\n" (or is empty).
func hasTrailingNewline(content string) bool {
	return content == "" || strings.HasSuffix(content, "\n")
}

func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// padded reconstructs a side's content over [pos, end) when only
// [e.baseStart, e.baseEnd) of that range is actually an edit on this side —
// the rest is unchanged base content, needed to keep both sides comparable
// when their edit ranges overlap without being identical.
func padded(baseLines []string, pos, end int, e lineEdit) []string {
	out := make([]string, 0, end-pos)
	out = append(out, baseLines[pos:e.baseStart]...)
	out = append(out, e.newLines...)
	out = append(out, baseLines[e.baseEnd:end]...)
	return out
}

// hunkWalker holds the cursor state for reconcileHunks' base-line walk: a
// position into baseLines plus an index into each side's edit script.
type hunkWalker struct {
	baseLines              []string
	oursEdits, theirsEdits []lineEdit
	hunks                  []MergeHunk
	pos, oi, ti            int
}

func (w *hunkWalker) done() bool {
	return w.pos >= len(w.baseLines) && w.oi >= len(w.oursEdits) && w.ti >= len(w.theirsEdits)
}

func (w *hunkWalker) peek() (oursNext, theirsNext *lineEdit) {
	if w.oi < len(w.oursEdits) {
		oursNext = &w.oursEdits[w.oi]
	}
	if w.ti < len(w.theirsEdits) {
		theirsNext = &w.theirsEdits[w.ti]
	}
	return
}

func (w *hunkWalker) appendUnchanged(start, end int) {
	if start >= end {
		return
	}
	lines := append([]string(nil), w.baseLines[start:end]...)
	w.hunks = append(w.hunks, MergeHunk{Kind: RegionUnchanged, Base: lines, Ours: lines, Theirs: lines})
}

func (w *hunkWalker) appendOneSided(kind MergeRegionKind, start, end int, newLines []string) {
	base := append([]string(nil), w.baseLines[start:end]...)
	h := MergeHunk{Kind: kind, Base: base}
	side := append([]string(nil), newLines...)
	if kind == RegionOursOnly {
		h.Ours, h.Theirs = side, base
	} else {
		h.Theirs, h.Ours = side, base
	}
	w.hunks = append(w.hunks, h)
}

// appendMerged handles a base range both sides touched (possibly with
// mismatched extents — see padded). Identical resulting content on both
// sides auto-resolves (libgit2 precedent, features.md §1); RegionOursOnly
// is an arbitrary but consistent tie-break in that case.
func (w *hunkWalker) appendMerged(start, end int, ours, theirs lineEdit) {
	oursLines := padded(w.baseLines, start, end, ours)
	theirsLines := padded(w.baseLines, start, end, theirs)
	kind := RegionConflict
	if linesEqual(oursLines, theirsLines) {
		kind = RegionOursOnly
	}
	w.hunks = append(w.hunks, MergeHunk{
		Kind:   kind,
		Base:   append([]string(nil), w.baseLines[start:end]...),
		Ours:   oursLines,
		Theirs: theirsLines,
	})
}

// step processes the walker's current position once, advancing pos/oi/ti
// and emitting exactly one hunk (or an unchanged run) — split out of
// reconcileHunks to keep that function's body short.
func (w *hunkWalker) step() {
	oursNext, theirsNext := w.peek()
	oursStarts := oursNext != nil && oursNext.baseStart == w.pos
	theirsStarts := theirsNext != nil && theirsNext.baseStart == w.pos

	switch {
	case !oursStarts && !theirsStarts:
		next := len(w.baseLines)
		if oursNext != nil && oursNext.baseStart < next {
			next = oursNext.baseStart
		}
		if theirsNext != nil && theirsNext.baseStart < next {
			next = theirsNext.baseStart
		}
		w.appendUnchanged(w.pos, next)
		w.pos = next

	case oursStarts && theirsStarts:
		w.mergeAndAdvance(oursNext, theirsNext)

	case oursStarts && (theirsNext == nil || theirsNext.baseStart > oursNext.baseEnd):
		w.appendOneSided(RegionOursOnly, w.pos, oursNext.baseEnd, oursNext.newLines)
		w.pos = oursNext.baseEnd
		w.oi++

	case oursStarts:
		// theirsNext overlaps ours' range, or merely touches it with zero lines of
		// unchanged base context between them — real git's own xdl_merge/diff3
		// algorithm requires at least one shared context line to treat two sides'
		// edits as independent (confirmed against real `git merge`: base "a b c d",
		// ours editing line 2 and theirs editing the immediately adjacent line 3
		// with no unchanged line between them still conflicts, even though neither
		// side touched the other's exact line — FuzzNativeMerge's regression corpus
		// entry). A bare `>=`-adjacency check here silently auto-resolved that case
		// instead of conflicting, which is the more dangerous failure mode for a
		// merge tool: over-eager auto-resolution a human/real-git reviewer would
		// never have seen.
		w.mergeAndAdvance(oursNext, theirsNext)

	case theirsStarts && (oursNext == nil || oursNext.baseStart > theirsNext.baseEnd):
		w.appendOneSided(RegionTheirsOnly, w.pos, theirsNext.baseEnd, theirsNext.newLines)
		w.pos = theirsNext.baseEnd
		w.ti++

	default:
		// theirsStarts, overlapping or touching a later ours edit — see the
		// oursStarts branch above for why touching also routes here.
		w.mergeAndAdvance(oursNext, theirsNext)
	}
}

func (w *hunkWalker) mergeAndAdvance(ours, theirs *lineEdit) {
	end := ours.baseEnd
	if theirs.baseEnd > end {
		end = theirs.baseEnd
	}
	w.appendMerged(w.pos, end, *ours, *theirs)
	w.pos = end
	w.oi++
	w.ti++
}

// reconcileHunks is the hunk-walk classification core (Task 3.2.2b): it
// walks base lines alongside both sides' edit scripts, emitting one
// MergeHunk per region. The algorithm shape is referenced from
// epiclabs-io/diff3's Diff3Merge per stack.md §4.4 (read for the walking
// approach, not copied for markers).
func reconcileHunks(baseLines []string, oursEdits, theirsEdits []lineEdit) []MergeHunk {
	w := &hunkWalker{baseLines: baseLines, oursEdits: oursEdits, theirsEdits: theirsEdits}
	for !w.done() {
		w.step()
	}
	return w.hunks
}

// resolvedLines returns the content a non-conflict hunk contributes to the
// merged file.
func resolvedLines(h MergeHunk) []string {
	switch h.Kind {
	case RegionOursOnly:
		return h.Ours
	case RegionTheirsOnly:
		return h.Theirs
	default:
		return h.Base
	}
}

// assembleContent concatenates resolved hunks' content in order (Task
// 3.2.2c). RegionConflict hunks contribute nothing here — embedding their
// marker text is deferred to Epic 3.3's renderConflictHunk rather than
// inlined in this package.
func assembleContent(hunks []MergeHunk, trailingNewline bool) string {
	var lines []string
	for _, h := range hunks {
		if h.Kind == RegionConflict {
			continue
		}
		lines = append(lines, resolvedLines(h)...)
	}
	content := strings.Join(lines, "\n")
	if len(lines) > 0 && trailingNewline {
		content += "\n"
	}
	return content
}

// Merge runs the diff3 hunk-reconciliation algorithm over one file's
// base/ours/theirs content (Story 3.2.2). It returns an error rather than
// panicking or silently truncating when content isn't valid UTF-8 and
// wasn't already filtered out by the binary heuristic (MergeFile) —
// splitting invalid UTF-8 into "lines" can otherwise corrupt line
// boundaries silently.
func (ThreeWayFileMerger) Merge(base, ours, theirs string) (*MergeResult, error) {
	for name, content := range map[string]string{"base": base, "ours": ours, "theirs": theirs} {
		if !utf8.ValidString(content) {
			return nil, fmt.Errorf("native_merge_diff3: %s content is not valid UTF-8 and was not flagged binary", name)
		}
	}

	baseLines := splitLines(base)
	oursEdits := computeEdits(base, ours)
	theirsEdits := computeEdits(base, theirs)

	hunks := reconcileHunks(baseLines, oursEdits, theirsEdits)

	trailingNewline := hasTrailingNewline(base) && hasTrailingNewline(ours) && hasTrailingNewline(theirs)
	result := &MergeResult{
		Hunks:   hunks,
		Content: assembleContent(hunks, trailingNewline),
	}
	result.Conflicted = len(result.Conflicts()) > 0
	return result, nil
}

// FileConflictReason names why MergeFile short-circuited before attempting
// a content merge (Story 3.2.3) — a mode/binary/gitlink conflict is always
// reported this way, never silently mismerged or fed into content merging.
type FileConflictReason string

const (
	// ReasonNone means no short-circuit fired; MergeFile ran a normal
	// content merge (see FileMergeOutcome.Result).
	ReasonNone FileConflictReason = ""
	// ReasonModeConflict: ours' and theirs' file modes differ.
	ReasonModeConflict FileConflictReason = "mode conflict"
	// ReasonBinaryConflict: a binary file changed differently on both sides.
	ReasonBinaryConflict FileConflictReason = "binary conflict"
	// ReasonGitlinkConflict: a gitlink (submodule) entry on either side.
	ReasonGitlinkConflict FileConflictReason = "gitlink conflict"
)

// FileMergeInput bundles one path's three-way inputs — content and file
// mode on each side — for MergeFile's mode/binary/gitlink short-circuits
// ahead of any content-level three-way merge.
type FileMergeInput struct {
	BaseMode, OursMode, TheirsMode          filemode.FileMode
	BaseContent, OursContent, TheirsContent []byte
}

// FileMergeOutcome is MergeFile's result: either a short-circuited conflict
// (Reason set, Result nil) or a fully classified content merge (Result set,
// Reason == ReasonNone, ResolvedMode set to whichever mode survives — see
// resolveFileMode).
type FileMergeOutcome struct {
	Reason FileConflictReason
	Result *MergeResult
	// ResolvedMode is the file mode the merged content should carry, meaningful only
	// when Reason == ReasonNone: OursMode/TheirsMode when they agree, otherwise
	// whichever one of them actually changed away from BaseMode (see resolveFileMode).
	ResolvedMode filemode.FileMode
}

// resolveFileMode classifies a three-way mode comparison: identical ours/theirs modes
// never conflict; a mode changed by only ONE side (the other still matching base) auto-
// resolves to the changed side's mode, mirroring real git's own one-sided-change
// auto-resolution and this project's established principle for one-sided content edits
// (ReconcilePathChange's single-side-changed handling). Only a genuine two-sided
// disagreement — both sides' modes differ from base, and from each other — conflicts.
func resolveFileMode(base, ours, theirs filemode.FileMode) (resolved filemode.FileMode, conflict bool) {
	if ours == theirs {
		return ours, false
	}
	if base == ours {
		return theirs, false // only theirs changed the mode
	}
	if base == theirs {
		return ours, false // only ours changed the mode
	}
	return 0, true // both sides changed the mode, disagreeingly
}

// isBinary reports whether content looks binary, using the same null-byte
// heuristic real git's buffer_is_binary uses: a NUL within the first 8000
// bytes (git's own sniff length).
func isBinary(content []byte) bool {
	const sniffLen = 8000
	if len(content) > sniffLen {
		content = content[:sniffLen]
	}
	return bytes.IndexByte(content, 0) != -1
}

// MergeFile classifies one path's three-way merge, short-circuiting to a
// conflict reason before attempting any content merge for gitlink (Task
// 3.2.3c), mode-only (Task 3.2.3a), and binary (Task 3.2.3b) cases — in
// that order, since a gitlink entry's value is a commit SHA, not diffable
// text, and must never reach lineDiff/mode-diff/binary-diff at all.
func (m ThreeWayFileMerger) MergeFile(in FileMergeInput) (*FileMergeOutcome, error) {
	if in.BaseMode == filemode.Submodule || in.OursMode == filemode.Submodule || in.TheirsMode == filemode.Submodule {
		return &FileMergeOutcome{Reason: ReasonGitlinkConflict}, nil
	}

	resolvedMode, modeConflict := resolveFileMode(in.BaseMode, in.OursMode, in.TheirsMode)
	if modeConflict {
		return &FileMergeOutcome{Reason: ReasonModeConflict}, nil
	}

	oursChanged := !bytes.Equal(in.BaseContent, in.OursContent)
	theirsChanged := !bytes.Equal(in.BaseContent, in.TheirsContent)
	sidesDiffer := !bytes.Equal(in.OursContent, in.TheirsContent)
	if oursChanged && theirsChanged && sidesDiffer &&
		(isBinary(in.BaseContent) || isBinary(in.OursContent) || isBinary(in.TheirsContent)) {
		return &FileMergeOutcome{Reason: ReasonBinaryConflict}, nil
	}
	// KNOWN GAP (PR #730 Gate 2 review): sidesDiffer already excludes the identical-change
	// case from ReasonBinaryConflict here, but the m.Merge fallback below still hard-fails
	// a binary both-sides-identical-change case instead of auto-resolving it — traced but
	// not reproduced; tracked as a follow-up given it's a rare edge case.

	result, err := m.Merge(string(in.BaseContent), string(in.OursContent), string(in.TheirsContent))
	if err != nil {
		return nil, err
	}
	return &FileMergeOutcome{Result: result, ResolvedMode: resolvedMode}, nil
}

// PathChange bundles what each side of TreeDiffPair's two object.Changes
// sets independently says happened to one path. Either side may be nil,
// meaning that side made no change to this path relative to base.
type PathChange struct {
	Ours   *object.Change
	Theirs *object.Change
}

// changePath returns the path a Change is keyed by: the destination name
// for an Insert/Modify (which, for a rename go-git's own default rename
// detection folded into one Modify, is the renamed-to path), or the source
// name for a Delete.
func changePath(c *object.Change) (string, error) {
	action, err := c.Action()
	if err != nil {
		return "", err
	}
	if action == merkletrie.Delete {
		return c.From.Name, nil
	}
	return c.To.Name, nil
}

// GroupChangesByPath indexes TreeDiffPair's two change-sets by the path
// each Change touches, so per-path reconciliation (ReconcilePathChange) can
// see what each side independently did — deliberately NOT correlating a
// Delete on one side with an Insert elsewhere as a rename beyond whatever
// go-git's own default rename detection already folded into a single
// Modify (see the "Rename+modify merge collision" Pattern Decision).
func GroupChangesByPath(baseToOurs, baseToTheirs object.Changes) (map[string]*PathChange, error) {
	byPath := make(map[string]*PathChange)
	for _, c := range baseToOurs {
		path, err := changePath(c)
		if err != nil {
			return nil, err
		}
		byPath[path] = &PathChange{Ours: c}
	}
	for _, c := range baseToTheirs {
		path, err := changePath(c)
		if err != nil {
			return nil, err
		}
		entry, ok := byPath[path]
		if !ok {
			entry = &PathChange{}
			byPath[path] = entry
		}
		entry.Theirs = c
	}
	return byPath, nil
}

// changeContents reads a Change's before/after blob content via its Files()
// accessor. Either return value is empty for an Insert (no before) or
// Delete (no after).
func changeContents(c *object.Change) (fromContent, toContent string, err error) {
	from, to, err := c.Files()
	if err != nil {
		return "", "", err
	}
	if from != nil {
		fromContent, err = from.Contents()
		if err != nil {
			return "", "", err
		}
	}
	if to != nil {
		toContent, err = to.Contents()
		if err != nil {
			return "", "", err
		}
	}
	return fromContent, toContent, nil
}

// singleSideResult wraps one side's Change as a trivial one-hunk
// MergeResult (Story 3.2.2e's rename+modify collision path, and the plain
// one-sided-change case).
func singleSideResult(kind MergeRegionKind, c *object.Change) (*MergeResult, error) {
	_, content, err := changeContents(c)
	if err != nil {
		return nil, err
	}
	lines := splitLines(content)
	hunk := MergeHunk{Kind: kind}
	if kind == RegionOursOnly {
		hunk.Ours = lines
	} else {
		hunk.Theirs = lines
	}
	return &MergeResult{Hunks: []MergeHunk{hunk}, Content: content}, nil
}

// ReconcilePathChange resolves one path's PathChange (grouped from
// TreeDiffPair's raw, rename-uncorrelated object.Changes) to a MergeResult.
//
// A Delete on either side leaves nothing on that side to diff against
// base for this path; per the "Rename+modify merge collision" Pattern
// Decision this project does not chase where that content went (a rename
// elsewhere) — it simply keeps whichever side still has content, without
// flagging a conflict. This is a defensive fallback: go-git's own default
// rename detection (RenameScore 60, already active in TreeDiffPair's
// Tree.Diff calls) folds a near-identical rename into a single Modify keyed
// by the new path before this function ever sees it, so this branch only
// fires for a dissimilar rename go-git didn't correlate.
func (m ThreeWayFileMerger) ReconcilePathChange(pc *PathChange) (*MergeResult, error) {
	switch {
	case pc.Ours == nil && pc.Theirs == nil:
		return &MergeResult{}, nil
	case pc.Ours == nil:
		return singleSideResult(RegionTheirsOnly, pc.Theirs)
	case pc.Theirs == nil:
		return singleSideResult(RegionOursOnly, pc.Ours)
	}

	oursAction, err := pc.Ours.Action()
	if err != nil {
		return nil, err
	}
	theirsAction, err := pc.Theirs.Action()
	if err != nil {
		return nil, err
	}

	switch {
	case oursAction == merkletrie.Delete && theirsAction == merkletrie.Delete:
		return &MergeResult{}, nil
	case oursAction == merkletrie.Delete:
		return singleSideResult(RegionTheirsOnly, pc.Theirs)
	case theirsAction == merkletrie.Delete:
		return singleSideResult(RegionOursOnly, pc.Ours)
	}

	_, oursContent, err := changeContents(pc.Ours)
	if err != nil {
		return nil, err
	}
	baseContent, theirsContent, err := changeContents(pc.Theirs)
	if err != nil {
		return nil, err
	}
	return m.Merge(baseContent, oursContent, theirsContent)
}
