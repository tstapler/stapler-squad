package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// DiagnosticSpawner is the narrow slice of *SessionService DiagnosticService
// needs to dispatch a hidden, one-shot diagnostic agent — mirrors
// session.ReviewGateSpawner's shape for the sibling dispatch case.
type DiagnosticSpawner interface {
	SpawnDiagnosticSession(ctx context.Context, item *session.BacklogItemData, prompt string) (*session.Instance, error)
}

// DiagnosticService backs the DiagnosticService RPCs for "Diagnose & Nudge"
// (backlog item 68964304): AssembleDiagnosticBundle (read) and
// DispatchDiagnose (write), mirroring GuidanceRequestService's read/write
// split. Instance lookup for linked-session state uses the same
// poller-then-external-discovery fallback chain as TerminalService.
type DiagnosticService struct {
	storage      *session.Storage
	spawner      DiagnosticSpawner
	poller       *session.ReviewQueuePoller
	extDiscovery *session.ExternalSessionDiscovery
}

// NewDiagnosticService creates a DiagnosticService. spawner may be nil in
// tests exercising only AssembleDiagnosticBundle. Wire poller/extDiscovery
// after construction via SetPoller/SetExternalDiscovery.
func NewDiagnosticService(storage *session.Storage, spawner DiagnosticSpawner) *DiagnosticService {
	return &DiagnosticService{storage: storage, spawner: spawner}
}

// SetPoller wires the live-instance poller for linked-session lookup.
func (d *DiagnosticService) SetPoller(p *session.ReviewQueuePoller) {
	d.poller = p
}

// SetExternalDiscovery wires external session discovery (mux-enabled sessions).
func (d *DiagnosticService) SetExternalDiscovery(disc *session.ExternalSessionDiscovery) {
	d.extDiscovery = disc
}

// findInstance mirrors TerminalService.findInstance's poller ->
// external-discovery fallback chain.
func (d *DiagnosticService) findInstance(id string) *session.Instance {
	if d.poller != nil {
		if inst := d.poller.FindInstance(id); inst != nil {
			return inst
		}
	}
	if d.extDiscovery != nil {
		if inst := d.extDiscovery.GetSession(id); inst != nil {
			return inst
		}
	}
	return nil
}

// +api: session:diagnose-assemble-bundle
// AssembleDiagnosticBundle builds the context bundle for item_id.
func (d *DiagnosticService) AssembleDiagnosticBundle(
	ctx context.Context,
	req *connect.Request[sessionv1.AssembleDiagnosticBundleRequest],
) (*connect.Response[sessionv1.AssembleDiagnosticBundleResponse], error) {
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	item, err := d.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item not found: %w", err))
	}

	bundle, err := d.buildBundle(ctx, item)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	prompt := bundle.Prompt()
	return connect.NewResponse(&sessionv1.AssembleDiagnosticBundleResponse{
		ItemId:          item.ID,
		Prompt:          prompt,
		Compacted:       bundle.Compacted,
		EstimatedTokens: int32(session.EstimateTokens(prompt)), //#nosec G115 -- estimate, bounded by maxDiagnosticBundleBytes
	}), nil
}

// +api: session:diagnose-dispatch
// DispatchDiagnose spawns a hidden, one-shot diagnostic session carrying the
// assembled bundle. See DiagnosticService's doc comment for the
// nudge-eligibility handling.
func (d *DiagnosticService) DispatchDiagnose(
	ctx context.Context,
	req *connect.Request[sessionv1.DispatchDiagnoseRequest],
) (*connect.Response[sessionv1.DispatchDiagnoseResponse], error) {
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	if d.spawner == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("diagnostic dispatch is not configured"))
	}
	item, err := d.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item not found: %w", err))
	}

	bundle, err := d.buildBundle(ctx, item)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	nudgeAllowed, nudgeDisallowedReason := d.evaluateNudgeEligibility(ctx, item.ID, req.Msg.StuckReason, req.Msg.NudgeTargetSessionUuid)

	prompt := buildDiagnosePrompt(bundle, req.Msg.NudgeTargetSessionUuid, nudgeAllowed, nudgeDisallowedReason)
	inst, spawnErr := d.spawner.SpawnDiagnosticSession(ctx, item, prompt)
	if spawnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("dispatch diagnostic session: %w", spawnErr))
	}

	if _, createErr := d.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleDiagnose,
	}); createErr != nil {
		log.Warn("[DiagnosticService] failed to link diagnostic ItemSession", "item", item.ID, "session", inst.UUID, "err", createErr)
	}

	return connect.NewResponse(&sessionv1.DispatchDiagnoseResponse{
		DiagnosticSessionUuid: inst.UUID,
		NudgeAllowed:          nudgeAllowed,
		NudgeDisallowedReason: nudgeDisallowedReason,
	}), nil
}

// evaluateNudgeEligibility applies AC2's two-part gate: the item's
// cap/cooldown (session.DiagnoseNudgeAllowed) and the target's current
// idle-status (session.CheckNudgeEligible). No target, or no stuck reason to
// key the cap on, means diagnose-only — nudgeAllowed is false, not an error.
// The write-time pane-ownership re-verification
// (session.VerifyPaneOwnershipBeforeWrite) happens later, immediately before
// the actual nudge write — see the diagnose_nudge_session MCP tool.
func (d *DiagnosticService) evaluateNudgeEligibility(ctx context.Context, itemID, stuckReason, targetSessionUUID string) (allowed bool, disallowedReason string) {
	if targetSessionUUID == "" {
		return false, "no nudge target specified"
	}
	if stuckReason == "" {
		return false, "no stuck reason specified — nudge cap cannot be evaluated"
	}
	allowed, disallowedReason, err := d.storage.DiagnoseNudgeAllowed(ctx, itemID, domain.StuckReason(stuckReason))
	if err != nil {
		return false, fmt.Sprintf("could not evaluate nudge eligibility: %v", err)
	}
	if !allowed {
		return false, disallowedReason
	}
	target := d.findInstance(targetSessionUUID)
	if target == nil {
		return false, "nudge target session not found or not live"
	}
	if eligErr := session.CheckNudgeEligible(target); eligErr != nil {
		return false, eligErr.Error()
	}
	return true, ""
}

// buildDiagnosePrompt wraps the assembled bundle with the dispatched agent's
// instructions: which MCP tools it may call, and whether a nudge is in its
// action space right now (AC2/AC4 — action-space narrowing is enforced here
// in the prompt as well as by the diagnose_nudge_session tool itself
// refusing a disallowed nudge, defense in depth).
func buildDiagnosePrompt(bundle session.DiagnosticBundle, nudgeTargetUUID string, nudgeAllowed bool, nudgeDisallowedReason string) string {
	var sb strings.Builder
	sb.WriteString("You are a diagnostic agent investigating a backlog item. You have been given a context bundle below.\n\n")
	sb.WriteString("Based on the evidence, decide ONE of:\n")
	sb.WriteString("1. File a backlog item via create_backlog_item with concrete evidence (log lines, timestamps, session UUIDs) if you find a genuine product/infra defect.\n")
	sb.WriteString("2. Post a diagnostic note via post_backlog_update if the situation is inconclusive or informational only.\n")
	if nudgeAllowed {
		fmt.Fprintf(&sb, "3. Nudge the linked session (%s) via diagnose_nudge_session if it is a simple stall — a session that just needs redirection, not a bug.\n", nudgeTargetUUID)
	} else {
		fmt.Fprintf(&sb, "3. Nudging is NOT available for this dispatch (%s) — do not attempt resume_session/steer_session/write_to_session/diagnose_nudge_session on it. File a bug or post a note instead.\n", nudgeDisallowedReason)
	}
	sb.WriteString("\nWhen you conclude your investigation, call submit_diagnosis_result exactly once with your outcome and evidence.\n\n")
	sb.WriteString(bundle.Prompt())
	return sb.String()
}

// buildBundle gathers item/AC/history/verdicts/linked-session/git data and
// hands it to session.BuildDiagnosticBundle (the pure assembly/compaction
// logic — see session/diagnose_bundle.go).
func (d *DiagnosticService) buildBundle(ctx context.Context, item *session.BacklogItemData) (session.DiagnosticBundle, error) {
	acCriteria, acErr := session.ParseAcCriteria(item.AcceptanceCriteria)
	if acErr != nil {
		log.Warn("[DiagnosticService] ParseAcCriteria failed", "item", item.ID, "err", acErr)
	}

	history, err := d.storage.ListActivityNotesForItem(ctx, item.ID)
	if err != nil {
		log.Warn("[DiagnosticService] ListActivityNotesForItem failed", "item", item.ID, "err", err)
	}

	verdictSummaries, err := d.storage.GetRecentReviewVerdictSummaries(ctx, item.ID, 5)
	if err != nil {
		log.Warn("[DiagnosticService] GetRecentReviewVerdictSummaries failed", "item", item.ID, "err", err)
	}
	verdicts := make([]session.DiagnosticReviewVerdict, 0, len(verdictSummaries))
	for _, v := range verdictSummaries {
		verdicts = append(verdicts, session.DiagnosticReviewVerdict{Outcome: v.OverallOutcome, Summary: v.Summary, CreatedAt: v.CreatedAt})
	}

	linked, gitDiff, gitDiffTruncated, gitLog := d.buildLinkedSessions(ctx, item.ID)

	in := session.DiagnosticBundleInput{
		ItemID:           item.ID,
		ItemTitle:        item.Title,
		ItemDescription:  item.Description,
		ItemStatus:       item.Status,
		AcCriteria:       acCriteria,
		ActivityHistory:  history,
		ReviewVerdicts:   verdicts,
		LinkedSessions:   linked,
		GitDiff:          gitDiff,
		GitDiffTruncated: gitDiffTruncated,
		GitLog:           gitLog,
	}
	return session.BuildDiagnosticBundle(in), nil
}

// buildLinkedSessions resolves each of item's ItemSessions to a
// DiagnosticLinkedSession (live status/program from the poller if the
// session is still running, branch from its worktree row) and picks the
// first session with a resolvable worktree to source the bundle's git
// diff/log from — extracted from buildBundle purely to stay under the
// funlen gate; this is not independently reusable.
func (d *DiagnosticService) buildLinkedSessions(ctx context.Context, itemID string) (linked []session.DiagnosticLinkedSession, gitDiff string, gitDiffTruncated bool, gitLog string) {
	itemSessions, err := d.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		log.Warn("[DiagnosticService] ListItemSessions failed", "item", itemID, "err", err)
	}
	linked = make([]session.DiagnosticLinkedSession, 0, len(itemSessions))
	for _, is := range itemSessions {
		ls := session.DiagnosticLinkedSession{SessionUUID: is.SessionUUID, Role: is.Role, LastActivity: lastActivityFor(is)}
		if inst := d.findInstance(is.SessionUUID); inst != nil {
			if ctrl := inst.GetController(); ctrl != nil {
				status, _ := ctrl.GetCurrentStatus() // second value is a description string, not an error
				ls.Status = status.String()
			}
			ls.Program = inst.Snapshot().Program
			ls.RecentLog = readRecentSessionLog(is.SessionUUID)
		}
		if wt, wtErr := d.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID); wtErr == nil {
			ls.Branch = wt.BranchName
			if gitDiff == "" && wt.WorktreePath != "" {
				gitDiff, gitDiffTruncated, _ = session.GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
				gitLog = gitLogSummary(ctx, wt.WorktreePath, wt.BaseCommitSHA, wt.BranchName)
			}
		}
		linked = append(linked, ls)
	}
	return linked, gitDiff, gitDiffTruncated, gitLog
}

// lastActivityFor picks the best available "last activity" timestamp for an
// ItemSession, in descending order of freshness — LastProgressAt refreshes
// on every report_progress call, LastCommitAt only on new commits, CreatedAt
// as the final fallback so the field is never a zero time.
func lastActivityFor(is session.ItemSessionSummary) time.Time {
	if is.LastProgressAt != nil {
		return *is.LastProgressAt
	}
	if is.LastCommitAt != nil {
		return *is.LastCommitAt
	}
	return is.CreatedAt
}

// maxRecentLogBytes bounds how much of a session's own log file
// readRecentSessionLog tails — kept well under maxDiagnosticBundleBytes
// since a bundle can have several linked sessions.
const maxRecentLogBytes = 20_000

// readRecentSessionLog best-effort-tails the last maxRecentLogBytes of
// sessionUUID's own log file (log.GetSessionLogFilePath) — "" (not an error)
// if the file doesn't exist or can't be read, since a diagnostic bundle
// degrades gracefully rather than failing over missing logs.
func readRecentSessionLog(sessionUUID string) string {
	path, err := log.GetSessionLogFilePath(nil, sessionUUID)
	if err != nil {
		return ""
	}
	f, err := os.Open(path) //#nosec G304 -- path is built from a sanitized session UUID via GetSessionLogFilePath
	if err != nil {
		return ""
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ""
	}
	size := info.Size()
	offset := int64(0)
	if size > maxRecentLogBytes {
		offset = size - maxRecentLogBytes
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return ""
	}
	buf := make([]byte, size-offset)
	n, readErr := io.ReadFull(f, buf)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		return ""
	}
	return string(buf[:n])
}

// gitLogOneLineCommits caps how many commits gitLogSummary includes.
const gitLogOneLineCommits = 30

// gitLogSummary returns a bounded `git log --oneline` of baseSHA..headRef in
// dir, best-effort ("" on any error) — mirrors GetGitDiff's own
// best-effort-on-failure convention for supplementary (not load-bearing)
// bundle content.
func gitLogSummary(ctx context.Context, dir, baseSHA, headRef string) string {
	if dir == "" || baseSHA == "" || headRef == "" {
		return ""
	}
	logCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rangeArg := fmt.Sprintf("%s..%s", baseSHA, headRef)
	out, err := safeexec.CommandContext(logCtx, "git", "-C", dir, "log", "--oneline", "-n", fmt.Sprintf("%d", gitLogOneLineCommits), rangeArg).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
