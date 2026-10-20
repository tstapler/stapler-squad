package services

// workflow_bypass_inventory_test.go is the repo-wide inventory of every call
// to a backlog status-write primitive (ADR-007 D9, Story 1.2.3b). It walks the
// whole module with go/parser, so a new raw call anywhere (server/, session/,
// server/mcp/, any other package) fails here unless it is classified.
//
// The pending class is a ratchet: it can only shrink. Epic 1.5 migrates a site,
// then deletes its entry; a stale entry (no matching call, or a different call
// count) fails. Story 1.5.7 migrated the last pending site, so no entry uses the
// class now; Story 1.5.9a replaces the scan.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// bypassPrimitives are the six status writers (ADR-007 D1); a call is matched
// by selector name on any receiver. The Raw/raw forms are the post-1.5.4 names;
// "TransitionBacklogItemStatusWithNote" is the raw writer behind GatedReconcileStuck.
// The Gated* entry points are counted too: a migrated site stays a registered
// site (engine-routed through the gate) until Story 1.5.9a replaces this scan.
var bypassPrimitives = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{
		"TransitionBacklogItemStatus", "ArchiveBacklogItem", "UnarchiveBacklogItem",
		"SetBacklogItemPRAndTransition", "ReconcileStuckItems", "TransitionBacklogItemStatusWithPRFields",
		"TransitionBacklogItemStatusWithNote",
		// ReconcileStuckItems has no Storage wrapper; GatedReconcileStuck is its only
		// form and stays counted as the gated lifecycle_reconcile_stuck site.
		"GatedReconcileStuck",
		"GatedTransition", "GatedArchive", "GatedUnarchive", "GatedSetPRAndTransition",
	} {
		m[n] = true
		m["Raw"+n] = true
		m["raw"+strings.ToUpper(n[:1])+n[1:]] = true
	}
	return m
}()

// gateEntryPoints are the Storage methods a migrated site calls instead of a
// raw writer, mapped to the minimum argument count that tells them apart from
// same-named methods elsewhere (tx.Rollback() takes none).
var gateEntryPoints = map[string]int{
	"GatedTransition": 0, "GatedArchive": 0, "GatedUnarchive": 0, "GatedSetPRAndTransition": 0,
	"Rollback": 4, "DebugForce": 0,
}

func isStatusWriteCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if bypassPrimitives[sel.Sel.Name] {
		return true
	}
	minArgs, gated := gateEntryPoints[sel.Sel.Name]
	return gated && len(call.Args) >= minArgs
}

type bypassClass string

const (
	classEngineRouted bypassClass = "engine-routed"
	classPending      bypassClass = "pending"
	// classGated sites call a Storage gate entry point (GatedTransition,
	// Rollback, DebugForce, ...) under a registered caller.
	classGated  bypassClass = "gated"
	classExempt bypassClass = "exempt"
)

// bypassSite is one (file, function) holding `calls` primitive calls.
type bypassSite struct {
	file, fn string
	calls    int
	class    bypassClass
	reason   string // pending: "migrate in Story 1.5.x"; exempt: why; engine-routed: how
}

func (s bypassSite) key() string { return s.file + " " + s.fn }

const (
	wrapperReason  = "Storage wrapper over the repo primitive; renamed Raw* in Task 1.5.4a"
	gateReason     = "Storage.GatedTransition choke point (ADR-007 D1)"
	delegateReason = "MCP delegate forwards to the handler, which routes through the engine (ADR-007 D2 forwarding)"
)

func gated(file, fn string, calls int, how string) bypassSite {
	return bypassSite{file: file, fn: fn, calls: calls, class: classGated, reason: how}
}

func gateRouted(file, fn string, calls int, how string) bypassSite {
	return bypassSite{file: file, fn: fn, calls: calls, class: classEngineRouted, reason: how}
}

func exempt(file, fn string, calls int, why string) bypassSite {
	return bypassSite{file: file, fn: fn, calls: calls, class: classExempt, reason: why}
}

// bypassInventory is the classification of every call at the time of writing
// (recounted by git grep at the Story 1.2.3 HEAD).
var bypassInventory = []bypassSite{
	// Exempt: wrappers and the MCP delegate.
	exempt("session/storage.go", "Storage.RawTransitionBacklogItemStatus", 1, wrapperReason),
	exempt("session/storage.go", "Storage.RawArchiveBacklogItem", 1, wrapperReason),
	exempt("session/storage.go", "Storage.RawUnarchiveBacklogItem", 1, wrapperReason),
	exempt("session/storage.go", "Storage.setPRAndTransition", 1, "single-UPDATE PR write shared by RawSetBacklogItemPRAndTransition and GatedSetPRAndTransition; its call is rawTransitionBacklogItemStatusWithPRFields"),
	exempt("session/transition_gate.go", "Storage.writeGated", 1, gateReason+": the write after the engine allowed"),
	exempt("session/ent_repository_backlog.go", "EntRepository.rawTransitionBacklogItemStatus", 1, "the raw writer delegating to its note-appending variant; both are unexported and reachable only through the gate and the Raw* wrappers"),
	exempt("session/transition_gate.go", "Storage.writeGatedArchive", 1, gateReason+": the archive write after the engine allowed"),
	exempt("session/transition_gate.go", "Storage.writeGatedUnarchive", 1, gateReason+": the unarchive write after the engine allowed"),
	exempt("session/transition_gate.go", "Storage.writeGatedReconcile", 1, gateReason+": the reconcile write after the engine allowed, appending the [auto] note in the same UPDATE"),
	exempt("session/transition_gate.go", "Storage.Rollback", 1, gateReason+": exempt entry point, counted"),
	exempt("session/transition_gate.go", "Storage.DebugForce", 2, gateReason+": exempt entry point, counted (transition and archive)"),
	exempt("server/mcp/tools_backlog_dependencies.go", "backlogHandlers.transitionBacklogItemStatus", 1, delegateReason),

	// Engine-routed: ask the WorkflowEngine today; re-pointed at the gate in Stories 1.5.5-1.5.7.
	gateRouted("server/services/backlog_service_lifecycle.go", "BacklogService.TransitionBacklogItemStatus", 1, "GatedTransition as operator_transition_rpc or mcp_transition_delegate, by context actor (Story 1.5.5a)"),
	gateRouted("server/services/backlog_service_lifecycle.go", "BacklogService.OverrideVerdict", 1, "GatedTransition as operator_override_verdict (Story 1.5.5a)"),

	// Gate-routed in Story 1.5.5 (operator and agent writers).
	gateRouted("server/services/notification_action_backlog.go", "backlogDecider.transition", 1, "forwards the operator's inline click to the transition handler as operator_notification_action"),
	gateRouted("server/services/backlog_service_lifecycle.go", "BacklogService.SubmitManualReview", 1, "GatedTransition as operator_submit_manual_review (Story 1.5.5a)"),
	gateRouted("server/services/backlog_service_lifecycle.go", "BacklogService.ArchiveBacklogItem", 1, "GatedArchive as operator_archive_rpc (Story 1.5.5b)"),
	gateRouted("server/services/backlog_service_lifecycle.go", "BacklogService.UnarchiveBacklogItem", 1, "GatedUnarchive as operator_unarchive_rpc (Story 1.5.5b)"),
	gateRouted("server/services/backlog_service_lifecycle.go", "BacklogService.UpdateBacklogItem", 1, "GatedSetPRAndTransition as operator_update_pr_association (Story 1.5.5b)"),
	gateRouted("server/mcp/tools_backlog.go", "backlogHandlers.requestReview", 2, "GatedTransition as mcp_request_review, recovery reopen and the transition (Story 1.5.5c)"),
	gateRouted("server/mcp/tools_backlog.go", "backlogHandlers.reportBlocked", 1, "GatedTransition as mcp_report_blocked (Story 1.5.5c)"),
	gateRouted("server/mcp/tools_backlog.go", "backlogHandlers.resumeWork", 1, "GatedTransition as mcp_resume_work (Story 1.5.5c)"),
	gateRouted("server/mcp/tools_backlog.go", "backlogHandlers.reportDuplicate", 1, "GatedArchive as mcp_report_duplicate (Story 1.5.5c)"),
	gateRouted("server/mcp/tools_backlog_pr.go", "backlogHandlers.reportPRCreated", 1, "GatedSetPRAndTransition as mcp_report_pr_created (Story 1.5.5c)"),

	// Gated, Story 1.5.6 (system writers in server/services, the two operator triage writers,
	// the rollbacks and the debug handler).
	gated("server/services/autonomous_orchestration_service.go", "AutonomousOrchestrationService.advanceItemOnDriverComplete", 1, "GatedTransition as autonomous_complete"),
	gated("server/services/backlog_service_triage.go", "BacklogService.transitionWithGuard", 1, "GatedTransition; caller derived from target and actor (queue, claim, Jules dispatch)"),
	gated("server/services/backlog_service_triage.go", "BacklogService.AutoReopenAfterFailedReview", 1, "GatedTransition as auto_reopen_after_failed_review"),
	gated("server/services/backlog_service_triage.go", "BacklogService.rollbackFailedReopen", 1, "gate.Rollback (exempt, counted)"),
	gated("server/services/backlog_service_triage.go", "BacklogService.autoReopenForPRFix", 1, "GatedTransition as auto_reopen_for_pr_fix"),
	gated("server/services/backlog_service_triage.go", "BacklogService.rollbackFailedPRFixReopen", 1, "gate.Rollback (exempt, counted)"),
	gated("server/services/backlog_service_triage.go", "BacklogService.claimAndSpawnCandidate", 1, "gate.Rollback (exempt, counted)"),
	gated("server/services/backlog_service_triage.go", "BacklogService.spawnSessionAfterGates", 1, "GatedTransition as spawn_session_after_gates (Unconditional)"),
	gated("server/services/backlog_service_triage.go", "BacklogService.respawnTriage", 1, "GatedTransition as respawn_triage"),
	gated("server/services/backlog_service_triage.go", "BacklogService.skipTriageForNewItem", 1, "GatedTransition as maybe_trigger_triage"),
	gated("server/services/backlog_service_triage.go", "BacklogService.TriggerReReview", 1, "GatedTransition as trigger_re_review"),
	gated("server/services/backlog_service_triage.go", "BacklogService.forceResetItem", 1, "GatedTransition as operator_force_reset (Unconditional)"),
	gated("server/services/backlog_service_trigger_triage.go", "BacklogService.TriggerTriage", 2, "GatedTransition as operator_trigger_triage and triage_complete"),
	gated("server/services/backlog_service_sync.go", "BacklogService.AttachSessionToItem", 1, "GatedTransition as attach_session_to_item (Unconditional)"),
	gated("server/services/backlog_debug_mutate_handler.go", "BacklogDebugMutateHandler.handleTransition", 1, "gate.DebugForce (exempt class debug, counted)"),
	gated("server/services/backlog_debug_mutate_handler.go", "BacklogDebugMutateHandler.handleArchive", 1, "gate.DebugForce (exempt class debug, counted)"),

	// Gated, Story 1.5.7 (system writers in session).
	gated("session/backlog_lifecycle.go", "BacklogLifecycleListener.onSessionExited", 1, "GatedTransition as lifecycle_session_exited"),
	gated("session/backlog_lifecycle.go", "BacklogLifecycleListener.transitionBouncingItemToDone", 2, "GatedTransition as lifecycle_bouncing_to_done"),
	gated("session/backlog_lifecycle.go", "BacklogLifecycleListener.ReconcileStuck", 1, "GatedReconcileStuck as lifecycle_reconcile_stuck"),
	gated("session/backlog_lifecycle_archive.go", "BacklogLifecycleListener.archiveStaleDoneItems", 1, "GatedTransition as lifecycle_archive_stale_done"),
	gated("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.closeIfSupersededByMain", 1, "GatedTransition as lifecycle_close_superseded_pr"),
	gated("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.pushAndCreatePR", 1, "GatedTransition as lifecycle_push_create_pr_fallback"),
	gated("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.recordOrphanedAgentPR", 1, "GatedSetPRAndTransition as lifecycle_orphaned_agent_pr"),
	gated("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.reconcilePRPendingItem", 1, "GatedTransition as lifecycle_reconcile_pr_pending"),
	gated("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.recoverDriftedPRItem", 1, "GatedTransition as lifecycle_recover_drifted_pr"),
	gated("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.resolveToPRPending", 1, "GatedTransition as lifecycle_resolve_to_pr_pending"),
	gated("session/jules_session_poller.go", "JulesSessionPoller.failJulesSession", 1, "GatedTransition as jules_fail_session"),
	gated("session/jules_session_poller.go", "JulesSessionPoller.applyCompletedState", 3, "GatedTransition twice and GatedSetPRAndTransition as jules_apply_completed"),
	gated("session/backlog_sync.go", "SyncLoop.transitionBackward", 1, "GatedTransition as sync_loop_backward"),
}

// walkModuleFuncs parses every non-test Go file under root (skipping web-app,
// tools/lint, gen and dot or vendor directories) and calls visit for each
// function declaration with a body, passing its module-relative file path.
func walkModuleFuncs(t *testing.T, root string, visit func(rel string, fd *ast.FuncDecl)) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch {
			case rel == ".":
				return nil
			case strings.HasPrefix(d.Name(), "."), d.Name() == "node_modules", d.Name() == "vendor",
				rel == "web-app", rel == "tools/lint", rel == "gen":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", rel, perr)
		}
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Body != nil {
				visit(rel, fd)
			}
		}
		return nil
	})
	require.NoError(t, err)
}

// calledName returns the function or selector name of a call, or "".
func calledName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name
	case *ast.Ident:
		return fun.Name
	}
	return ""
}

// scanBypassCalls returns the number of primitive calls per "file function"
// key. Test files and tools/lint are excluded; the analyzers there are not
// status writers.
func scanBypassCalls(t *testing.T, root string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	walkModuleFuncs(t, root, func(rel string, fd *ast.FuncDecl) {
		name := funcDisplayName(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isStatusWriteCall(call) {
				counts[rel+" "+name]++
			}
			return true
		})
	})
	return counts
}

func funcDisplayName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	typ := fd.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if idx, ok := typ.(*ast.IndexExpr); ok {
		typ = idx.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found above test directory")
		dir = parent
	}
}

func TestWorkflowBypassInventory_EveryStatusWriteCallIsClassified(t *testing.T) {
	actual := scanBypassCalls(t, moduleRoot(t))
	want := map[string]bypassSite{}
	for _, s := range bypassInventory {
		_, dup := want[s.key()]
		require.False(t, dup, "duplicate inventory entry %s", s.key())
		want[s.key()] = s
	}

	var problems []string
	for key, n := range actual {
		s, ok := want[key]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("unclassified status-write call(s) in %s (%d): route through the workflow engine (ADR-007) or add an inventory entry with a reason", key, n))
		case s.calls != n:
			problems = append(problems, fmt.Sprintf("%s has %d status-write call(s), inventory says %d (%s): a new call needs a classification, a migrated one needs its entry shrunk or deleted", key, n, s.calls, s.class))
		}
	}
	for key, s := range want {
		if _, ok := actual[key]; !ok {
			problems = append(problems, fmt.Sprintf("stale %s entry %s: no status-write call remains, delete it", s.class, key))
		}
	}
	sort.Strings(problems)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}

func TestWorkflowBypassInventory_EveryEntryHasClassAndReason(t *testing.T) {
	for _, s := range bypassInventory {
		assert.Contains(t, []bypassClass{classEngineRouted, classPending, classExempt, classGated}, s.class, s.key())
		assert.NotEmpty(t, strings.TrimSpace(s.reason), "%s lacks a reason", s.key())
		assert.Positive(t, s.calls, s.key())
		if s.class == classPending {
			assert.True(t, strings.HasPrefix(s.reason, "migrate in Story 1.5."), "%s pending reason must name its migrating story: %q", s.key(), s.reason)
		}
	}
}

func TestWorkflowBypassInventory_ScannerCountsEveryReceiverAndRawForms(t *testing.T) {
	dir := t.TempDir()
	src := `package x
type a struct{}
func (a *a) M(s, r interface{ ArchiveBacklogItem(); RawArchiveBacklogItem(); rawTransitionBacklogItemStatus() }) {
	s.ArchiveBacklogItem()
	r.RawArchiveBacklogItem()
	r.rawTransitionBacklogItemStatus()
	s.Unrelated()
}
func Free(x interface{ ReconcileStuckItems() }) { x.ReconcileStuckItems() }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600))
	got := scanBypassCalls(t, dir)
	assert.Equal(t, map[string]int{"x.go a.M": 3, "x.go Free": 1}, got)
}

func TestWorkflowBypassInventory_ScannerCountsGateEntryPointsButNotTransactionRollback(t *testing.T) {
	dir := t.TempDir()
	src := `package x
func Migrated(s interface{}, tx interface{}) {
	s.GatedTransition(ctx, req)
	s.Rollback(ctx, id, to, pre, by)
	s.DebugForce(ctx, id, to, by)
	defer tx.Rollback()
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600))
	assert.Equal(t, map[string]int{"x.go Migrated": 3}, scanBypassCalls(t, dir))
}

const delegateFile = "server/mcp/tools_backlog_dependencies.go"

// registryCallCounts sums the primitive calls the caller registry accounts for,
// keyed like the inventory ("file function").
func registryCallCounts() map[string]int {
	counts := map[string]int{}
	add := func(cs session.CallSite) { counts[cs.File+" "+cs.Func] += cs.Calls }
	for _, c := range session.TransitionCallers() {
		for _, cs := range c.Sites {
			add(cs)
		}
	}
	for _, e := range session.ExemptTransitionSites() {
		add(e.CallSite)
	}
	for _, cs := range session.SharedGuardHelperSites() {
		add(cs)
	}
	return counts
}

// Story 1.5.1a: every site the inventory still has to migrate (and every
// engine-routed one) maps to registry entries whose call counts add up, and the
// registry names no site the inventory does not know.
func TestWorkflowBypassInventory_EveryPendingAndEngineRoutedSiteIsRegistered(t *testing.T) {
	got := registryCallCounts()
	var problems []string
	covered := map[string]bool{}
	for _, s := range bypassInventory {
		inScope := s.class == classPending || s.class == classEngineRouted || s.class == classGated || s.file == delegateFile
		if !inScope {
			continue
		}
		covered[s.key()] = true
		switch n := got[s.key()]; {
		case n == 0:
			problems = append(problems, fmt.Sprintf("unregistered %s site %s: add a caller entry to session/transition_callers.go", s.class, s.key()))
		case n != s.calls:
			problems = append(problems, fmt.Sprintf("%s: registry accounts for %d call(s), inventory says %d (duplicate or missing registration)", s.key(), n, s.calls))
		}
	}
	for key := range got {
		if !covered[key] {
			problems = append(problems, fmt.Sprintf("registry names %s, which is not a pending or engine-routed inventory site", key))
		}
	}
	sort.Strings(problems)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}

// Closes pre-mortem #13: every (to) literal handed to transitionWithGuard or
// GuardedTransitionAllowed is an edge some registered caller declares, and the
// functions making those calls are registered as an Origin or Site.
func TestWorkflowBypassInventory_GuardedTransitionCallersAreRegisteredWithTheirTargets(t *testing.T) {
	owners := map[string][]session.TransitionCallerSpec{}
	origins := map[string]bool{}
	for _, c := range session.TransitionCallers() {
		for _, cs := range c.Origins {
			owners[cs.File+" "+cs.Func] = append(owners[cs.File+" "+cs.Func], c)
			origins[cs.File+" "+cs.Func] = true
		}
		for _, cs := range c.Sites {
			owners[cs.File+" "+cs.Func] = append(owners[cs.File+" "+cs.Func], c)
		}
	}
	guarded := map[string]bool{"transitionWithGuard": true, "GuardedTransitionAllowed": true}
	found := map[string]bool{}
	var problems []string
	walkModuleFuncs(t, moduleRoot(t), func(rel string, fd *ast.FuncDecl) {
		key := rel + " " + funcDisplayName(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !guarded[calledName(call)] || len(call.Args) < 3 {
				return true
			}
			found[key] = true
			specs := owners[key]
			if len(specs) == 0 {
				problems = append(problems, fmt.Sprintf("%s calls %s but is not a registered origin", key, calledName(call)))
				return true
			}
			if to := statusLiteralName(call.Args[2]); to != "" && !anyDeclaresTarget(specs, to) {
				problems = append(problems, fmt.Sprintf("%s requests %s but no registered caller there declares that target", key, to))
			}
			return true
		})
	})
	for key := range origins {
		if !found[key] {
			problems = append(problems, fmt.Sprintf("stale origin %s: it no longer calls a guarded transition helper", key))
		}
	}
	sort.Strings(problems)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}

var statusConstNames = map[string]session.BacklogStatus{
	"BacklogStatusIdea": session.BacklogStatusIdea, "BacklogStatusRefining": session.BacklogStatusRefining,
	"BacklogStatusReady": session.BacklogStatusReady, "BacklogStatusQueued": session.BacklogStatusQueued,
	"BacklogStatusInProgress": session.BacklogStatusInProgress, "BacklogStatusReview": session.BacklogStatusReview,
	"BacklogStatusPRPending": session.BacklogStatusPRPending, "BacklogStatusDone": session.BacklogStatusDone,
	"BacklogStatusArchived": session.BacklogStatusArchived,
}

// statusLiteralName returns the status value of a BacklogStatus* constant
// expression, or "" for a computed value.
func statusLiteralName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return string(statusConstNames[v.Sel.Name])
	case *ast.Ident:
		return string(statusConstNames[v.Name])
	}
	return ""
}

func anyDeclaresTarget(specs []session.TransitionCallerSpec, to string) bool {
	for _, c := range specs {
		for _, e := range c.Edges {
			if string(e.To) == to {
				return true
			}
		}
	}
	return false
}

// The autonomous_complete registry entry pins the set {ready}: toStatus is
// assigned exactly once in onAutonomousDriverComplete, to BacklogStatusReady.
func TestWorkflowBypassInventory_AutonomousCompleteTargetSetIsExactlyReady(t *testing.T) {
	const file = "server/services/autonomous_orchestration_service.go"
	var assigned []string
	walkModuleFuncs(t, moduleRoot(t), func(rel string, fd *ast.FuncDecl) {
		if rel != file {
			return
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || as.Tok != token.ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			if id, ok := as.Lhs[0].(*ast.Ident); ok && id.Name == "toStatus" {
				assigned = append(assigned, statusLiteralName(as.Rhs[0]))
			}
			return true
		})
	})
	assert.Equal(t, []string{"ready"}, assigned, "a new toStatus assignment needs a registry edge and a new pin")

	c, ok := session.LookupTransitionCaller(session.CallerAutonomousComplete)
	require.True(t, ok)
	assert.Equal(t, []session.SystemEdge{{From: session.BacklogStatusIdea, To: session.BacklogStatusReady}}, c.Edges)
}

func TestRecordEngineBypass_IncrementsCounter(t *testing.T) {
	const c = session.TransitionCaller("inventory_test_caller")
	before := session.EngineBypassCount(c)
	session.RecordEngineBypass(c)
	session.RecordEngineBypass(c)
	assert.Equal(t, before+2, session.EngineBypassCount(c))
	assert.Equal(t, before+2, session.EngineBypassSnapshot()[string(c)])
}

// --- creation (ADR-007 D1, exempt class "create") -----------------------------
//
// Creation has no "from" status, so it is not a transition. The rule is static
// (no runtime rejection in Storage.CreateBacklogItem: about 300 test fixtures
// create items at non-entry statuses): every non-test CreateBacklogItem call
// sets no Status or BacklogStatusIdea, or is a CreateBacklogItemForDebug call
// listed here with a reason. A new creator that sets any other status fails
// naming its file and function.

type createClass string

const (
	// classCreate callers must create at idea or with no status; the test checks it.
	classCreate createClass = "create"
	// classCreateDebug callers use CreateBacklogItemForDebug (any status, counted).
	classCreateDebug createClass = "exempt: debug"
	// classCreateForwarder functions pass a caller-supplied item through; each of
	// their own callers is classified separately.
	classCreateForwarder createClass = "exempt: forwarder"
)

type createSite struct {
	file, fn string
	calls    int
	class    createClass
	reason   string
}

func (s createSite) key() string { return s.file + " " + s.fn }

const (
	debugCreateReason = "operator-only debug endpoint; Storage.CreateBacklogItemForDebug counts it as debug_create"
	idleCreateReason  = "creates at idea (BacklogStatusIdea) or sets no status"
)

var createInventory = []createSite{
	{"session/storage.go", "Storage.CreateBacklogItem", 1, classCreateForwarder, "forwards the caller's item to the repository"},
	{"session/storage.go", "Storage.CreateBacklogItemForDebug", 1, classCreateForwarder, "forwards the debug caller's item; counted as debug_create"},

	{"server/mcp/tools_backlog.go", "backlogHandlers.createBacklogItem", 1, classCreate, idleCreateReason},
	{"server/mcp/tools_backlog.go", "backlogHandlers.importGitHubIssue", 1, classCreate, idleCreateReason},
	{"server/mcp/tools_backlog_import_tracker.go", "backlogHandlers.importTrackerIssue", 1, classCreate, idleCreateReason},
	{"server/services/backlog_service_lifecycle.go", "BacklogService.CreateBacklogItem", 1, classCreate, idleCreateReason + " (through its data literal)"},
	{"server/services/backlog_service_sync.go", "BacklogService.ImportGitHubIssue", 1, classCreate, idleCreateReason},
	{"session/backlog_sync.go", "SyncLoop.SyncOne", 1, classCreateForwarder, "item comes from the source plugin's MapToBacklogItem (not resolvable here): the GitHub plugins set idea and the Jira and Linear plugins set no status"},

	{"server/services/backlog_debug_mutate_handler.go", "BacklogDebugMutateHandler.handleCreate", 1, classCreateDebug, debugCreateReason},
	{"server/services/backlog_debug_seed_handler.go", "BacklogDebugSeedHandler.createSimpleBacklogItem", 1, classCreateDebug, debugCreateReason},
	{"server/services/backlog_debug_seed_handler.go", "BacklogDebugSeedHandler.handleSeed", 1, classCreateDebug, debugCreateReason},
	{"server/services/backlog_debug_seed_handler.go", "BacklogDebugSeedHandler.handleSeedQueued", 1, classCreateDebug, debugCreateReason},
	{"server/services/backlog_debug_seed_handler.go", "BacklogDebugSeedHandler.handleSeedWorkSessionWithWorktree", 1, classCreateDebug, debugCreateReason},
}

// createCallName is the unqualified name of a creator call, or "".
func createCallName(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) != 2 {
		return ""
	}
	if _, isRequest := call.Args[1].(*ast.CallExpr); isRequest {
		return "" // the BacklogService RPC form, CreateBacklogItem(ctx, connect.NewRequest(...))
	}
	if sel.Sel.Name == "CreateBacklogItem" || sel.Sel.Name == "CreateBacklogItemForDebug" {
		return sel.Sel.Name
	}
	return ""
}

// statusExprIsIdle reports whether a Status value is BacklogStatusIdea, in
// either `BacklogStatusIdea`, `session.BacklogStatusIdea` or a string(...)
// conversion of one.
func statusExprIsIdle(e ast.Expr) bool {
	if conv, ok := e.(*ast.CallExpr); ok && len(conv.Args) == 1 {
		e = conv.Args[0]
	}
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name == "BacklogStatusIdea"
	case *ast.Ident:
		return v.Name == "BacklogStatusIdea"
	}
	return false
}

// literalCreatesIdle reports whether a BacklogItemData literal sets no Status
// or BacklogStatusIdea.
func literalCreatesIdle(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Status" {
				return statusExprIsIdle(kv.Value)
			}
		}
	}
	return true
}

// creatorItemIsIdle resolves the item argument of a creator call: a literal
// directly, or an identifier through its composite literal in the same
// function (any later `<ident>.Status = x` must also be idle). Anything it
// cannot resolve is not idle.
func creatorItemIsIdle(fd *ast.FuncDecl, arg ast.Expr) bool {
	switch v := arg.(type) {
	case *ast.CompositeLit:
		return literalCreatesIdle(v)
	case *ast.Ident:
		return identItemIsIdle(fd, v.Name)
	}
	return false
}

func identItemIsIdle(fd *ast.FuncDecl, name string) bool {
	resolved, idle := false, true
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != len(as.Rhs) {
			return true
		}
		for i, lhs := range as.Lhs {
			switch l := lhs.(type) {
			case *ast.Ident:
				if lit, ok := as.Rhs[i].(*ast.CompositeLit); ok && l.Name == name {
					resolved = true
					idle = idle && literalCreatesIdle(lit)
				}
			case *ast.SelectorExpr:
				if x, ok := l.X.(*ast.Ident); ok && x.Name == name && l.Sel.Name == "Status" {
					idle = idle && statusExprIsIdle(as.Rhs[i])
				}
			}
		}
		return true
	})
	return resolved && idle
}

type creatorCall struct {
	count        int
	forDebug     bool
	notIdleSites int
}

// scanCreators returns, per "file function", the creator calls and how many of
// them cannot be shown to create at idle.
func scanCreators(t *testing.T, root string) map[string]*creatorCall {
	t.Helper()
	found := map[string]*creatorCall{}
	walkModuleFuncs(t, root, func(rel string, fd *ast.FuncDecl) {
		key := rel + " " + funcDisplayName(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := createCallName(call)
			if name == "" {
				return true
			}
			c := found[key]
			if c == nil {
				c = &creatorCall{}
				found[key] = c
			}
			c.count++
			c.forDebug = c.forDebug || name == "CreateBacklogItemForDebug"
			if !creatorItemIsIdle(fd, call.Args[1]) {
				c.notIdleSites++
			}
			return true
		})
	})
	return found
}

func TestWorkflowBypassInventory_EveryCreatorIsClassifiedAndCreatesAtIdea(t *testing.T) {
	want := map[string]createSite{}
	for _, s := range createInventory {
		_, dup := want[s.key()]
		require.False(t, dup, "duplicate create inventory entry %s", s.key())
		assert.NotEmpty(t, strings.TrimSpace(s.reason), "%s lacks a reason", s.key())
		want[s.key()] = s
	}

	var problems []string
	actual := scanCreators(t, moduleRoot(t))
	for key, c := range actual {
		s, ok := want[key]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("unclassified CreateBacklogItem call in %s: create at BacklogStatusIdea (or set no Status), use CreateBacklogItemForDebug from an operator-only debug endpoint, or add a create inventory entry with a reason", key))
		case s.calls != c.count:
			problems = append(problems, fmt.Sprintf("%s has %d creator call(s), create inventory says %d", key, c.count, s.calls))
		case s.class == classCreate && c.notIdleSites > 0:
			problems = append(problems, fmt.Sprintf("%s creates a backlog item with a status other than BacklogStatusIdea (or one the test cannot resolve): only idea or no status is allowed outside CreateBacklogItemForDebug", key))
		case s.class == classCreateDebug && !c.forDebug:
			problems = append(problems, fmt.Sprintf("%s is listed as exempt: debug but does not call CreateBacklogItemForDebug", key))
		case s.class != classCreateDebug && c.forDebug:
			problems = append(problems, fmt.Sprintf("%s calls CreateBacklogItemForDebug but is not listed as exempt: debug", key))
		}
	}
	for key, s := range want {
		if _, ok := actual[key]; !ok {
			problems = append(problems, fmt.Sprintf("stale %s entry %s: no creator call remains, delete it", s.class, key))
		}
	}
	sort.Strings(problems)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}

func TestWorkflowBypassInventory_CreateScannerFlagsNonIdleStatuses(t *testing.T) {
	dir := t.TempDir()
	src := `package x
type a struct{}
func (a *a) Idle(s st)         { s.CreateBacklogItem(ctx, session.BacklogItemData{Status: string(session.BacklogStatusIdea)}) }
func (a *a) NoStatus(s st)     { s.CreateBacklogItem(ctx, session.BacklogItemData{Title: "t"}) }
func (a *a) Done(s st)         { s.CreateBacklogItem(ctx, session.BacklogItemData{Status: string(session.BacklogStatusDone)}) }
func (a *a) Variable(s st, v string) { s.CreateBacklogItem(ctx, session.BacklogItemData{Status: v}) }
func (a *a) ViaStringLiteral(s st)      { d := session.BacklogItemData{Status: "idea"}; _ = d; s.CreateBacklogItem(ctx, d) }
func (a *a) ViaIdent(s st)     { d := session.BacklogItemData{Status: string(session.BacklogStatusIdea)}; s.CreateBacklogItem(ctx, d) }
func (a *a) ViaReassigned(s st) { d := session.BacklogItemData{}; d.Status = string(session.BacklogStatusDone); s.CreateBacklogItem(ctx, d) }
func (a *a) Param(s st, d session.BacklogItemData) { s.CreateBacklogItem(ctx, d) }
func (a *a) RPC(s st)          { s.CreateBacklogItem(ctx, connect.NewRequest(req)) }
func (a *a) Debug(s st)        { s.CreateBacklogItemForDebug(ctx, session.BacklogItemData{Status: "done"}) }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600))
	got := map[string]int{}
	for key, c := range scanCreators(t, dir) {
		got[key] = c.notIdleSites
	}
	assert.Equal(t, map[string]int{
		"x.go a.Idle": 0, "x.go a.NoStatus": 0, "x.go a.Done": 1, "x.go a.Variable": 1,
		"x.go a.ViaStringLiteral": 1, "x.go a.ViaIdent": 0, "x.go a.ViaReassigned": 1, "x.go a.Param": 1,
		"x.go a.Debug": 1,
	}, got, "RPC-form calls are not creators; unresolved or non-idle statuses are flagged")
}
