package services

// workflow_bypass_strict_test.go is the strict half of the bypass inventory
// (ADR-007 D9, Story 1.5.9a). The inventory test in workflow_bypass_inventory_test.go
// classifies every status-write call; the rules here close the ways a writer can
// still go unnoticed:
//
//	(a) an old-name or Raw*/raw* writer call outside the allow-list,
//	(b) a TransitionRequest whose Caller is not a registered caller,
//	(c) a registered, non-forwarding caller no code path uses,
//	(d) a BacklogItem ent builder's SetStatus outside the allow-listed writers.
//
// Helpers carry the cls prefix to stay clear of other test files.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// --- rule (a): legacy writer names ---------------------------------------------

// clsLegacyWriterNames are the six pre-1.5.4 writer names, their Raw/raw forms,
// and the raw writer behind GatedReconcileStuck.
var clsLegacyWriterNames = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{
		"TransitionBacklogItemStatus", "ArchiveBacklogItem", "UnarchiveBacklogItem",
		"SetBacklogItemPRAndTransition", "ReconcileStuckItems", "TransitionBacklogItemStatusWithPRFields",
		"TransitionBacklogItemStatusWithNote",
	} {
		m[n] = true
		m["Raw"+n] = true
		m["raw"+strings.ToUpper(n[:1])+n[1:]] = true
	}
	return m
}()

// clsLegacyAllowedFiles are the only files an allow-listed legacy-name call may
// live in: the Storage wrappers, the gate, the repository writers and the MCP delegate.
var clsLegacyAllowedFiles = map[string]bool{
	"session/storage.go":                       true,
	"session/transition_gate.go":               true,
	"session/ent_repository_backlog.go":        true,
	"server/mcp/tools_backlog_dependencies.go": true,
}

// clsLegacyHandlerForwarders call the BacklogService RPC handler, which shares
// its name with an old writer but routes through the gate, so they hold no write.
var clsLegacyHandlerForwarders = map[string]string{
	"server/services/notification_action_backlog.go backlogDecider.transition": "forwards the operator's click to the TransitionBacklogItemStatus handler, which resolves operator_notification_action and writes through GatedTransition",
}

func clsScanLegacyCalls(t *testing.T, root string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	walkModuleFuncs(t, root, func(rel string, fd *ast.FuncDecl) {
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && clsLegacyWriterNames[calledName(call)] {
				counts[rel+" "+funcDisplayName(fd)]++
			}
			return true
		})
	})
	return counts
}

// clsLegacyViolations flags every legacy-name call site that the inventory does
// not carry as an exempt entry in an allowed file.
func clsLegacyViolations(actual map[string]int, inventory []bypassSite) []string {
	exemptSite := map[string]bool{}
	for _, s := range inventory {
		if s.class == classExempt {
			exemptSite[s.key()] = true
		}
	}
	var problems []string
	for key, n := range actual {
		file, _, _ := strings.Cut(key, " ")
		switch {
		case clsLegacyHandlerForwarders[key] != "":
		case !exemptSite[key]:
			problems = append(problems, fmt.Sprintf("%s makes %d call(s) to an old-name or Raw/raw status writer outside the allow-list: route through Storage.Gated* (ADR-007 D1)", key, n))
		case !clsLegacyAllowedFiles[file]:
			problems = append(problems, fmt.Sprintf("%s is an exempt legacy-name site in %s, which is not an allowed file", key, file))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestWorkflowBypassStrict_NoLegacyWriterCallOutsideTheAllowList(t *testing.T) {
	problems := clsLegacyViolations(clsScanLegacyCalls(t, moduleRoot(t)), bypassInventory)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}

func TestWorkflowBypassStrict_LegacyRuleFlagsAnyReceiverAndFormNamingFileAndFunction(t *testing.T) {
	dir := t.TempDir()
	src := `package x
func Sneaky(s interface{ RawArchiveBacklogItem(); rawUnarchiveBacklogItem(); ReconcileStuckItems() }) {
	s.RawArchiveBacklogItem()
	s.rawUnarchiveBacklogItem()
	s.ReconcileStuckItems()
}
func Fine(s interface{ GatedArchive() }) { s.GatedArchive() }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600))
	problems := clsLegacyViolations(clsScanLegacyCalls(t, dir), nil)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "x.go Sneaky makes 3 call(s)")

	inv := []bypassSite{exempt("x.go", "Sneaky", 3, "test")}
	problems = clsLegacyViolations(clsScanLegacyCalls(t, dir), inv)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "not an allowed file", "an exempt entry outside the allowed files is still a violation")
}

// --- rule (b): every TransitionRequest names a registered caller ----------------

// clsRequestLiteral is one TransitionRequest composite literal in non-test code.
type clsRequestLiteral struct {
	key      string // "file function"
	caller   string // source text of the Caller value, "" when the key is absent
	constRef string // the Caller* identifier when the value is a constant reference
}

func clsScanRequestLiterals(t *testing.T, root string) []clsRequestLiteral {
	t.Helper()
	var out []clsRequestLiteral
	walkModuleFuncs(t, root, func(rel string, fd *ast.FuncDecl) {
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || clsTypeName(lit.Type) != "TransitionRequest" {
				return true
			}
			rl := clsRequestLiteral{key: rel + " " + funcDisplayName(fd)}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Caller" {
						rl.caller = clsExprText(kv.Value)
						rl.constRef = clsTypeName(kv.Value)
					}
				}
			}
			out = append(out, rl)
			return true
		})
	})
	return out
}

// clsTypeName is the trailing identifier of an identifier or selector, "" otherwise.
func clsTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

func clsExprText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return clsExprText(v.X) + "." + v.Sel.Name
	case *ast.CallExpr:
		return clsExprText(v.Fun) + "(...)"
	}
	return fmt.Sprintf("%T", e)
}

// clsRegistryConstants maps each Caller* constant in session/transition_callers.go
// to its string value.
func clsRegistryConstants(t *testing.T, root string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "session", "transition_callers.go"), nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if len(vs.Names) == 1 && len(vs.Values) == 1 && strings.HasPrefix(vs.Names[0].Name, "Caller") {
				if lit, ok := vs.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					out[vs.Names[0].Name] = strings.Trim(lit.Value, `"`)
				}
			}
		}
	}
	return out
}

// clsDynamicCallers are the functions whose Caller is computed. Each is bounded
// by a test below that proves every value it can return is registered.
var clsDynamicCallers = map[string]string{
	"server/services/backlog_service_lifecycle.go BacklogService.TransitionBacklogItemStatus": "resolveTransitionCaller(ctx), bounded by TestWorkflowBypassStrict_ResolveTransitionCallerReturnsRegisteredCallers",
	"server/services/backlog_service_triage.go BacklogService.transitionWithGuard":            "guardedTransitionCaller(to, actor), bounded by TestWorkflowBypassStrict_GuardedTransitionCallerReturnsRegisteredCallers",
	"session/backlog_remediation.go Storage.redriveOne":                                       "the recorded denial's caller, re-checked by LookupTransitionCaller in transitionStillDenied and limited to redriveByWrite",
	"session/backlog_remediation.go Storage.transitionStillDenied":                            "a decision only (no write): spec.Name is a LookupTransitionCaller result",
}

func clsRequestViolations(lits []clsRequestLiteral, consts map[string]string, dynamic map[string]string) []string {
	var problems []string
	for _, l := range lits {
		value, isConst := consts[l.constRef]
		switch {
		case l.caller == "":
			problems = append(problems, fmt.Sprintf("%s builds a TransitionRequest with no Caller: name a registered caller (session/transition_callers.go)", l.key))
		case isConst:
			if _, ok := session.LookupTransitionCaller(session.TransitionCaller(value)); !ok {
				problems = append(problems, fmt.Sprintf("%s uses %s (%q), which is not in the caller registry", l.key, l.constRef, value))
			}
		case dynamic[l.key] != "":
		default:
			problems = append(problems, fmt.Sprintf("%s computes its Caller (%s): use a registered Caller* constant, or list the function in clsDynamicCallers with a bounding test", l.key, l.caller))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestWorkflowBypassStrict_EveryTransitionRequestNamesARegisteredCaller(t *testing.T) {
	root := moduleRoot(t)
	lits := clsScanRequestLiterals(t, root)
	require.NotEmpty(t, lits, "the scan must see the production literals")
	problems := clsRequestViolations(lits, clsRegistryConstants(t, root), clsDynamicCallers)
	assert.Empty(t, problems, strings.Join(problems, "\n"))

	seen := map[string]bool{}
	for _, l := range lits {
		seen[l.key] = true
	}
	for key := range clsDynamicCallers {
		assert.True(t, seen[key], "stale clsDynamicCallers entry %s: no TransitionRequest literal remains there", key)
	}
}

func TestWorkflowBypassStrict_RequestRuleFlagsUnregisteredMissingAndComputedCallers(t *testing.T) {
	dir := t.TempDir()
	src := `package x
func Unregistered() { _ = session.TransitionRequest{Caller: session.CallerBogus} }
func Missing()      { _ = session.TransitionRequest{ItemID: "a"} }
func Computed(c string) { _ = TransitionRequest{Caller: pick(c)} }
func Good()         { _ = session.TransitionRequest{Caller: session.CallerQueueBacklogItem} }
func Dynamic(c string)  { _ = TransitionRequest{Caller: pick(c)} }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600))
	consts := map[string]string{"CallerBogus": "no_such_caller", "CallerQueueBacklogItem": "queue_backlog_item"}
	problems := clsRequestViolations(clsScanRequestLiterals(t, dir), consts, map[string]string{"x.go Dynamic": "bounded"})
	require.Len(t, problems, 3, strings.Join(problems, "\n"))
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "x.go Unregistered uses CallerBogus")
	assert.Contains(t, joined, "x.go Missing builds a TransitionRequest with no Caller")
	assert.Contains(t, joined, "x.go Computed computes its Caller")
}

func TestWorkflowBypassStrict_ResolveTransitionCallerReturnsRegisteredCallers(t *testing.T) {
	for _, actor := range []string{session.TriggeredByUser, session.TriggeredByAgent, session.TriggeredBySystem, "robot", ""} {
		ctx := WithTransitionActor(t.Context(), actor)
		_, ok := session.LookupTransitionCaller(resolveTransitionCaller(ctx))
		assert.True(t, ok, "actor %q", actor)
		ctx = WithTransitionCaller(ctx, session.CallerOperatorNotificationAction)
		_, ok = session.LookupTransitionCaller(resolveTransitionCaller(ctx))
		assert.True(t, ok, "actor %q with the notification caller", actor)
	}
}

func TestWorkflowBypassStrict_GuardedTransitionCallerReturnsRegisteredCallers(t *testing.T) {
	statuses := []session.BacklogStatus{
		session.BacklogStatusIdea, session.BacklogStatusRefining, session.BacklogStatusReady, session.BacklogStatusQueued,
		session.BacklogStatusInProgress, session.BacklogStatusReview, session.BacklogStatusPRPending, session.BacklogStatusDone,
		session.BacklogStatusArchived,
	}
	for _, to := range statuses {
		for _, actor := range []string{session.TriggeredByUser, session.TriggeredByAgent, session.TriggeredBySystem, ""} {
			_, ok := session.LookupTransitionCaller(guardedTransitionCaller(to, actor))
			assert.True(t, ok, "to=%s actor=%q", to, actor)
		}
	}
}

// --- rule (c): every registered caller has a call path --------------------------

// clsCallerRefs maps each Caller* constant to the "file function" keys that
// mention it in non-test code outside the registry file.
func clsCallerRefs(t *testing.T, root string, consts map[string]string) map[string]map[string]bool {
	t.Helper()
	refs := map[string]map[string]bool{}
	walkModuleFuncs(t, root, func(rel string, fd *ast.FuncDecl) {
		if rel == "session/transition_callers.go" {
			return
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			name := ""
			switch v := n.(type) {
			case *ast.Ident:
				name = v.Name
			case *ast.SelectorExpr:
				name = v.Sel.Name
			}
			if _, ok := consts[name]; ok {
				if refs[name] == nil {
					refs[name] = map[string]bool{}
				}
				refs[name][rel+" "+funcDisplayName(fd)] = true
			}
			return true
		})
	})
	return refs
}

// clsDynamicProducerKeys are the functions that compute a caller from the
// request, so a constant they mention is reachable (bounded by the tests above).
var clsDynamicProducerKeys = []string{
	"server/services/backlog_service_triage.go guardedTransitionCaller",
	"server/services/backlog_service_lifecycle.go resolveTransitionCaller",
}

// clsUnusedCallers returns the non-forwarding registry entries with no call path:
// no owned site or origin, or a constant mentioned nowhere a write can start.
func clsUnusedCallers(specs []session.TransitionCallerSpec, consts map[string]string, lits []clsRequestLiteral, refs map[string]map[string]bool) []string {
	constFor := map[string]string{}
	for name, value := range consts {
		constFor[value] = name
	}
	literal := map[string]bool{}
	for _, l := range lits {
		literal[l.constRef] = true
	}
	var problems []string
	for _, spec := range specs {
		if spec.Forwarding {
			continue
		}
		name := constFor[string(spec.Name)]
		if name == "" {
			problems = append(problems, fmt.Sprintf("registered caller %s has no Caller* constant in session/transition_callers.go", spec.Name))
			continue
		}
		reached := literal[name]
		for _, cs := range append(append([]session.CallSite{}, spec.Sites...), spec.Origins...) {
			reached = reached || refs[name][cs.File+" "+cs.Func]
		}
		for _, key := range clsDynamicProducerKeys {
			reached = reached || refs[name][key]
		}
		switch {
		case len(spec.Sites) == 0 && len(spec.Origins) == 0:
			problems = append(problems, fmt.Sprintf("registered caller %s owns no call site or origin: remove it or register its site", spec.Name))
		case !reached:
			problems = append(problems, fmt.Sprintf("registered caller %s (%s) is named by no TransitionRequest and by none of its own sites: delete the entry or wire the caller", spec.Name, name))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestWorkflowBypassStrict_EveryNonForwardingRegisteredCallerHasACallSite(t *testing.T) {
	root := moduleRoot(t)
	consts := clsRegistryConstants(t, root)
	problems := clsUnusedCallers(session.TransitionCallers(), consts, clsScanRequestLiterals(t, root), clsCallerRefs(t, root, consts))
	assert.Empty(t, problems, strings.Join(problems, "\n"))

	forwarding := 0
	for _, c := range session.TransitionCallers() {
		if c.Forwarding {
			forwarding++
			assert.Equal(t, session.CallerMCPTransitionDelegate, c.Name, "only the MCP delegate is exempt; its coverage is the handler caller-resolution test")
		}
	}
	assert.Equal(t, 1, forwarding)
}

func TestWorkflowBypassStrict_UnusedCallerRuleFlagsAnEntryWithNoPath(t *testing.T) {
	consts := map[string]string{"CallerGhost": "ghost", "CallerLive": "live", "CallerFwd": "fwd"}
	specs := []session.TransitionCallerSpec{
		{Name: "ghost", Sites: []session.CallSite{{File: "a.go", Func: "F", Calls: 1}}},
		{Name: "live", Sites: []session.CallSite{{File: "a.go", Func: "G", Calls: 1}}},
		{Name: "fwd", Forwarding: true},
		{Name: "nosite"},
		{Name: "noconst", Sites: []session.CallSite{{File: "a.go", Func: "H", Calls: 1}}},
	}
	lits := []clsRequestLiteral{{key: "a.go G", caller: "CallerLive", constRef: "CallerLive"}, {key: "a.go H", caller: "pick(...)"}}
	problems := clsUnusedCallers(specs, consts, lits, map[string]map[string]bool{})
	require.Len(t, problems, 3, strings.Join(problems, "\n"))
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "ghost")
	assert.Contains(t, joined, "nosite")
	assert.Contains(t, joined, "noconst has no Caller* constant")
}

// --- rule (d): BacklogItem builder SetStatus ------------------------------------

const clsEntBacklogFile = "session/ent_repository_backlog.go"

// clsSetStatusAllowed are the only functions in package session that may call
// SetStatus on a BacklogItem create/update/upsert builder.
var clsSetStatusAllowed = map[string]string{
	clsEntBacklogFile + " EntRepository.CreateBacklogItem":                          "create: items are created at idea or with no status (Task 1.5.4g)",
	clsEntBacklogFile + " EntRepository.rawArchiveBacklogItem":                      "raw writer behind GatedArchive",
	clsEntBacklogFile + " EntRepository.rawUnarchiveBacklogItem":                    "raw writer behind GatedUnarchive",
	clsEntBacklogFile + " EntRepository.rawTransitionBacklogItemStatusWithNote":     "raw writer behind GatedTransition and GatedReconcileStuck",
	clsEntBacklogFile + " EntRepository.rawTransitionBacklogItemStatusWithPRFields": "raw writer behind GatedSetPRAndTransition",
}

// clsMentionsBacklogItemBuilder reports whether e is, or was assigned from, an
// expression naming the ent BacklogItem client or one of its builder types.
func clsMentionsBacklogItemBuilder(fd *ast.FuncDecl, e ast.Expr, depth int) bool {
	if depth > 6 {
		return false
	}
	switch v := e.(type) {
	case *ast.CallExpr:
		return clsMentionsBacklogItemBuilder(fd, v.Fun, depth)
	case *ast.SelectorExpr:
		return clsIsBacklogItemName(v.Sel.Name) || clsMentionsBacklogItemBuilder(fd, v.X, depth)
	case *ast.StarExpr:
		return clsMentionsBacklogItemBuilder(fd, v.X, depth)
	case *ast.ParenExpr:
		return clsMentionsBacklogItemBuilder(fd, v.X, depth)
	case *ast.Ident:
		return clsIdentDefinedFromBacklogItem(fd, v.Name, depth+1)
	}
	return false
}

func clsIsBacklogItemName(name string) bool {
	switch name {
	case "BacklogItem", "BacklogItemUpdate", "BacklogItemUpdateOne", "BacklogItemCreate", "BacklogItemUpsert", "BacklogItemMutation":
		return true
	}
	return false
}

func clsIdentDefinedFromBacklogItem(fd *ast.FuncDecl, name string, depth int) bool {
	found := false
	checkParams := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				if n.Name == name && clsMentionsBacklogItemBuilder(fd, f.Type, depth) {
					found = true
				}
			}
		}
	}
	checkParams(fd.Type.Params)
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			checkParams(v.Type.Params)
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					rhs := v.Rhs[0]
					if i < len(v.Rhs) {
						rhs = v.Rhs[i]
					}
					found = found || clsMentionsBacklogItemBuilder(fd, rhs, depth)
				}
			}
		case *ast.ValueSpec:
			for i, id := range v.Names {
				if id.Name != name {
					continue
				}
				if v.Type != nil {
					found = found || clsMentionsBacklogItemBuilder(fd, v.Type, depth)
				}
				if i < len(v.Values) {
					found = found || clsMentionsBacklogItemBuilder(fd, v.Values[i], depth)
				}
			}
		}
		return true
	})
	return found
}

// clsSetStatusViolations scans <root>/session (the only package that can reach
// the ent client) for SetStatus calls on a BacklogItem builder outside allowed.
func clsSetStatusViolations(t *testing.T, root string, allowed map[string]string) []string {
	t.Helper()
	var problems []string
	hit := map[string]bool{}
	walkModuleFuncs(t, root, func(rel string, fd *ast.FuncDecl) {
		if filepath.ToSlash(filepath.Dir(rel)) != "session" {
			return
		}
		key := rel + " " + funcDisplayName(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "SetStatus" || !clsMentionsBacklogItemBuilder(fd, sel.X, 0) {
				return true
			}
			hit[key] = true
			if allowed[key] == "" {
				problems = append(problems, fmt.Sprintf("%s calls SetStatus on a BacklogItem ent builder outside the allow-listed writers: a status write must go through Storage.Gated* (ADR-007 D1)", key))
			}
			return true
		})
	})
	for key := range allowed {
		if !hit[key] {
			problems = append(problems, fmt.Sprintf("stale SetStatus allow-list entry %s: no BacklogItem SetStatus call remains there", key))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestBacklogItemSetStatus_OnlyInsideAllowListedWriters(t *testing.T) {
	problems := clsSetStatusViolations(t, moduleRoot(t), clsSetStatusAllowed)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}

func TestBacklogItemSetStatus_FixtureSeventhWriterIsFlaggedNamingFileAndFunction(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "session"), 0o750))
	src := `package session
type repo struct{ client *entClient }
func (r *repo) Sneaky(id int) error {
	_, err := r.client.BacklogItem.UpdateOneID(id).SetStatus("done").Save(nil)
	return err
}
func (r *repo) ViaVariable(id int) error {
	u := r.client.BacklogItem.UpdateOneID(id)
	_, err := u.SetArchivedAt(nil).SetStatus("done").Save(nil)
	return err
}
func (r *repo) ViaUpsert() { r.client.X.OnConflict().Update(func(u *ent.BacklogItemUpsert) { u.SetStatus("done") }) }
func (r *repo) Create() { r.client.BacklogItem.Create().SetStatus("idea") }
func (r *repo) OtherEntity(id int) { r.client.Session.UpdateOneID(id).SetStatus(1); r.client.BacklogProgressNote.Create().SetStatus("x") }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "session", "x.go"), []byte(src), 0o600))

	problems := clsSetStatusViolations(t, dir, map[string]string{"session/x.go repo.Create": "create"})
	require.Len(t, problems, 3, strings.Join(problems, "\n"))
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "session/x.go repo.Sneaky")
	assert.Contains(t, joined, "session/x.go repo.ViaVariable")
	assert.Contains(t, joined, "session/x.go repo.ViaUpsert")
	assert.NotContains(t, joined, "OtherEntity", "a progress note or session builder is a different type")
}
