package diagnose

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitdiff "github.com/go-git/go-git/v5/utils/diff"
	dmp "github.com/sergi/go-diff/diffmatchpatch"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// DiagnosticBundleAssembler builds the non-transcript sections of a
// DiagnosticBundle for one backlog item + its linked session(s)
// (Transaction-Script-style service, per the plan's Pattern Decisions).
// Every assemble* method here operates on local DTOs rather than package
// session's own types -- package diagnose cannot import package session
// directly (session -> session/tmux -> session/diagnose already exists, so
// the reverse import would close a cycle; see bundle_transcript.go's
// HandoffSummaryRow doc comment for the same constraint applied to Epic 2.2).
// Whatever wires a real backlog item/session/instance into this assembler
// adapts it into these DTOs at the call site, in a package that can import
// both.
type DiagnosticBundleAssembler struct {
	cfg DiagnosticBundleConfig
}

// NewDiagnosticBundleAssembler constructs a DiagnosticBundleAssembler that
// enforces cfg's per-section byte budgets.
func NewDiagnosticBundleAssembler(cfg DiagnosticBundleConfig) *DiagnosticBundleAssembler {
	return &DiagnosticBundleAssembler{cfg: cfg}
}

// BundleItem mirrors the session.BacklogItemData fields
// AssembleDescriptionAndAC reads (see DiagnosticBundleAssembler's doc comment
// for why this package defines its own mirror instead of importing
// session.BacklogItemData directly).
type BundleItem struct {
	ID                 string
	Description        string
	AcceptanceCriteria domain.AcCriteriaJSON
}

// AssembleDescriptionAndAC returns the budget-enforced Description and
// AcceptanceCriteria bundle section contents for item.
func (a *DiagnosticBundleAssembler) AssembleDescriptionAndAC(item BundleItem) (description, acceptanceCriteria string) {
	description = enforceBudget(BundleSectionDescription, item.Description, a.cfg)
	acceptanceCriteria = enforceBudget(BundleSectionAcceptanceCriteria, renderAcceptanceCriteria(item.AcceptanceCriteria), a.cfg)
	return description, acceptanceCriteria
}

// renderAcceptanceCriteria formats raw as a numbered checklist, mirroring
// session.buildAcChecklist's marker convention (that function is unexported
// in package session and, per this file's import-cycle constraint, couldn't
// be reused here even if it were exported).
func renderAcceptanceCriteria(raw domain.AcCriteriaJSON) string {
	criteria, err := domain.ParseAcCriteria(raw)
	if err != nil || len(criteria) == 0 {
		return "(no acceptance criteria)"
	}

	var sb strings.Builder
	for _, c := range criteria {
		marker := "[ ]"
		switch c.Status {
		case domain.AcStatusDone:
			marker = "[x]"
		case domain.AcStatusInProgress:
			marker = "[~]"
		case domain.AcStatusFail:
			marker = "[!]"
		}
		fmt.Fprintf(&sb, "%d. %s %s\n", c.Index, marker, c.Text)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// SessionHistoryEntry is one prior session's pre-rendered summary block for
// the History bundle section -- mirrors the handful of session.ItemSessionSummary
// fields BuildTokenBudgetedPrompt already renders per session, but as a single
// Summary string (rather than the individual fields) so assembleHistory's
// budget/drop logic doesn't need to know session.ItemSessionSummary's shape.
type SessionHistoryEntry struct {
	SessionUUID string
	Summary     string
}

// historyDroppedNoteFormat is the one-line note assembleHistory prepends when
// it drops one or more oldest entries to fit History's section budget (Story
// 2.1.1 AC).
const historyDroppedNoteFormat = "[%d older session(s) dropped for budget]"

// assembleHistory renders history (expected oldest-first, matching
// ItemSessionSummary's natural query order) into the History bundle section,
// dropping entries from the oldest end -- one at a time -- until the joined
// content fits History's SectionBudget.MaxBytes, per Story 2.1.1's AC. A
// one-line note is prepended whenever anything was dropped.
func (a *DiagnosticBundleAssembler) assembleHistory(history []SessionHistoryEntry) string {
	budget := sectionMaxBytes(BundleSectionHistory, a.cfg)

	remaining := history
	dropped := 0
	content := joinHistoryEntries(remaining, dropped)
	for budget > 0 && len(content) > budget && len(remaining) > 0 {
		remaining = remaining[1:]
		dropped++
		content = joinHistoryEntries(remaining, dropped)
	}

	return enforceBudget(BundleSectionHistory, content, a.cfg)
}

func joinHistoryEntries(entries []SessionHistoryEntry, dropped int) string {
	var sb strings.Builder
	if dropped > 0 {
		fmt.Fprintf(&sb, historyDroppedNoteFormat+"\n", dropped)
	}
	for i, e := range entries {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(e.Summary)
	}
	return sb.String()
}

// ReviewVerdictSummary mirrors the session.ReviewVerdictSummary fields
// assemblePriorVerdicts reads (see DiagnosticBundleAssembler's doc comment
// for the import-cycle reason this isn't session.ReviewVerdictSummary
// directly).
type ReviewVerdictSummary struct {
	OverallOutcome string
	Summary        string
	CreatedAt      time.Time
}

// ReviewVerdictSource is the minimal session.Storage/session.EntRepository
// surface assemblePriorVerdicts needs: GetRecentReviewVerdictSummaries
// itself, adapted to return this package's ReviewVerdictSummary mirror.
type ReviewVerdictSource interface {
	GetRecentReviewVerdictSummaries(ctx context.Context, itemID string, limit int) ([]ReviewVerdictSummary, error)
}

// priorVerdictsLimit bounds how many recent review verdicts assemblePriorVerdicts
// fetches, before budget enforcement trims further if needed.
const priorVerdictsLimit = 5

// assemblePriorVerdicts renders itemID's most recent review verdicts (fetched
// via source) into the PriorVerdicts bundle section.
func (a *DiagnosticBundleAssembler) assemblePriorVerdicts(ctx context.Context, source ReviewVerdictSource, itemID string) (string, error) {
	verdicts, err := source.GetRecentReviewVerdictSummaries(ctx, itemID, priorVerdictsLimit)
	if err != nil {
		return "", fmt.Errorf("diagnose: assemblePriorVerdicts: %w", err)
	}
	if len(verdicts) == 0 {
		return enforceBudget(BundleSectionPriorVerdicts, "(no prior review verdicts)", a.cfg), nil
	}

	var sb strings.Builder
	for i, v := range verdicts {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "- %s (%s): %s", v.CreatedAt.Format(time.RFC3339), v.OverallOutcome, v.Summary)
	}
	return enforceBudget(BundleSectionPriorVerdicts, sb.String(), a.cfg), nil
}

// SessionSnapshot mirrors the session.InstanceSnapshot fields
// assembleSessionSnapshot reads (see DiagnosticBundleAssembler's doc comment
// for why this package can't import session.InstanceSnapshot directly).
type SessionSnapshot struct {
	Path   string
	Branch string
}

// InstanceSnapshotter is the minimal *session.Instance surface
// assembleSessionSnapshot depends on: only Snapshot(), never the raw
// Path/Branch fields session.Instance also exposes. Scoping the dependency to
// this interface (rather than to *session.Instance's exported fields, which
// this package can't reference anyway per the import-cycle note above) makes
// a raw-field read impossible to compile against, not just a lint violation
// -- see .claude/rules/instance-lock-free-reads.md. Whatever call site in
// package session constructs a SessionSnapshot to satisfy this interface must
// do so from inst.Snapshot(), never inst.Path/inst.Branch.
type InstanceSnapshotter interface {
	Snapshot() SessionSnapshot
}

// assembleSessionSnapshot builds the SessionSnapshot bundle section content
// from inst.Snapshot() only.
func (a *DiagnosticBundleAssembler) assembleSessionSnapshot(inst InstanceSnapshotter) string {
	snap := inst.Snapshot()
	content := fmt.Sprintf("Path: %s\nBranch: %s", snap.Path, snap.Branch)
	return enforceBudget(BundleSectionSessionSnapshot, content, a.cfg)
}

// LogWindow bounds assembleRecentLogs's scan to roughly one session's
// lifetime, so unrelated sessions' log lines don't crowd out the ones
// actually relevant to the item being diagnosed. Both bounds are optional --
// a zero Start/End means "unbounded" on that side.
type LogWindow struct {
	Start time.Time
	End   time.Time
}

// maxLogTailBytes bounds how much of the log file assembleRecentLogs reads
// from the end before filtering to LogWindow, so a multi-GB log doesn't get
// read in full just to find one session's window.
const maxLogTailBytes = 8 * 1024 * 1024

// logLineTime mirrors just the "time" field slog's JSON handler writes per
// line (log/log.go's structured logger) -- enough to filter by LogWindow
// without parsing the rest of the record.
type logLineTime struct {
	Time time.Time `json:"time"`
}

// assembleRecentLogs tails the log file resolved via log.GetLogFilePath (which
// itself resolves through log.GetConfigDir()'s priority list), keeping only
// lines whose "time" field falls within window, and returns the budget-enforced
// Logs bundle section content.
func (a *DiagnosticBundleAssembler) assembleRecentLogs(window LogWindow) (string, error) {
	path, err := log.GetLogFilePath(nil)
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleRecentLogs: resolve log file: %w", err)
	}

	f, err := os.Open(path) //nolint:gosec // path comes from log.GetLogFilePath, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return enforceBudget(BundleSectionLogs, "(no log file found)", a.cfg), nil
		}
		return "", fmt.Errorf("diagnose: assembleRecentLogs: open %s: %w", path, err)
	}
	defer f.Close()

	if info, statErr := f.Stat(); statErr == nil && info.Size() > maxLogTailBytes {
		if _, seekErr := f.Seek(-maxLogTailBytes, io.SeekEnd); seekErr != nil {
			return "", fmt.Errorf("diagnose: assembleRecentLogs: seek %s: %w", path, seekErr)
		}
	}

	content, err := scanLogWindow(f, window)
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleRecentLogs: scan %s: %w", path, err)
	}
	if content == "" {
		content = "(no log lines in session window)"
	}
	return enforceBudget(BundleSectionLogs, content, a.cfg), nil
}

func scanLogWindow(r io.Reader, window LogWindow) (string, error) {
	var sb strings.Builder

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !logLineInWindow(line, window) {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan log lines: %w", err)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// logLineInWindow reports whether line's "time" field (if present and
// parseable) falls within window. A line with no parseable timestamp is kept
// rather than dropped -- an unrecognized line shape shouldn't silently
// disappear from a diagnostic bundle. An unbounded window (both ends zero)
// always keeps the line.
func logLineInWindow(line string, window LogWindow) bool {
	if window.Start.IsZero() && window.End.IsZero() {
		return true
	}

	var ts logLineTime
	if err := json.Unmarshal([]byte(line), &ts); err != nil || ts.Time.IsZero() {
		return true
	}
	if !window.Start.IsZero() && ts.Time.Before(window.Start) {
		return false
	}
	if !window.End.IsZero() && ts.Time.After(window.End) {
		return false
	}
	return true
}

// assembleDiff builds the Diff bundle section content from worktreePath's
// current uncommitted changes against its HEAD commit, via go-git directly
// (github.com/go-git/go-git/v5) rather than a git subshell, per the
// prefer-go-git-over-subshells skill. worktreePath is expected to be the
// caller's Workspace().ActiveDir (package diagnose can't import package
// session for the Workspace/Instance types themselves -- see SessionSnapshot's
// doc comment for the import-cycle reason). This deliberately does not reuse
// session/git.GitWorktree.Diff(), for the same reason: package session/git
// transitively imports session/tmux, which imports this package.
func (a *DiagnosticBundleAssembler) assembleDiff(worktreePath string) (string, error) {
	repo, err := gogit.PlainOpen(worktreePath)
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleDiff: open %s: %w", worktreePath, err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleDiff: resolve HEAD: %w", err)
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleDiff: read HEAD commit: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleDiff: read HEAD tree: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleDiff: open worktree: %w", err)
	}
	status, err := worktree.Status()
	if err != nil {
		return "", fmt.Errorf("diagnose: assembleDiff: status: %w", err)
	}

	content := renderWorkingTreeDiff(status, headTree, worktreePath)
	if content == "" {
		content = "(no uncommitted changes)"
	}
	return enforceBudget(BundleSectionDiff, content, a.cfg), nil
}

// renderWorkingTreeDiff builds a simple +/- line-level diff (not a full
// unified-hunk patch -- that's session/git/diff.go's much larger job, which
// this package can't import; see assembleDiff's doc comment) between
// headTree and each changed path's current on-disk content under
// worktreePath.
func renderWorkingTreeDiff(status gogit.Status, headTree *object.Tree, worktreePath string) string {
	paths := make([]string, 0, len(status))
	for path := range status {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var sb strings.Builder
	for _, path := range paths {
		entry := status[path]
		if entry.Worktree == gogit.Unmodified && entry.Staging == gogit.Unmodified {
			continue
		}

		oldContent := readTreeFileContent(headTree, path)
		newContent := readWorktreeFileContent(worktreePath, path)
		if oldContent == newContent {
			continue
		}

		fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
		// gogitdiff.Do is go-git's own line-oriented wrapper around
		// diffmatchpatch (see its doc comment) -- used instead of a raw
		// character-mode dmp.DiffMain so changed lines come back as whole
		// Insert/Delete chunks rather than sub-line character runs.
		for _, d := range gogitdiff.Do(oldContent, newContent) {
			switch d.Type {
			case dmp.DiffInsert:
				writePrefixedLines(&sb, "+", d.Text)
			case dmp.DiffDelete:
				writePrefixedLines(&sb, "-", d.Text)
			}
		}
	}
	return sb.String()
}

// readTreeFileContent returns path's content in tree, or "" if path doesn't
// exist there (a new, untracked file).
func readTreeFileContent(tree *object.Tree, path string) string {
	f, err := tree.File(path)
	if err != nil {
		return ""
	}
	content, err := f.Contents()
	if err != nil {
		return ""
	}
	return content
}

// readWorktreeFileContent returns path's current on-disk content under root,
// or "" if it doesn't exist there (a deleted file).
func readWorktreeFileContent(root, path string) string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) //nolint:gosec // path comes from go-git's own status/tree listing
	if err != nil {
		return ""
	}
	return string(data)
}

// writePrefixedLines writes each line of text to sb, prefixed with prefix
// (diffmatchpatch's DiffInsert/DiffDelete chunks are UTF-8 text spans, not
// necessarily whole lines, so a chunk can legitimately split mid-line -- this
// is a diagnostic rendering, not a byte-exact patch, so that's acceptable).
func writePrefixedLines(sb *strings.Builder, prefix, text string) {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for _, line := range lines {
		fmt.Fprintf(sb, "%s%s\n", prefix, line)
	}
}
