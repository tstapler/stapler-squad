package session

// transition_callers.go is the closed caller registry (ADR-007 D2): every code
// path that writes a backlog item's status is one entry here, with its class,
// legacy policy, guard policy and the edges it can request. The system-required
// edge set, the class-aware selector and the gate are derived from this table,
// so they cannot drift from the call sites. server/services'
// workflow_bypass_inventory_test.go cross-checks Sites against a go/parser walk.

import (
	"sort"

	"github.com/tstapler/stapler-squad/session/domain"
)

// TransitionCaller is the registered name of a status-write caller; it is also
// the {caller} label of workflow_engine_bypass_total.
type TransitionCaller string

// LegacyPolicy is what the path does today, which Off and Shadow reproduce.
type LegacyPolicy string

const (
	// LegacyEngine asks the Default workflow engine before writing.
	LegacyEngine LegacyPolicy = "engine"
	// LegacyUnchecked writes straight to storage, so Off and Shadow allow.
	LegacyUnchecked LegacyPolicy = "unchecked"
)

// GuardPolicy says whether the Default built-in TransitionGuard applies.
type GuardPolicy string

const (
	GuardsBuiltIn GuardPolicy = "built_in"
	GuardsNone    GuardPolicy = "none"
)

// AnyStatus as an edge's From is the wildcard used by archive callers.
const AnyStatus BacklogStatus = "*"

// CallSite is a (file, function) pair holding Calls status-primitive calls.
type CallSite struct {
	File  string
	Func  string
	Calls int
}

// TransitionCallerSpec is one registry entry.
type TransitionCallerSpec struct {
	Name   TransitionCaller
	Class  CallerClass
	Legacy LegacyPolicy
	Guards GuardPolicy
	// Edges are the (from,to) pairs the caller can request, derived from the code.
	// From may be AnyStatus. Empty only for a Forwarding entry.
	Edges []SystemEdge
	// Forwarding marks a generic forwarder of an edge somebody else chose. It
	// declares no edges and contributes none to SystemRequiredEdges (ADR-007 D4).
	Forwarding bool
	// Sites are the status-primitive call sites this entry owns. The sum of
	// Calls over all entries, ExemptTransitionSites and SharedGuardHelperSites
	// equals the inventory's count for each (file, func).
	Sites []CallSite
	// Origins are functions that reach the primitive through a shared helper
	// (transitionWithGuard), so they hold no primitive call themselves.
	Origins []CallSite
}

// ExemptKind classifies a status write the engine deliberately never checks.
type ExemptKind string

const (
	ExemptRollback ExemptKind = "rollback"
	ExemptDebug    ExemptKind = "debug"
)

// ExemptSite is an exempt status write (ADR-007 D1): counted, never engine-checked.
type ExemptSite struct {
	CallSite
	Kind   ExemptKind
	Reason string
}

const (
	fileLifecycle    = "server/services/backlog_service_lifecycle.go"
	fileNotifyAction = "server/services/notification_action_backlog.go"
	fileTriage       = "server/services/backlog_service_triage.go"
	fileTriggerTri   = "server/services/backlog_service_trigger_triage.go"
	fileSync         = "server/services/backlog_service_sync.go"
	fileAutonomous   = "server/services/autonomous_orchestration_service.go"
	fileDebugMutate  = "server/services/backlog_debug_mutate_handler.go"
	fileJulesDisp    = "server/services/jules_dispatch_service.go"
	fileMCPBacklog   = "server/mcp/tools_backlog.go"
	fileMCPBacklogPR = "server/mcp/tools_backlog_pr.go"
	fileMCPDelegate  = "server/mcp/tools_backlog_dependencies.go"
	fileRemediation  = "session/backlog_remediation.go"
	fileSessLife     = "session/backlog_lifecycle.go"
	fileSessLifeArch = "session/backlog_lifecycle_archive.go"
	fileSessLifePR   = "session/backlog_lifecycle_pr.go"
	fileSessSync     = "session/backlog_sync.go"
	fileJulesPoller  = "session/jules_session_poller.go"
)

// Registered caller names. Typed constants keep the gate's callers and the
// silenttransition operator-name set (Task 1.5.4f) checkable against the table.
const (
	CallerOperatorTransitionRPC   TransitionCaller = "operator_transition_rpc"
	CallerOperatorOverrideVerdict TransitionCaller = "operator_override_verdict"
	CallerOperatorManualReview    TransitionCaller = "operator_submit_manual_review"
	CallerOperatorArchiveRPC      TransitionCaller = "operator_archive_rpc"
	CallerOperatorUnarchiveRPC    TransitionCaller = "operator_unarchive_rpc"
	CallerOperatorPRAssociation   TransitionCaller = "operator_update_pr_association"
	CallerOperatorTriggerTriage   TransitionCaller = "operator_trigger_triage"
	CallerOperatorForceReset      TransitionCaller = "operator_force_reset"
	// CallerOperatorNotificationAction is an operator's inline notification
	// click (accept or rework), which reaches the transition handler with this
	// caller instead of operator_transition_rpc.
	CallerOperatorNotificationAction TransitionCaller = "operator_notification_action"

	CallerMCPRequestReview   TransitionCaller = "mcp_request_review"
	CallerMCPReportBlocked   TransitionCaller = "mcp_report_blocked"
	CallerMCPResumeWork      TransitionCaller = "mcp_resume_work"
	CallerMCPReportDuplicate TransitionCaller = "mcp_report_duplicate"
	CallerMCPReportPRCreated TransitionCaller = "mcp_report_pr_created"
	// CallerMCPTransitionDelegate is the generic MCP transition tool; the only
	// forwarding entry.
	CallerMCPTransitionDelegate TransitionCaller = "mcp_transition_delegate"

	CallerQueueBacklogItem        TransitionCaller = "queue_backlog_item"
	CallerClaimAndSpawn           TransitionCaller = "claim_and_spawn"
	CallerJulesDispatchStart      TransitionCaller = "jules_dispatch_start"
	CallerAttachSessionToItem     TransitionCaller = "attach_session_to_item"
	CallerSyncLoopBackward        TransitionCaller = "sync_loop_backward"
	CallerAutonomousComplete      TransitionCaller = "autonomous_complete"
	CallerAutoReopenFailedReview  TransitionCaller = "auto_reopen_after_failed_review"
	CallerAutoReopenForPRFix      TransitionCaller = "auto_reopen_for_pr_fix"
	CallerSpawnSessionAfterGates  TransitionCaller = "spawn_session_after_gates"
	CallerRespawnTriage           TransitionCaller = "respawn_triage"
	CallerMaybeTriggerTriage      TransitionCaller = "maybe_trigger_triage"
	CallerTriggerReReview         TransitionCaller = "trigger_re_review"
	CallerTriageComplete          TransitionCaller = "triage_complete"
	CallerSessionExited           TransitionCaller = "lifecycle_session_exited"
	CallerBouncingToDone          TransitionCaller = "lifecycle_bouncing_to_done"
	CallerReconcileStuck          TransitionCaller = "lifecycle_reconcile_stuck"
	CallerArchiveStaleDone        TransitionCaller = "lifecycle_archive_stale_done"
	CallerCloseSupersededPR       TransitionCaller = "lifecycle_close_superseded_pr"
	CallerPushCreatePRFallback    TransitionCaller = "lifecycle_push_create_pr_fallback"
	CallerOrphanedAgentPR         TransitionCaller = "lifecycle_orphaned_agent_pr"
	CallerReconcilePRPendingMerge TransitionCaller = "lifecycle_reconcile_pr_pending"
	CallerRecoverDriftedPR        TransitionCaller = "lifecycle_recover_drifted_pr"
	CallerResolveToPRPending      TransitionCaller = "lifecycle_resolve_to_pr_pending"
	CallerJulesFailSession        TransitionCaller = "jules_fail_session"
	CallerJulesApplyCompleted     TransitionCaller = "jules_apply_completed"
)

func edge(from, to BacklogStatus) SystemEdge { return SystemEdge{From: from, To: to} }

func site(file, fn string, calls int) []CallSite {
	return []CallSite{{File: file, Func: fn, Calls: calls}}
}

// everyGraphEdge lists every edge of the authoritative table. The handler-style
// operator callers choose their edge at request time, so the graph governs them.
func everyGraphEdge() []SystemEdge {
	var out []SystemEdge
	for from, tos := range domain.ValidTransitions() {
		for to := range tos {
			out = append(out, edge(from, to))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

func specOp(name TransitionCaller, legacy LegacyPolicy, edges []SystemEdge, sites []CallSite) TransitionCallerSpec {
	return TransitionCallerSpec{Name: name, Class: ClassOperator, Legacy: legacy, Guards: GuardsBuiltIn, Edges: edges, Sites: sites}
}

func specAgent(name TransitionCaller, edges []SystemEdge, sites []CallSite) TransitionCallerSpec {
	return TransitionCallerSpec{Name: name, Class: ClassAgent, Legacy: LegacyUnchecked, Guards: GuardsBuiltIn, Edges: edges, Sites: sites}
}

func specSys(name TransitionCaller, legacy LegacyPolicy, guards GuardPolicy, edges []SystemEdge, sites []CallSite) TransitionCallerSpec {
	return TransitionCallerSpec{Name: name, Class: ClassSystem, Legacy: legacy, Guards: guards, Edges: edges, Sites: sites}
}

func specUnchecked(name TransitionCaller, edges []SystemEdge, sites []CallSite) TransitionCallerSpec {
	return specSys(name, LegacyUnchecked, GuardsNone, edges, sites)
}

// specViaHelper is a system caller that reaches the primitive through
// transitionWithGuard, so it owns an Origin rather than a primitive call site.
func specViaHelper(name TransitionCaller, edges []SystemEdge, file, fn string) TransitionCallerSpec {
	s := specSys(name, LegacyEngine, GuardsBuiltIn, edges, nil)
	s.Origins = site(file, fn, 0)
	return s
}

// withOrigin adds a function that pre-checks the caller's edge through
// GuardedTransitionAllowed without holding the primitive call itself.
func withOrigin(s TransitionCallerSpec, file, fn string) TransitionCallerSpec {
	s.Origins = append(s.Origins, CallSite{File: file, Func: fn})
	return s
}

const (
	stIdea, stRefining, stReady, stQueued = BacklogStatusIdea, BacklogStatusRefining, BacklogStatusReady, BacklogStatusQueued
	stInProg, stReview, stPrPend, stDone  = BacklogStatusInProgress, BacklogStatusReview, BacklogStatusPRPending, BacklogStatusDone
	stArchived                            = BacklogStatusArchived
)

var archiveFromAny = []SystemEdge{edge(AnyStatus, stArchived)}

var transitionCallers = []TransitionCallerSpec{
	// Operator.
	specOp(CallerOperatorTransitionRPC, LegacyEngine, everyGraphEdge(), site(fileLifecycle, "BacklogService.TransitionBacklogItemStatus", 1)),
	specOp(CallerOperatorOverrideVerdict, LegacyEngine, everyGraphEdge(), site(fileLifecycle, "BacklogService.OverrideVerdict", 1)),
	specOp(CallerOperatorManualReview, LegacyUnchecked, []SystemEdge{edge(stReview, stDone)}, site(fileLifecycle, "BacklogService.SubmitManualReview", 1)),
	specOp(CallerOperatorArchiveRPC, LegacyUnchecked, archiveFromAny, site(fileLifecycle, "BacklogService.ArchiveBacklogItem", 1)),
	specOp(CallerOperatorUnarchiveRPC, LegacyUnchecked, []SystemEdge{edge(stArchived, stIdea)}, site(fileLifecycle, "BacklogService.UnarchiveBacklogItem", 1)),
	specOp(CallerOperatorPRAssociation, LegacyUnchecked, []SystemEdge{edge(stReview, stPrPend)}, site(fileLifecycle, "BacklogService.UpdateBacklogItem", 1)),
	specOp(CallerOperatorTriggerTriage, LegacyUnchecked, []SystemEdge{edge(stReady, stIdea)}, site(fileTriggerTri, "BacklogService.TriggerTriage", 1)),
	specOp(CallerOperatorForceReset, LegacyUnchecked, []SystemEdge{edge(stReview, stInProg)}, site(fileTriage, "BacklogService.forceResetItem", 1)),
	specOp(CallerOperatorNotificationAction, LegacyEngine, []SystemEdge{edge(stReview, stDone), edge(stReview, stReady), edge(stPrPend, stReady)}, site(fileNotifyAction, "backlogDecider.transition", 1)),

	// Agent. request_review's skip-stReview-gate path writes ->stDone from
	// in_progress, an edge validTransitions lacks (system-required recovery group).
	specAgent(CallerMCPRequestReview, []SystemEdge{
		edge(stInProg, stReview), edge(stPrPend, stReview), edge(stInProg, stDone), edge(stPrPend, stDone), edge(stReview, stInProg),
	}, site(fileMCPBacklog, "backlogHandlers.requestReview", 2)),
	specAgent(CallerMCPReportBlocked, []SystemEdge{edge(stInProg, stReady), edge(stInProg, stReview), edge(stReview, stReady)}, site(fileMCPBacklog, "backlogHandlers.reportBlocked", 1)),
	specAgent(CallerMCPResumeWork, []SystemEdge{edge(stReady, stInProg)}, site(fileMCPBacklog, "backlogHandlers.resumeWork", 1)),
	specAgent(CallerMCPReportDuplicate, archiveFromAny, site(fileMCPBacklog, "backlogHandlers.reportDuplicate", 1)),
	specAgent(CallerMCPReportPRCreated, []SystemEdge{edge(stReview, stPrPend)}, site(fileMCPBacklogPR, "backlogHandlers.reportPRCreated", 1)),
	{Name: CallerMCPTransitionDelegate, Class: ClassAgent, Legacy: LegacyEngine, Guards: GuardsBuiltIn, Forwarding: true,
		Sites: site(fileMCPDelegate, "backlogHandlers.transitionBacklogItemStatus", 1)},

	// System: engine-checked today.
	specViaHelper(CallerQueueBacklogItem, []SystemEdge{edge(stReady, stQueued)}, fileTriage, "BacklogService.queueBacklogItem"),
	specViaHelper(CallerClaimAndSpawn, []SystemEdge{edge(stQueued, stInProg), edge(stReady, stInProg)}, fileTriage, "BacklogService.claimAndSpawnCandidate"),
	specViaHelper(CallerJulesDispatchStart, []SystemEdge{edge(stReady, stInProg)}, fileJulesDisp, "JulesDispatchService.DispatchToJules"),
	specSys(CallerAttachSessionToItem, LegacyEngine, GuardsBuiltIn,
		[]SystemEdge{edge(stReady, stInProg), edge(stQueued, stInProg), edge(stReview, stInProg), edge(stPrPend, stInProg), edge(stDone, stInProg)},
		site(fileSync, "BacklogService.AttachSessionToItem", 1)),
	withOrigin(specSys(CallerSyncLoopBackward, LegacyEngine, GuardsBuiltIn,
		[]SystemEdge{edge(stIdea, stArchived), edge(stRefining, stArchived), edge(stReady, stArchived), edge(stQueued, stArchived)},
		site(fileSessSync, "SyncLoop.transitionBackward", 1)), fileSessSync, "SyncLoop.SyncOne"),

	// System: storage-direct today. toStatus is assigned once (stIdea->stReady).
	specUnchecked(CallerAutonomousComplete, []SystemEdge{edge(stIdea, stReady)}, site(fileAutonomous, "AutonomousOrchestrationService.advanceItemOnDriverComplete", 1)),
	specUnchecked(CallerAutoReopenFailedReview, []SystemEdge{edge(stReview, stInProg)}, site(fileTriage, "BacklogService.AutoReopenAfterFailedReview", 1)),
	specUnchecked(CallerAutoReopenForPRFix, []SystemEdge{edge(stPrPend, stInProg)}, site(fileTriage, "BacklogService.autoReopenForPRFix", 1)),
	specUnchecked(CallerSpawnSessionAfterGates, []SystemEdge{edge(stReady, stInProg)}, site(fileTriage, "BacklogService.spawnSessionAfterGates", 1)),
	specUnchecked(CallerRespawnTriage, []SystemEdge{edge(stQueued, stIdea)}, site(fileTriage, "BacklogService.respawnTriage", 1)),
	specUnchecked(CallerMaybeTriggerTriage, []SystemEdge{edge(stIdea, stReady)}, site(fileTriage, "BacklogService.skipTriageForNewItem", 1)),
	specUnchecked(CallerTriggerReReview, []SystemEdge{edge(stReview, stDone)}, site(fileTriage, "BacklogService.TriggerReReview", 1)),
	specUnchecked(CallerTriageComplete, []SystemEdge{edge(stIdea, stReady)}, site(fileTriggerTri, "BacklogService.TriggerTriage", 1)),
	specUnchecked(CallerSessionExited, []SystemEdge{edge(stInProg, stReview), edge(stInProg, stDone)}, site(fileSessLife, "BacklogLifecycleListener.onSessionExited", 1)),
	specUnchecked(CallerBouncingToDone, []SystemEdge{edge(stInProg, stReview), edge(stReview, stDone)}, site(fileSessLife, "BacklogLifecycleListener.transitionBouncingItemToDone", 2)),
	specUnchecked(CallerReconcileStuck, []SystemEdge{edge(stInProg, stReview)}, site(fileSessLife, "BacklogLifecycleListener.ReconcileStuck", 1)),
	specUnchecked(CallerArchiveStaleDone, []SystemEdge{edge(stDone, stArchived)}, site(fileSessLifeArch, "BacklogLifecycleListener.archiveStaleDoneItems", 1)),
	specUnchecked(CallerCloseSupersededPR, []SystemEdge{edge(stPrPend, stDone)}, site(fileSessLifePR, "BacklogLifecycleListener.closeIfSupersededByMain", 1)),
	specUnchecked(CallerPushCreatePRFallback, []SystemEdge{edge(stReview, stDone), edge(stReady, stDone)}, site(fileSessLifePR, "BacklogLifecycleListener.pushAndCreatePR", 1)),
	specUnchecked(CallerOrphanedAgentPR, []SystemEdge{edge(stReview, stPrPend)}, site(fileSessLifePR, "BacklogLifecycleListener.recordOrphanedAgentPR", 1)),
	specUnchecked(CallerReconcilePRPendingMerge, []SystemEdge{edge(stPrPend, stDone)}, site(fileSessLifePR, "BacklogLifecycleListener.reconcilePRPendingItem", 1)),
	// FindDriftedPRItems selects every status except pr_pending, stDone and stArchived.
	specUnchecked(CallerRecoverDriftedPR, []SystemEdge{
		edge(stIdea, stPrPend), edge(stRefining, stPrPend), edge(stReady, stPrPend), edge(stQueued, stPrPend), edge(stInProg, stPrPend), edge(stReview, stPrPend),
	}, site(fileSessLifePR, "BacklogLifecycleListener.recoverDriftedPRItem", 1)),
	specUnchecked(CallerResolveToPRPending, []SystemEdge{edge(stReview, stPrPend)}, site(fileSessLifePR, "BacklogLifecycleListener.resolveToPRPending", 1)),
	specUnchecked(CallerJulesFailSession, []SystemEdge{edge(stInProg, stReady)}, site(fileJulesPoller, "JulesSessionPoller.failJulesSession", 1)),
	specUnchecked(CallerJulesApplyCompleted, []SystemEdge{edge(stInProg, stReview), edge(stReview, stPrPend)}, site(fileJulesPoller, "JulesSessionPoller.applyCompletedState", 3)),
}

// exemptTransitionSites are the 3 rollbacks and 2 debug writes (ADR-007 D1).
var exemptTransitionSites = []ExemptSite{
	{CallSite{fileTriage, "BacklogService.rollbackFailedReopen", 1}, ExemptRollback, "restores review after a failed respawn"},
	{CallSite{fileTriage, "BacklogService.rollbackFailedPRFixReopen", 1}, ExemptRollback, "restores pr_pending after a failed respawn"},
	{CallSite{fileTriage, "BacklogService.claimAndSpawnCandidate", 1}, ExemptRollback, "restores the pre-claim status after a failed spawn"},
	{CallSite{fileDebugMutate, "BacklogDebugMutateHandler.handleTransition", 1}, ExemptDebug, "operator-only debug endpoint"},
	{CallSite{fileDebugMutate, "BacklogDebugMutateHandler.handleArchive", 1}, ExemptDebug, "operator-only debug endpoint"},
}

// sharedGuardHelperSites hold the one primitive call that several registered
// callers reach: transitionWithGuard, and the re-drive of a recorded denial,
// which re-attempts the transition under the denied caller's own name (only
// the redriveByWrite callers).
var sharedGuardHelperSites = []CallSite{
	{fileTriage, "BacklogService.transitionWithGuard", 1},
	{fileRemediation, "Storage.redriveOne", 1},
}

// knownOffGraphCallerEdges are caller bugs the audit tolerates until the named
// migration story fixes the caller (ADR-007 D7: a caller bug is fixed in the
// caller, not by growing the system-required set). It is empty since Story
// 1.5.5 fixed report_blocked's review->review.
var knownOffGraphCallerEdges []KnownOffGraphEdge

// KnownOffGraphEdge is a registered edge deliberately left out of the graph.
type KnownOffGraphEdge struct {
	Caller TransitionCaller
	Edge   SystemEdge
	Reason string
}

// TransitionCallers returns a copy of the registry in declaration order.
func TransitionCallers() []TransitionCallerSpec {
	out := make([]TransitionCallerSpec, len(transitionCallers))
	copy(out, transitionCallers)
	return out
}

// LookupTransitionCaller returns the entry registered under name.
func LookupTransitionCaller(name TransitionCaller) (TransitionCallerSpec, bool) {
	for _, c := range transitionCallers {
		if c.Name == name {
			return c, true
		}
	}
	return TransitionCallerSpec{}, false
}

// ExemptTransitionSites returns the rollback and debug sites.
func ExemptTransitionSites() []ExemptSite { return append([]ExemptSite(nil), exemptTransitionSites...) }

// SharedGuardHelperSites returns the primitive call sites shared by several Origins.
func SharedGuardHelperSites() []CallSite { return append([]CallSite(nil), sharedGuardHelperSites...) }

// KnownOffGraphCallerEdges returns the tolerated caller bugs.
func KnownOffGraphCallerEdges() []KnownOffGraphEdge {
	return append([]KnownOffGraphEdge(nil), knownOffGraphCallerEdges...)
}
