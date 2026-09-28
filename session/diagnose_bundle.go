package session

// diagnose_bundle.go — pure context-bundle assembly for Diagnose & Nudge
// (AC0/AC1). BuildDiagnosticBundle takes pre-fetched data (the caller,
// server/services.DiagnosticService, has the Storage/poller/git access this
// package doesn't) and stays a pure function so its section-selection and
// compaction logic is exhaustively unit-testable without a live database,
// tmux, or LLM call.
//
// Compaction (AC1) never truncates mid-section: when the rendered prompt
// exceeds DiagnosticBundleTokenBudget, whole omittable sections are dropped
// (replaced with a one-line placeholder), lowest-priority first, until the
// bundle fits or nothing more can be dropped. A full LLM-based summarization
// pipeline (mirroring HandoffSummaryGenerator's own GENERATING/ERROR-state
// table) is deferred — this graceful degradation satisfies "never hard
// truncation" without that additional subsystem; see the item's PR
// description for the explicit scope note.
//
// Every untrusted string (item description, activity-note/log/diff content)
// is XML-tag-escaped and wrapped in a named delimiter tag before being
// folded into the prompt, mirroring buildOrchestrationPrompt's
// anti-prompt-injection technique (session/autonomous_driver.go) — the
// dispatched diagnostic agent's action space includes filing bugs and
// nudging a live session, so this is treated as a hard requirement here,
// not optional.

import (
	"fmt"
	"strings"
	"time"
)

// diagnosticTagEscaper neutralizes the two characters that could otherwise
// let untrusted content close its delimiter tag early and spoof outer
// prompt structure — same technique and rationale as
// autonomous_driver.go's lastNudgeTagEscaper.
var diagnosticTagEscaper = strings.NewReplacer("<", "&lt;", ">", "&gt;")

// wrapDiagnosticSection XML-escapes content and wraps it in a named
// delimiter tag. Deliberately does NOT truncate or strip here — a length cap
// at this layer would (a) silently hard-truncate mid-section, which AC1
// explicitly forbids, and (b) hide oversized content from the whole-bundle
// compaction loop in BuildDiagnosticBundle, which is the one place section
// dropping is allowed to happen. Escaping (not sanitizeField's HTML-tag
// stripping) is used so legitimate content containing angle brackets (a git
// diff's C++ template syntax, a shell redirect) survives losslessly while
// still being unable to spoof a delimiter tag boundary.
func wrapDiagnosticSection(tag, content string) string {
	if strings.TrimSpace(content) == "" {
		return fmt.Sprintf("<%s>(none)</%s>", tag, tag)
	}
	escaped := diagnosticTagEscaper.Replace(content)
	return fmt.Sprintf("<%s>\n%s\n</%s>", tag, escaped, tag)
}

// maxDiagnosticBundleBytes is the absolute last-resort ceiling on the
// rendered prompt (validation.md edge case 5: "compacted output itself is
// pathologically large... add an explicit ceiling"), applied only once
// BuildDiagnosticBundle has already dropped every omittable section and the
// bundle is STILL over budget — meaning the overage is in non-omittable core
// content (item description, AC, verdicts, linked-session summary) that
// compaction-by-omission cannot address. Sized well above
// DiagnosticBundleTokenBudget so it never fires in the normal
// compaction-by-omission path this file's tests exercise; it is a safety
// net, not the primary mechanism.
const maxDiagnosticBundleBytes = 4 * DiagnosticBundleTokenBudget * bytesPerTokenEstimate

// DiagnosticLinkedSession captures the point-in-time state of one session
// linked to a backlog item — AC0's "linked session(s)' Snapshot()-based
// state". Status is a best-effort detection-status description; empty means
// the session is no longer live (edge case: target terminated before
// dispatch).
type DiagnosticLinkedSession struct {
	SessionUUID  string
	Role         string
	Status       string
	Program      string
	Branch       string
	LastActivity time.Time
	RecentLog    string
}

// DiagnosticReviewVerdict is a rendering-ready projection of
// ReviewVerdictSummary.
type DiagnosticReviewVerdict struct {
	Outcome   string
	Summary   string
	CreatedAt time.Time
}

// DiagnosticBundleInput is everything BuildDiagnosticBundle needs. Every
// field is pre-fetched by the caller.
type DiagnosticBundleInput struct {
	ItemID           string
	ItemTitle        string
	ItemDescription  string
	ItemStatus       string
	AcCriteria       []AcCriterion
	ActivityHistory  []ActivityNoteData
	ReviewVerdicts   []DiagnosticReviewVerdict
	LinkedSessions   []DiagnosticLinkedSession
	GitDiff          string
	GitDiffTruncated bool
	GitLog           string
}

// DiagnosticBundleSection is one of the bundle's rendered sections.
// Omittable sections may be dropped entirely (never truncated mid-text)
// during compaction — see dropDiagnosticSection.
type DiagnosticBundleSection struct {
	Tag       string
	Content   string
	Omittable bool
}

// DiagnosticBundle is the assembled, sanitized, budget-checked context
// bundle for a Diagnose & Nudge dispatch.
type DiagnosticBundle struct {
	ItemID    string
	Sections  []DiagnosticBundleSection
	Compacted bool

	// renderedPrompt is computed once by applyHardCeiling (called at the end
	// of BuildDiagnosticBundle) so Prompt() is a cheap accessor rather than
	// re-joining Sections — and so the hard-ceiling truncation, when it
	// fires, is applied exactly once rather than on every Prompt() call.
	renderedPrompt string
}

// diagnosticSectionDropOrder is the lowest-priority-first order compaction
// drops omittable sections in: recent logs first (supplementary context),
// then git log, then git diff (the concrete evidence a diagnostic agent
// needs last) — the reverse of review_gate.go's diff-first priority, since
// this bundle's job is triage across several possible causes, not grading a
// specific diff.
var diagnosticSectionDropOrder = []string{"recent_logs", "git_log", "git_diff"}

// BuildDiagnosticBundle assembles the six required sections (AC0:
// description/AC/status/history, review verdicts, linked-session state,
// recent logs, git diff, git log) and compacts by omission if the rendered
// prompt would exceed DiagnosticBundleTokenBudget (AC1).
func BuildDiagnosticBundle(in DiagnosticBundleInput) DiagnosticBundle {
	bundle := DiagnosticBundle{
		ItemID: in.ItemID,
		Sections: []DiagnosticBundleSection{
			{Tag: "item", Content: renderItemSection(in), Omittable: false},
			{Tag: "review_verdicts", Content: renderVerdictsSection(in.ReviewVerdicts), Omittable: false},
			{Tag: "linked_sessions", Content: renderLinkedSessionsSection(in.LinkedSessions), Omittable: false},
			{Tag: "recent_logs", Content: renderRecentLogsSection(in.LinkedSessions), Omittable: true},
			{Tag: "git_diff", Content: renderGitDiffSection(in), Omittable: true},
			{Tag: "git_log", Content: wrapDiagnosticSection("git_log", in.GitLog), Omittable: true},
		},
	}

	for ExceedsDiagnosticBundleBudget(bundle.Prompt()) {
		if !dropNextOmittableSection(&bundle) {
			break
		}
	}
	bundle.applyHardCeiling()
	return bundle
}

// applyHardCeiling is the safety net described at maxDiagnosticBundleBytes:
// only reachable once every omittable section has already been dropped and
// the bundle is still oversized, so the overage lives in non-omittable core
// content. Stores the (possibly truncated) rendered prompt so Prompt() never
// needs to recompute it, and marks Compacted so callers can tell the bundle
// was altered either way.
func (b *DiagnosticBundle) applyHardCeiling() {
	rendered := b.renderSections()
	if len(rendered) > maxDiagnosticBundleBytes {
		rendered = rendered[:maxDiagnosticBundleBytes] + "\n\n(bundle truncated at an absolute size ceiling — core content, not an omittable section, exceeded the budget)"
		b.Compacted = true
	}
	b.renderedPrompt = rendered
}

// dropNextOmittableSection replaces the highest-drop-priority remaining
// omittable section's content with a placeholder, in
// diagnosticSectionDropOrder. Returns false once nothing left is droppable.
func dropNextOmittableSection(bundle *DiagnosticBundle) bool {
	for _, tag := range diagnosticSectionDropOrder {
		for i := range bundle.Sections {
			s := &bundle.Sections[i]
			if s.Tag != tag || !s.Omittable {
				continue
			}
			s.Content = fmt.Sprintf("<%s>(omitted — bundle exceeded the %d-token budget; see the item's full history/logs directly if needed)</%s>", tag, DiagnosticBundleTokenBudget, tag)
			s.Omittable = false // already omitted; don't select it again
			bundle.Compacted = true
			return true
		}
	}
	return false
}

// Prompt returns the text handed to the dispatched diagnostic agent. Before
// BuildDiagnosticBundle's final applyHardCeiling call, this recomputes from
// Sections on every call (used by the in-progress compaction loop to
// re-check the budget as sections are dropped); afterward it returns the
// cached, ceiling-checked renderedPrompt.
func (b DiagnosticBundle) Prompt() string {
	if b.renderedPrompt != "" {
		return b.renderedPrompt
	}
	return b.renderSections()
}

// renderSections joins every section's content, unconditionally recomputing
// from Sections — the shared body Prompt() and applyHardCeiling both use.
func (b DiagnosticBundle) renderSections() string {
	var sb strings.Builder
	for _, s := range b.Sections {
		sb.WriteString(s.Content)
		sb.WriteString("\n\n")
	}
	if b.Compacted {
		sb.WriteString("(Note: one or more sections above were omitted to stay within the context budget.)\n")
	}
	return sb.String()
}

func renderItemSection(in DiagnosticBundleInput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Title: %s\nStatus: %s\n\nDescription:\n%s\n\nAcceptance Criteria:\n%s\n\nHistory:\n%s",
		truncateField(in.ItemTitle, 500),
		truncateField(in.ItemStatus, 100),
		in.ItemDescription,
		buildAcChecklist(in.AcCriteria),
		renderActivityHistory(in.ActivityHistory),
	)
	return wrapDiagnosticSection("item", sb.String())
}

func renderActivityHistory(notes []ActivityNoteData) string {
	if len(notes) == 0 {
		return "(no activity history)"
	}
	var sb strings.Builder
	for _, n := range notes {
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", n.CreatedAt.Format(time.RFC3339), truncateField(n.AuthorSessionTitle, 100), sanitizeField(n.Message, 1000))
	}
	return sb.String()
}

func renderVerdictsSection(verdicts []DiagnosticReviewVerdict) string {
	if len(verdicts) == 0 {
		return wrapDiagnosticSection("review_verdicts", "(no prior review verdicts)")
	}
	var sb strings.Builder
	for _, v := range verdicts {
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", v.CreatedAt.Format(time.RFC3339), v.Outcome, sanitizeField(v.Summary, 2000))
	}
	return wrapDiagnosticSection("review_verdicts", sb.String())
}

func renderLinkedSessionsSection(sessions []DiagnosticLinkedSession) string {
	if len(sessions) == 0 {
		return wrapDiagnosticSection("linked_sessions", "(no linked sessions — no nudge target exists for this item)")
	}
	var sb strings.Builder
	for _, s := range sessions {
		status := s.Status
		if status == "" {
			status = "(not live)"
		}
		fmt.Fprintf(&sb, "- session=%s role=%s status=%s program=%s branch=%s last_activity=%s\n",
			s.SessionUUID, s.Role, status, s.Program, s.Branch, s.LastActivity.Format(time.RFC3339))
	}
	return wrapDiagnosticSection("linked_sessions", sb.String())
}

func renderRecentLogsSection(sessions []DiagnosticLinkedSession) string {
	if len(sessions) == 0 {
		return wrapDiagnosticSection("recent_logs", "(no linked sessions)")
	}
	var sb strings.Builder
	for _, s := range sessions {
		if strings.TrimSpace(s.RecentLog) == "" {
			continue
		}
		fmt.Fprintf(&sb, "--- session %s ---\n%s\n", s.SessionUUID, s.RecentLog)
	}
	return wrapDiagnosticSection("recent_logs", sb.String())
}

func renderGitDiffSection(in DiagnosticBundleInput) string {
	content := in.GitDiff
	if in.GitDiffTruncated {
		content = "[WARNING: diff truncated upstream]\n" + content
	}
	return wrapDiagnosticSection("git_diff", content)
}
