package services

// workflow_bypass_inventory_test.go is the repo-wide inventory of every call
// to a backlog status-write primitive (ADR-007 D9, Story 1.2.3b). It walks the
// whole module with go/parser, so a new raw call anywhere (server/, session/,
// server/mcp/, any other package) fails here unless it is classified.
//
// The pending list is a ratchet: it can only shrink. Epic 1.5 migrates a site,
// then deletes its entry; a stale entry (no matching call, or a different call
// count) fails. Story 1.5.9a empties the list.

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
// by selector name on any receiver. The Raw/raw forms are the post-1.5.4 names.
var bypassPrimitives = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{
		"TransitionBacklogItemStatus", "ArchiveBacklogItem", "UnarchiveBacklogItem",
		"SetBacklogItemPRAndTransition", "ReconcileStuckItems", "TransitionBacklogItemStatusWithPRFields",
	} {
		m[n] = true
		m["Raw"+n] = true
		m["raw"+strings.ToUpper(n[:1])+n[1:]] = true
	}
	return m
}()

type bypassClass string

const (
	classEngineRouted bypassClass = "engine-routed"
	classPending      bypassClass = "pending"
	classExempt       bypassClass = "exempt"
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

func engineRouted(file, fn, how string) bypassSite {
	return bypassSite{file: file, fn: fn, calls: 1, class: classEngineRouted, reason: how}
}

func pending(file, fn string, calls int, story string) bypassSite {
	return bypassSite{file: file, fn: fn, calls: calls, class: classPending, reason: "migrate in Story " + story}
}

func exempt(file, fn string, calls int, why string) bypassSite {
	return bypassSite{file: file, fn: fn, calls: calls, class: classExempt, reason: why}
}

// bypassInventory is the classification of every call at the time of writing
// (recounted by git grep at the Story 1.2.3 HEAD).
var bypassInventory = []bypassSite{
	// Exempt: wrappers and the MCP delegate.
	exempt("session/storage.go", "Storage.TransitionBacklogItemStatus", 1, wrapperReason),
	exempt("session/storage.go", "Storage.ArchiveBacklogItem", 1, wrapperReason),
	exempt("session/storage.go", "Storage.UnarchiveBacklogItem", 1, wrapperReason),
	exempt("session/storage.go", "Storage.SetBacklogItemPRAndTransition", 1, wrapperReason+"; its call is TransitionBacklogItemStatusWithPRFields"),
	exempt("session/transition_gate.go", "Storage.writeGated", 1, gateReason+": the write after the engine allowed"),
	exempt("session/transition_gate.go", "Storage.Rollback", 1, gateReason+": exempt entry point, counted"),
	exempt("session/transition_gate.go", "Storage.DebugForce", 2, gateReason+": exempt entry point, counted (transition and archive)"),
	exempt("server/mcp/tools_backlog_dependencies.go", "backlogHandlers.transitionBacklogItemStatus", 1, delegateReason),
	exempt("server/services/notification_action_backlog.go", "backlogDecider.transition", 1, "notification action forwards the operator's click to the RPC handler, which routes through the engine; Story 1.5.5 registers it as an operator caller"),

	// Engine-routed: ask the WorkflowEngine today; re-pointed at the gate in Stories 1.5.5-1.5.7.
	engineRouted("server/services/backlog_service_lifecycle.go", "BacklogService.TransitionBacklogItemStatus", "handler: CanTransition + ValidateGates before the write"),
	engineRouted("server/services/backlog_service_lifecycle.go", "BacklogService.OverrideVerdict", "asks the engine before the write"),
	engineRouted("server/services/backlog_service_triage.go", "BacklogService.transitionWithGuard", "engine check, shared by queue/dequeue/dispatch callers"),
	engineRouted("server/services/backlog_service_sync.go", "BacklogService.AttachSessionToItem", "engine check before ->in_progress"),
	engineRouted("session/backlog_sync.go", "SyncLoop.SyncOne", "engine check on the backward-sync target"),

	// Pending, Story 1.5.5 (operator and agent writers).
	pending("server/services/backlog_service_lifecycle.go", "BacklogService.SubmitManualReview", 1, "1.5.5a"),
	pending("server/services/backlog_service_lifecycle.go", "BacklogService.ArchiveBacklogItem", 1, "1.5.5b"),
	pending("server/services/backlog_service_lifecycle.go", "BacklogService.UnarchiveBacklogItem", 1, "1.5.5b"),
	pending("server/services/backlog_service_lifecycle.go", "BacklogService.UpdateBacklogItem", 1, "1.5.5b (PR association via SetBacklogItemPRAndTransition)"),
	pending("server/mcp/tools_backlog.go", "backlogHandlers.requestReview", 2, "1.5.5c"),
	pending("server/mcp/tools_backlog.go", "backlogHandlers.reportBlocked", 1, "1.5.5c"),
	pending("server/mcp/tools_backlog.go", "backlogHandlers.resumeWork", 1, "1.5.5c"),
	pending("server/mcp/tools_backlog.go", "backlogHandlers.reportDuplicate", 1, "1.5.5c (ArchiveBacklogItem)"),
	pending("server/mcp/tools_backlog_pr.go", "backlogHandlers.reportPRCreated", 1, "1.5.5c (SetBacklogItemPRAndTransition)"),

	// Pending, Story 1.5.6 (system writers in server/services). Rollback sites move to gate.Rollback.
	pending("server/services/autonomous_orchestration_service.go", "AutonomousOrchestrationService.onAutonomousDriverComplete", 1, "1.5.6a"),
	pending("server/services/backlog_service_triage.go", "BacklogService.AutoReopenAfterFailedReview", 2, "1.5.6b (transition) and 1.5.6d (rollback to gate.Rollback)"),
	pending("server/services/backlog_service_triage.go", "BacklogService.autoReopenForPRFix", 2, "1.5.6c (transition and rollback)"),
	pending("server/services/backlog_service_triage.go", "BacklogService.claimAndSpawnCandidate", 1, "1.5.6d (rollback to gate.Rollback)"),
	pending("server/services/backlog_service_triage.go", "BacklogService.spawnSessionAfterGates", 1, "1.5.6d"),
	pending("server/services/backlog_service_triage.go", "BacklogService.respawnTriage", 1, "1.5.6d"),
	pending("server/services/backlog_service_triage.go", "BacklogService.MaybeTriggerTriage", 1, "1.5.6d"),
	pending("server/services/backlog_service_triage.go", "BacklogService.TriggerReReview", 1, "1.5.6d"),
	pending("server/services/backlog_service_triage.go", "BacklogService.forceResetItem", 1, "1.5.6e"),
	pending("server/services/backlog_service_trigger_triage.go", "BacklogService.TriggerTriage", 2, "1.5.6e"),
	pending("server/services/backlog_debug_mutate_handler.go", "BacklogDebugMutateHandler.handleTransition", 1, "1.5.6f (to DebugForce, exempt class debug)"),
	pending("server/services/backlog_debug_mutate_handler.go", "BacklogDebugMutateHandler.handleArchive", 1, "1.5.6f (to DebugForce, exempt class debug)"),

	// Pending, Story 1.5.7 (system writers in session).
	pending("session/backlog_lifecycle.go", "BacklogLifecycleListener.onSessionExited", 1, "1.5.7a"),
	pending("session/backlog_lifecycle.go", "BacklogLifecycleListener.transitionBouncingItemToDone", 2, "1.5.7a"),
	pending("session/backlog_lifecycle.go", "BacklogLifecycleListener.ReconcileStuck", 1, "1.5.7a (ReconcileStuckItems, via GatedReconcileStuck)"),
	pending("session/backlog_lifecycle_archive.go", "BacklogLifecycleListener.archiveStaleDoneItems", 1, "1.5.7a"),
	pending("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.closeIfSupersededByMain", 1, "1.5.7b"),
	pending("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.pushAndCreatePR", 1, "1.5.7b"),
	pending("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.reconcileOrphanedAgentPRs", 1, "1.5.7b (SetBacklogItemPRAndTransition)"),
	pending("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.reconcilePRPendingItem", 1, "1.5.7b"),
	pending("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.recoverDriftedPRItem", 1, "1.5.7b"),
	pending("session/backlog_lifecycle_pr.go", "BacklogLifecycleListener.resolveToPRPending", 1, "1.5.7b"),
	pending("session/jules_session_poller.go", "JulesSessionPoller.failJulesSession", 1, "1.5.7c"),
	pending("session/jules_session_poller.go", "JulesSessionPoller.applyCompletedState", 3, "1.5.7c (two transitions and SetBacklogItemPRAndTransition)"),
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
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && bypassPrimitives[sel.Sel.Name] {
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
		assert.Contains(t, []bypassClass{classEngineRouted, classPending, classExempt}, s.class, s.key())
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
		inScope := s.class == classPending || s.class == classEngineRouted || s.file == delegateFile
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
