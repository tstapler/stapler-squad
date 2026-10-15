//go:build sinkguard

package deliverygate

// The sink-enumeration guard (Story 2.6, ADR-001 decision 7): a type-based scan
// of the product packages that fails when a new delivery path appears without a
// reviewed allowlist entry, so the hidden-session leak cannot regrow a fourth
// time through a path the gate does not cover. Runs through
// `make test-delivery-guards`; it is not part of the inner `make test` loop.

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const (
	modulePath       = "github.com/tstapler/stapler-squad"
	eventsPkgPath    = modulePath + "/pkg/events"
	pushPkgPath      = modulePath + "/server/push"
	serverEventsPath = modulePath + "/server/events" // type/const aliases of pkg/events
	notifPkgPath     = modulePath + "/server/notifications"
	svcPkgPath       = modulePath + "/server/services"
	sessionPkgPath   = modulePath + "/session"
	fixturePkgSuffix = "/server/deliverygate/testdata/sinkfixture"
)

// finding is one scan hit, keyed "<kind>: <package path>.<identifier>".
type finding string

// reviewedNotifiers are the concrete push.Notifier implementations. A new one
// needs a reviewed reason, or must be routed through the gate.
var reviewedNotifiers = map[finding]string{
	"notifier: " + pushPkgPath + ".WebPushNotifier": "the only push backend; reached through StartDeliverySubscriber, which gates status-change pushes and receives only bus-filtered notifications",
}

// reviewedConsumers are the functions that subscribe to the bus in a package
// that handles events.EventNotification.
var reviewedConsumers = map[finding]string{
	"consumer: " + notifPkgPath + ".StartSubscriberWithInterval": "history store ingest; sits behind the bus publish filter",
	"consumer: " + pushPkgPath + ".StartDeliverySubscriber":      "push fan-out; sits behind the bus publish filter plus the status-change gate",

	// server/services references EventNotification (event_converter, producers),
	// so every Subscribe there is listed; only WatchSessions forwards it.
	"consumer: " + svcPkgPath + ".WatchSessions":                           "forwards EventNotification to connected clients via event_converter (toasts); sits behind the bus publish filter",
	"consumer: " + svcPkgPath + ".StartBacklogGitHubForwardSyncSubscriber": "subscribes for backlog item changes only; ignores notifications",
	"consumer: " + svcPkgPath + ".StartVerdictSteering":                    "subscribes for backlog item changes only; ignores notifications",
	"consumer: " + svcPkgPath + ".WatchUnfinishedWork":                     "converts only the unfinished-work event types (convertUnfinishedEvent); other types, notifications included, are dropped",
	"consumer: " + svcPkgPath + ".waitForEvent":                            "generic wait for a caller-supplied match predicate on a session transition; forwards nothing to a client",
	"consumer: " + svcPkgPath + ".watchBacklogItems":                       "backlog item stream; ignores notifications",
	"consumer: " + svcPkgPath + ".watchWorkflows":                          "workflow stream; ignores notifications",
}

// reviewedAppendCallers are the concrete NotificationHistoryStore.Append* call
// sites outside the notifications package itself.
var reviewedAppendCallers = map[finding]string{}

// reviewedRawHiddenReads are the raw session.Instance.Hidden selector reads in
// non-test server code. The legacy per-site notification checks were removed
// in Story 2.9; a new entry needs a reason that is not "notification".
var reviewedRawHiddenReads = map[finding]string{
	"hidden: " + modulePath + "/server/services.ListSessions":        "session-list include_hidden filtering, not notification delivery",
	"hidden: " + modulePath + "/server/services.WatchSessions":       "session-stream include_hidden filtering, not notification delivery",
	"hidden: " + modulePath + "/server/services.isAutomationSession": "automation-session classification",
}

// scanResult holds the findings of one scan, by category.
type scanResult struct {
	notifiers     map[finding]bool
	consumers     map[finding]bool
	appendCallers map[finding]bool
	hiddenReads   map[finding]bool
}

func loadPackages(t *testing.T, patterns ...string) []*packages.Package {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &packages.Config{
		Dir:   root,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			t.Fatalf("package %s: %v", p.PkgPath, e)
		}
	}
	return pkgs
}

// findNotifierInterface locates push.Notifier among the loaded packages and
// their imports.
func findNotifierInterface(pkgs []*packages.Package) *types.Interface {
	var found *types.Interface
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.PkgPath != pushPkgPath || p.Types == nil {
			return
		}
		if obj := p.Types.Scope().Lookup("Notifier"); obj != nil {
			if iface, ok := obj.Type().Underlying().(*types.Interface); ok {
				found = iface
			}
		}
	})
	return found
}

func scan(t *testing.T, pkgs []*packages.Package, onlyServer bool) scanResult {
	t.Helper()
	res := scanResult{
		notifiers: map[finding]bool{}, consumers: map[finding]bool{},
		appendCallers: map[finding]bool{}, hiddenReads: map[finding]bool{},
	}
	notifier := findNotifierInterface(pkgs)
	if notifier == nil {
		t.Fatal("push.Notifier interface not found; the guard would silently pass")
	}
	for _, p := range pkgs {
		if p.Types == nil || p.TypesInfo == nil {
			continue
		}
		scanNotifierImpls(p, notifier, res)
		scanFuncs(p, res, onlyServer)
	}
	return res
}

func scanNotifierImpls(p *packages.Package, iface *types.Interface, res scanResult) {
	scope := p.Types.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		if _, isIface := tn.Type().Underlying().(*types.Interface); isIface {
			continue
		}
		if types.Implements(tn.Type(), iface) || types.Implements(types.NewPointer(tn.Type()), iface) {
			res.notifiers[finding("notifier: "+p.PkgPath+"."+name)] = true
		}
	}
}

// fileReferences reports whether the file uses the named object from any of
// pkgPaths (server/events re-exports the constants as aliases).
func fileReferences(p *packages.Package, f *ast.File, name string, pkgPaths ...string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || found || id.Name != name {
			return !found
		}
		obj := p.TypesInfo.Uses[id]
		if obj == nil || obj.Pkg() == nil {
			return true
		}
		for _, path := range pkgPaths {
			if obj.Pkg().Path() == path {
				found = true
			}
		}
		return !found
	})
	return found
}

func scanFuncs(p *packages.Package, res scanResult, onlyServer bool) {
	// A bus consumer is a function that subscribes in a package that handles
	// events.EventNotification anywhere (the switch may sit in a helper or
	// another file). Broad on purpose: a new Subscribe in such a package must be
	// reviewed, even when it only wants other event types.
	refsNotification := false
	for _, f := range p.Syntax {
		refsNotification = refsNotification || fileReferences(p, f, "EventNotification", eventsPkgPath, serverEventsPath)
	}
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fname := fd.Name.Name
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					classifyCall(p, x, fname, refsNotification, res)
				case *ast.SelectorExpr:
					if isRawHiddenRead(p, x) && (!onlyServer || isServerPkg(p.PkgPath)) {
						res.hiddenReads[finding("hidden: "+p.PkgPath+"."+fname)] = true
					}
				}
				return true
			})
		}
	}
}

func isServerPkg(path string) bool {
	return path == modulePath+"/server" || strings.HasPrefix(path, modulePath+"/server/")
}

func classifyCall(p *packages.Package, call *ast.CallExpr, fname string, refsNotification bool, res scanResult) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	s := p.TypesInfo.Selections[sel]
	if s == nil {
		return
	}
	named := namedOf(s.Recv())
	if named == nil || named.Obj().Pkg() == nil {
		return
	}
	owner := named.Obj().Pkg().Path()
	switch {
	case owner == eventsPkgPath && named.Obj().Name() == "EventBus" && sel.Sel.Name == "Subscribe":
		if refsNotification {
			res.consumers[finding("consumer: "+p.PkgPath+"."+fname)] = true
		}
	case owner == notifPkgPath && named.Obj().Name() == "NotificationHistoryStore" &&
		strings.HasPrefix(sel.Sel.Name, "Append") && p.PkgPath != notifPkgPath:
		res.appendCallers[finding("append: "+p.PkgPath+"."+fname)] = true
	}
}

func isRawHiddenRead(p *packages.Package, sel *ast.SelectorExpr) bool {
	if sel.Sel.Name != "Hidden" {
		return false
	}
	s := p.TypesInfo.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal {
		return false
	}
	named := namedOf(s.Recv())
	return named != nil && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == sessionPkgPath && named.Obj().Name() == "Instance"
}

// namedOf returns the named type behind t, looking through pointers and aliases
// (server/events aliases the pkg/events types).
func namedOf(t types.Type) *types.Named {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	n, _ := t.(*types.Named)
	return n
}

func sortedKeys(m map[finding]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

func assertCovered(t *testing.T, label string, got map[finding]bool, allow map[finding]string) {
	t.Helper()
	for _, k := range sortedKeys(got) {
		if reason, ok := allow[finding(k)]; !ok || strings.TrimSpace(reason) == "" {
			t.Errorf("%s not in the reviewed allowlist (add it with a reason, or route it through the delivery gate): %s", label, k)
		}
	}
	for k := range allow {
		if !got[k] {
			t.Errorf("%s allowlist entry is stale (no longer found): %s", label, k)
		}
	}
}

// T-MX-05: every Notifier implementation, bus EventNotification consumer and
// history-store Append* caller in the product is in a reviewed allowlist.
func TestSinkGuard_ShouldRequireAllowlistReason_WhenNotifierImplEventNotificationConsumerOrAppendCallFound(t *testing.T) {
	pkgs := loadPackages(t, "./server/...", "./pkg/...", "./session/...", "./cmd/...", ".")
	var product []*packages.Package
	for _, p := range pkgs {
		if !strings.HasSuffix(p.PkgPath, fixturePkgSuffix) {
			product = append(product, p)
		}
	}
	res := scan(t, product, false)
	assertCovered(t, "notifier implementation", res.notifiers, reviewedNotifiers)
	assertCovered(t, "EventNotification consumer", res.consumers, reviewedConsumers)
	assertCovered(t, "NotificationHistoryStore.Append* caller", res.appendCallers, reviewedAppendCallers)
}

// T-MX-07: raw session.Instance.Hidden reads in server non-test code are
// allowlisted; none of the remaining readers is notification-related.
func TestRawHiddenReads_ShouldOnlyAppearInAllowlist_WhenScanningServerNonTestFiles(t *testing.T) {
	pkgs := loadPackages(t, "./server/...")
	var product []*packages.Package
	for _, p := range pkgs {
		if !strings.HasSuffix(p.PkgPath, fixturePkgSuffix) {
			product = append(product, p)
		}
	}
	res := scan(t, product, true)
	assertCovered(t, "raw Instance.Hidden read", res.hiddenReads, reviewedRawHiddenReads)
}

// T-MX-06: the negative control. The fixture package adds each kind of new
// path under unrelated names; the scan must report every one, so the guard
// cannot silently rot into a scan that finds nothing.
func TestSinkGuard_ShouldFail_WhenFixturePackageAddsNewNotifierOrDifferentlyNamedConsumer(t *testing.T) {
	pkgs := loadPackages(t, "./server/deliverygate/testdata/sinkfixture")
	res := scan(t, pkgs, false)
	fixture := modulePath + "/server/deliverygate/testdata/sinkfixture"
	for _, want := range []struct {
		label string
		got   map[finding]bool
		key   string
	}{
		{"notifier", res.notifiers, "notifier: " + fixture + ".RogueNotifier"},
		{"consumer", res.consumers, "consumer: " + fixture + ".WatchTheFirehose"},
		{"append caller", res.appendCallers, "append: " + fixture + ".WriteHistoryDirectly"},
		{"raw Hidden read", res.hiddenReads, "hidden: " + fixture + ".PeekHidden"},
	} {
		if !want.got[finding(want.key)] {
			t.Errorf("scan missed the fixture's %s (%s); found %v", want.label, want.key, sortedKeys(want.got))
		}
	}
	// And the allowlist logic itself must reject all of them.
	if len(reviewedConsumers[finding("consumer: "+fixture+".WatchTheFirehose")]) != 0 {
		t.Error("fixture finding must not be allowlisted")
	}
}
