package services

// diagnose_dispatcher.go — Phase 5 of
// project_plans/backlog-diagnose-and-nudge/implementation/plan.md: Epic 5.1
// (Stories 5.1.1 RequestDiagnosis + in-flight guard, 5.1.2 dispatch the
// headless-diagnose-* session, 5.1.3 dispatch-failure handling) plus Story
// 5.2.2's notifyDiagnoseEvent. Story 5.2.1's DiagnoseDispatch ent schema +
// DiagnoseDispatchStore (diagnose_dispatch_store.go) already landed in an
// earlier phase; this file consumes it.
//
// This package is the adapter boundary session/diagnose's DTOs exist for:
// package diagnose cannot import package session (import-cycle constraint --
// see session/diagnose/bundle.go's and bundle_transcript.go's doc comments),
// so server/services, which can import both, is where a real
// *session.Instance/session.BacklogItemData/session.ItemSessionSummary/
// session.ReviewVerdictSummary/*session.HandoffSummaryGenerator gets adapted
// into diagnose's local BundleItem/SessionHistoryEntry/ReviewVerdictSummary/
// InstanceSnapshotter/LinkedTranscriptSummaryGenerator shapes.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// HeadlessDiagnosticSessionIDPrefix is prepended to every diagnose dispatch's
// diagnostic session UUID ("headless-diagnose-<itemID>-<dispatch uuid>").
// The "headless-" prefix is what makes the dispatched session classify as a
// Synthetic Session with no backing Instance in the web UI
// (web-app/src/lib/backlog/sessionKind.ts's classifySessionKind), the same
// convention headlessTriageUUIDPrefix (backlog_service_triage.go) already
// established for headless triage/re-review sessions.
const HeadlessDiagnosticSessionIDPrefix = "headless-diagnose-"

// DiagnosticDataSource is the narrow session-package surface DiagnoseDispatcher
// needs to gather what DiagnosticBundleAssembler requires for one item, in
// real session/domain types. Satisfied by *session.Storage.
type DiagnosticDataSource interface {
	GetBacklogItem(ctx context.Context, itemID string) (*session.BacklogItemData, error)
	ListItemSessions(ctx context.Context, itemID string) ([]session.ItemSessionSummary, error)
	GetRecentReviewVerdictSummaries(ctx context.Context, itemID string, limit int) ([]session.ReviewVerdictSummary, error)
}

// InstanceLookup resolves a target session's live *session.Instance, when one
// exists, for the SessionSnapshot/Diff bundle sections. Scoped narrowly
// (rather than exposing a whole session registry) so tests can inject a fake
// without a real Instance/tmux/worktree stack.
type InstanceLookup interface {
	// FindInstance returns sessionUUID's live Instance, or found=false when it
	// has none (already ended, or a synthetic headless-* session that was
	// never Instance-backed to begin with).
	FindInstance(sessionUUID string) (inst *session.Instance, found bool)
}

// HeadlessDiagnosticSessionRequest is everything a
// HeadlessDiagnosticSessionCreator needs to launch the dispatched diagnostic
// agent.
type HeadlessDiagnosticSessionRequest struct {
	ItemID                string
	DiagnosticSessionUUID string
	RepoPath              string
	Prompt                string
}

// HeadlessDiagnosticSessionCreator is the narrow session-creation surface
// DiagnoseDispatcher.dispatch needs. Its production implementation is meant
// to reuse this codebase's existing headless dispatch primitive -- the same
// bounded, one-shot Claude Code call plus synthetic (DB-only, no tmux
// Instance) ItemSession row TriggerTriage/re-review sessions already use
// (backlog_service_trigger_triage.go's headlessPool + storage.CreateItemSession)
// -- rather than DiagnoseDispatcher reinventing worktree/tmux setup itself.
//
// Deliberately left as a narrow interface here rather than wired to a
// concrete implementation in this file: constructing a real
// DiagnoseDispatcher with a concrete creator is deferred to whichever later
// phase exposes RequestDiagnosis over RPC (mirrors how DiagnoseDispatchStore
// itself is constructed elsewhere, in server/mcp/server.go, from a concrete
// *session.Storage via NewDiagnoseDispatchStore -- see that function's doc
// comment). See this file's package doc comment / the worker report that
// introduced it for the full rationale.
type HeadlessDiagnosticSessionCreator interface {
	CreateHeadlessDiagnosticSession(ctx context.Context, req HeadlessDiagnosticSessionRequest) error
}

// diagnoseEventNotifier is the narrow live-notification surface
// notifyDiagnoseEvent needs -- satisfied by *EventBusNotifier's existing
// Notify method (backlog_notifier.go). Kept as a narrow interface so tests
// can assert on published title/message content without a real
// *events.EventBus, and so a nil notifier (no bus wired) safely no-ops,
// mirroring EventBusNotifier.Notify's own nil-receiver guard.
type diagnoseEventNotifier interface {
	Notify(itemID, title, message string, notificationType int32, urgent, important bool)
}

// RequestDiagnosisResult is RequestDiagnosis's outcome: either a fresh
// dispatch was recorded and launched, or an equivalent dispatch was already
// in flight for this item and nothing new was started.
type RequestDiagnosisResult struct {
	// AlreadyInFlight is true when a concurrent RequestDiagnosis call for the
	// same item was already in progress (Story 5.1.1's duplicate-call guard).
	// DispatchID/DiagnosticSessionUUID are unset in this case.
	AlreadyInFlight bool
	// DispatchID is the DiagnoseDispatch row's ID (Task 5.1.1d), set whenever
	// AlreadyInFlight is false -- regardless of whether dispatch (Story
	// 5.1.2) itself went on to succeed or fail (Story 5.1.3).
	DispatchID string
	// DiagnosticSessionUUID is the headless-diagnose-* ID assigned to this
	// dispatch's diagnostic session (Story 5.1.2).
	DiagnosticSessionUUID string
	// DispatchFailed is true when session creation itself failed
	// synchronously (Story 5.1.3): the DiagnoseDispatch row was still
	// recorded (Pending) and then marked Completed/DispatchFailed.
	DispatchFailed bool
	// FailureReason is set only when DispatchFailed is true.
	FailureReason string
}

// DiagnoseDispatcher orchestrates Story 5.1.1's RequestDiagnosis entry point:
// guard against a duplicate in-flight dispatch for the same item, assemble
// the diagnostic bundle (session/diagnose), persist a Pending DiagnoseDispatch
// row before ever creating the diagnostic session (architecture-review
// Blocker 1), then dispatch it (Story 5.1.2), translating a dispatch-creation
// failure into a DispatchFailed outcome (Story 5.1.3). Forked in shape from
// reconcileOrphanedTriageItems/retryOrphanedTriageWithBackoffGate
// (backlog_service_triage.go), per the plan's Pattern Decisions table.
type DiagnoseDispatcher struct {
	// diagnoseInFlight guards RequestDiagnosis's synchronous body (bundle
	// assembly through dispatch-creation) against a duplicate concurrent call
	// for the same item -- mirrors steerInFlight's LoadOrStore/defer-Delete
	// idiom (backlog_service_pr_fix_steer.go). It does not span the
	// diagnostic agent's own asynchronous investigation: the durable Pending
	// DiagnoseDispatchStore row (Task 5.1.1d), not this in-memory map, is
	// what makes an in-flight dispatch visible across a page refresh or
	// process restart (architecture-review Blocker 1).
	diagnoseInFlight sync.Map

	dispatchStore DiagnoseDispatchStore
	notifier      diagnoseEventNotifier

	dataSource  DiagnosticDataSource
	instances   InstanceLookup
	transcripts diagnose.LinkedTranscriptSummaryGenerator

	sessionCreator HeadlessDiagnosticSessionCreator

	assembler *diagnose.DiagnosticBundleAssembler
}

// DiagnoseDispatcherDeps groups NewDiagnoseDispatcher's dependencies. instances
// and transcripts may be nil when no live-Instance lookup or transcript
// summarizer is wired (e.g. a deployment/test with storage-only data) --
// AssembleDiagnosticBundle degrades the SessionSnapshot/Diff/LinkedTranscript
// sections to placeholders rather than failing bundle assembly.
// BundleTokenBudget is typically config.DiagnoseNudgeConfig.BundleTokenBudgetOrDefault().
type DiagnoseDispatcherDeps struct {
	DispatchStore     DiagnoseDispatchStore
	Notifier          diagnoseEventNotifier
	DataSource        DiagnosticDataSource
	Instances         InstanceLookup
	Transcripts       diagnose.LinkedTranscriptSummaryGenerator
	SessionCreator    HeadlessDiagnosticSessionCreator
	BundleTokenBudget int
}

// NewDiagnoseDispatcher constructs a DiagnoseDispatcher from deps.
func NewDiagnoseDispatcher(deps DiagnoseDispatcherDeps) *DiagnoseDispatcher {
	return &DiagnoseDispatcher{
		dispatchStore:  deps.DispatchStore,
		notifier:       deps.Notifier,
		dataSource:     deps.DataSource,
		instances:      deps.Instances,
		transcripts:    deps.Transcripts,
		sessionCreator: deps.SessionCreator,
		assembler:      diagnose.NewDiagnosticBundleAssembler(diagnose.DefaultDiagnosticBundleConfig(deps.BundleTokenBudget)),
	}
}

// reviewVerdictLimit bounds how many recent review verdicts RequestDiagnosis
// fetches for the PriorVerdicts bundle section, mirroring
// DiagnosticBundleAssembler's own priorVerdictsLimit (bundle.go) -- kept as a
// separate constant since that one is unexported to package diagnose.
const reviewVerdictLimit = 5

// RequestDiagnosis is Story 5.1.1's entry point: guards against a duplicate
// concurrent dispatch for itemID, assembles the diagnostic bundle, persists a
// Pending DiagnoseDispatch row, then dispatches the headless-diagnose-*
// session (Story 5.1.2), translating a dispatch-creation failure into a
// DispatchFailed outcome with no automatic retry (Story 5.1.3).
func (d *DiagnoseDispatcher) RequestDiagnosis(ctx context.Context, itemID string) (RequestDiagnosisResult, error) {
	if _, already := d.diagnoseInFlight.LoadOrStore(itemID, struct{}{}); already {
		log.Info("[DiagnoseDispatcher] diagnosis already in flight; rejecting concurrent attempt", "item", itemID)
		return RequestDiagnosisResult{AlreadyInFlight: true}, nil
	}
	defer d.diagnoseInFlight.Delete(itemID)

	item, itemSessions, err := d.loadItemAndSessions(ctx, itemID)
	if err != nil {
		return RequestDiagnosisResult{}, err
	}
	targetSessionUUID := resolveTargetSessionUUID(itemSessions)
	diagnosticSessionUUID := HeadlessDiagnosticSessionIDPrefix + itemID + "-" + uuid.New().String()

	// Task 5.1.1d / architecture-review Blocker 1: persist the Pending row
	// BEFORE the diagnostic session is created, so ListDiagnoseDispatches has
	// something durable to return even if a caller queries immediately after
	// (simulating a page refresh mid-dispatch), and before any dispatch
	// failure below.
	dispatchID, err := d.dispatchStore.Record(ctx, DiagnoseDispatchRequest{
		ItemID:                itemID,
		TargetSessionUUID:     targetSessionUUID,
		DiagnosticSessionUUID: diagnosticSessionUUID,
	})
	if err != nil {
		return RequestDiagnosisResult{}, fmt.Errorf("diagnose dispatcher: record pending dispatch for item %s: %w", itemID, err)
	}

	return d.dispatchAndRecord(ctx, dispatchID, diagnosticSessionUUID, item, itemSessions, targetSessionUUID), nil
}

// dispatchAndRecord renders the prompt, launches the headless-diagnose-*
// session (Story 5.1.2), and translates a dispatch-creation error into
// Story 5.1.3's DispatchFailed outcome -- split out of RequestDiagnosis purely
// to keep that method's own length manageable.
func (d *DiagnoseDispatcher) dispatchAndRecord(ctx context.Context, dispatchID, diagnosticSessionUUID string, item *session.BacklogItemData, itemSessions []session.ItemSessionSummary, targetSessionUUID string) RequestDiagnosisResult {
	prompt := diagnose.RenderPrompt(d.assembleBundle(ctx, item, itemSessions, targetSessionUUID))

	dispatchErr := d.sessionCreator.CreateHeadlessDiagnosticSession(ctx, HeadlessDiagnosticSessionRequest{
		ItemID:                item.ID,
		DiagnosticSessionUUID: diagnosticSessionUUID,
		RepoPath:              item.RepoPath,
		Prompt:                prompt,
	})
	if dispatchErr != nil {
		return d.handleDispatchFailure(ctx, dispatchID, diagnosticSessionUUID, item, dispatchErr)
	}

	log.Info("[DiagnoseDispatcher] dispatched diagnostic session", "item", item.ID, "dispatch", dispatchID, "session", diagnosticSessionUUID)
	return RequestDiagnosisResult{DispatchID: dispatchID, DiagnosticSessionUUID: diagnosticSessionUUID}
}

// loadItemAndSessions fetches itemID's BacklogItemData and its ItemSessions in
// one place, wrapping either failure with the same "diagnose dispatcher: ..."
// error-context convention this file uses throughout.
func (d *DiagnoseDispatcher) loadItemAndSessions(ctx context.Context, itemID string) (*session.BacklogItemData, []session.ItemSessionSummary, error) {
	item, err := d.dataSource.GetBacklogItem(ctx, itemID)
	if err != nil {
		return nil, nil, fmt.Errorf("diagnose dispatcher: load item %s: %w", itemID, err)
	}
	itemSessions, err := d.dataSource.ListItemSessions(ctx, itemID)
	if err != nil {
		return nil, nil, fmt.Errorf("diagnose dispatcher: list sessions for item %s: %w", itemID, err)
	}
	return item, itemSessions, nil
}

// handleDispatchFailure implements Story 5.1.3: a dispatch-creation error
// updates the SAME Pending row Record already wrote to
// Completed/DispatchFailed -- never a second row -- and never retries
// automatically (research/pitfalls.md §4's double-dispatch risk for
// ambiguous failures).
func (d *DiagnoseDispatcher) handleDispatchFailure(ctx context.Context, dispatchID, diagnosticSessionUUID string, item *session.BacklogItemData, dispatchErr error) RequestDiagnosisResult {
	reason := fmt.Sprintf("session dispatch unreachable: %v", dispatchErr)
	outcome := diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindDispatchFailed, FailureReason: &reason}

	log.Warn("[DiagnoseDispatcher] dispatch failed", "item", item.ID, "dispatch", dispatchID, "error", dispatchErr)
	d.notifyDiagnoseEvent(ctx, dispatchID, item.ID, item.Title, outcome)

	return RequestDiagnosisResult{
		DispatchID:            dispatchID,
		DiagnosticSessionUUID: diagnosticSessionUUID,
		DispatchFailed:        true,
		FailureReason:         reason,
	}
}

// ambiguousWriteMarker is the literal write_outcome_unknown marker text
// (server/mcp/tools_terminal.go's writeOutcomeUnknownMarker) that
// deriveWriteAttempted looks for in an outcome's self-reported text -- see
// that function's doc comment for why this substring match, rather than a
// precise tool-call inspection, is what Task 5.2.2d settles for.
const ambiguousWriteMarker = "write_outcome_unknown"

// notifyDiagnoseEvent implements Story 5.2.2: unconditionally calls
// DiagnoseDispatchStore.MarkCompleted (updating the Pending row Task 5.1.1d
// already created to Completed -- never inserting a fresh row) BEFORE the
// live-toast publish, so durability survives even if the live publish fails
// or the notifier is unwired -- mirroring notifyReworkCapHit's exact
// two-part shape (backlog_service_triage.go).
//
// Callable from every outcome branch in DiagnoseDispatcher (today, just
// handleDispatchFailure's DispatchFailed branch -- the Nudged/BugFiled/
// InconclusiveNoteFiled/SkippedSafetyGate outcomes are produced by the
// diagnostic agent's own create_backlog_item/post_backlog_update/nudge tool
// calls, handled elsewhere via DiagnoseDispatchStore.FindByDiagnosticSessionUUID;
// those call sites should call this same function).
func (d *DiagnoseDispatcher) notifyDiagnoseEvent(ctx context.Context, dispatchID, itemID, itemTitle string, outcome diagnose.DiagnoseOutcome) {
	if outcome.WriteAttempted == nil {
		outcome.WriteAttempted = deriveWriteAttempted(outcome)
	}

	if err := d.dispatchStore.MarkCompleted(ctx, dispatchID, outcome); err != nil {
		log.Error("[DiagnoseDispatcher] failed to durably record dispatch outcome", "dispatch", dispatchID, "item", itemID, "error", err)
	}

	if d.notifier == nil {
		return
	}
	title, message, notificationType, urgent, important := DiagnoseEventNotification(itemTitle, outcome)
	d.notifier.Notify(itemID, title, message, notificationType, urgent, important)
}

// deriveWriteAttempted implements Task 5.2.2d's WriteAttempted signal.
//
// Judgment call: DiagnoseDispatchStore's only write-tracking primitive
// (CheckAndSetWriteAttempted, backing Story 4.1.4g's duplicate-write MCP
// gate in server/mcp/diagnose_gate_wiring.go) is a check-and-SET guard, not a
// read-only query -- calling it here to "check" would itself mutate dispatch
// state, and even read-only it only ever records "some write was attempted,"
// never whether that write's specific result was the ambiguous
// write_outcome_unknown case this task asks about. The precise
// classification (classifyWriteResult) only ever exists inside
// server/mcp/tools_terminal.go's per-tool-call handling, in a separate
// request lifecycle from whatever eventually calls notifyDiagnoseEvent --
// there is no session-wide tool-call transcript exposed to server/services
// today to inspect after the fact. Given that, the diagnostic agent's own
// outcome text is the only realistically inspectable server-side signal: the
// dispatch prompt (session/diagnose/prompt.go's writeOutcomeUnknownInstruction)
// instructs the agent to reference the ambiguous write when it files its
// post_backlog_update note, so a literal "write_outcome_unknown" substring in
// the outcome's NoteText/FailureReason is treated as the agent's own
// self-report. This is a heuristic, not a guarantee -- flagged here rather
// than presented as precise tool-call telemetry.
func deriveWriteAttempted(outcome diagnose.DiagnoseOutcome) *bool {
	if outcome.NoteText != nil && strings.Contains(*outcome.NoteText, ambiguousWriteMarker) {
		attempted := true
		return &attempted
	}
	if outcome.FailureReason != nil && strings.Contains(*outcome.FailureReason, ambiguousWriteMarker) {
		attempted := true
		return &attempted
	}
	return nil
}

// resolveTargetSessionUUID picks the item's currently active (not-yet-ended)
// session to diagnose -- itemSessions is oldest-first (Storage.ListItemSessions's
// order), so the last not-yet-ended entry is the most recently started active
// one. Falls back to the most recent session overall (still useful evidence,
// even already ended) when none is active, and "" when the item has no
// sessions yet.
func resolveTargetSessionUUID(itemSessions []session.ItemSessionSummary) string {
	for i := len(itemSessions) - 1; i >= 0; i-- {
		if itemSessions[i].EndedAt == nil {
			return itemSessions[i].SessionUUID
		}
	}
	if len(itemSessions) > 0 {
		return itemSessions[len(itemSessions)-1].SessionUUID
	}
	return ""
}

// assembleBundle adapts real session/domain values (item, itemSessions, and
// -- when found -- the target session's live *session.Instance) into
// session/diagnose's local DTOs and drives AssembleDiagnosticBundle. This is
// the adapter boundary session/diagnose/bundle.go's doc comment describes:
// package diagnose can't import package session, so this package (which can
// import both) does the conversion.
func (d *DiagnoseDispatcher) assembleBundle(ctx context.Context, item *session.BacklogItemData, itemSessions []session.ItemSessionSummary, targetSessionUUID string) diagnose.DiagnosticBundle {
	input := diagnose.AssembleDiagnosticBundleInput{
		ItemID: item.ID,
		Item: diagnose.BundleItem{
			ID:                 item.ID,
			Description:        item.Description,
			AcceptanceCriteria: item.AcceptanceCriteria,
		},
		History:             adaptSessionHistory(itemSessions),
		ReviewVerdictSource: diagnosticDataSourceVerdictAdapter{d.dataSource},
		LogWindow:           diagnosticLogWindow(itemSessions, targetSessionUUID),
	}

	if targetSessionUUID != "" && d.instances != nil {
		if inst, found := d.instances.FindInstance(targetSessionUUID); found {
			input.Instance = instanceSnapshotAdapter{inst}
			input.WorktreePath = inst.Workspace().ActiveDir
		}
	}

	return diagnose.AssembleDiagnosticBundle(ctx, d.assembler, input)
}

// diagnosticLogWindow bounds the Logs bundle section to roughly the target
// session's own lifetime: from its StartedAt/CreatedAt to its EndedAt (or
// "still running" when nil), so unrelated sessions' log lines don't crowd out
// the ones actually relevant to this diagnosis. Returns a zero (unbounded)
// LogWindow when targetSessionUUID isn't found in itemSessions.
func diagnosticLogWindow(itemSessions []session.ItemSessionSummary, targetSessionUUID string) diagnose.LogWindow {
	for _, s := range itemSessions {
		if s.SessionUUID != targetSessionUUID {
			continue
		}
		window := diagnose.LogWindow{}
		if s.StartedAt != nil {
			window.Start = *s.StartedAt
		} else {
			window.Start = s.CreatedAt
		}
		if s.EndedAt != nil {
			window.End = *s.EndedAt
		}
		return window
	}
	return diagnose.LogWindow{}
}

// adaptSessionHistory converts real session.ItemSessionSummary rows into
// diagnose.SessionHistoryEntry, rendering each summary line in the same shape
// BuildTokenBudgetedPrompt's "Prior Attempts" section already uses
// (session/backlog_context.go's buildSessionInitialPrompt) -- not a
// byte-for-byte port (that renderer also emits per-criterion verdict detail
// for the most recent few sessions, which the diagnostic agent gets in full
// via the separate PriorVerdicts section instead).
func adaptSessionHistory(itemSessions []session.ItemSessionSummary) []diagnose.SessionHistoryEntry {
	entries := make([]diagnose.SessionHistoryEntry, 0, len(itemSessions))
	for _, s := range itemSessions {
		var sb strings.Builder
		fmt.Fprintf(&sb, "- Role: %s | Commits: %d", s.Role, s.CommitCountSinceSpawn)
		if s.LastCommitMessage != "" {
			fmt.Fprintf(&sb, " | Last commit: %s", s.LastCommitMessage)
		}
		if s.ReviewVerdict != nil {
			fmt.Fprintf(&sb, " | Verdict: %s", s.ReviewVerdict.OverallOutcome)
		}
		if s.EndedAt == nil {
			sb.WriteString(" | (still running)")
		}
		entries = append(entries, diagnose.SessionHistoryEntry{SessionUUID: s.SessionUUID, Summary: sb.String()})
	}
	return entries
}

// The diagnosticDataSourceVerdictAdapter/instanceSnapshotAdapter/
// handoffSummaryGeneratorAdapter types satisfying session/diagnose's local
// DTOs from real session package types live in
// diagnose_dispatcher_adapters.go, split out purely to keep this
// orchestration file's own length manageable.
