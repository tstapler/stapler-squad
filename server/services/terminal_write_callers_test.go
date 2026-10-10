//go:build sinkguard

package services

// Checks (b) and (c) of Story 5.1d (the unary half, PR 5u). Run through
// `make test-delivery-guards`.
//
//   (b) every RPC of every session.v1 service is classified as a terminal write
//       or not; UpdateSession is classified by field.
//   (c) the callers of the primitive set, the steer chain entry points and the
//       capability and lease constructors equal the pinned table in both
//       directions.

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ---- check (b) ---------------------------------------------------------

// rpcClassificationFindings compares the descriptors with the tables.
func rpcClassificationFindings(descriptors map[string][]string, writes map[string]string, notWrites map[string][]string) []string {
	var out []string
	classified := map[string]int{}
	for k := range writes {
		classified[k]++
	}
	for svc, names := range notWrites {
		for _, n := range names {
			classified[svc+"."+n]++
		}
	}
	live := map[string]bool{}
	for svc, names := range descriptors {
		for _, n := range names {
			key := svc + "." + n
			live[key] = true
			switch classified[key] {
			case 0:
				out = append(out, "unclassified RPC (add it to terminalWriteRPCs or notTerminalWriteRPCs): "+key)
			case 1:
			default:
				out = append(out, "RPC classified twice: "+key)
			}
		}
	}
	for key := range classified {
		if !live[key] {
			out = append(out, "stale classification, no such RPC: "+key)
		}
	}
	sort.Strings(out)
	return out
}

func sessionV1Descriptors() map[string][]string {
	out := map[string][]string{}
	protoregistry.GlobalFiles.RangeFilesByPackage("session.v1", func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			s := svcs.Get(i)
			for j := 0; j < s.Methods().Len(); j++ {
				out[string(s.Name())] = append(out[string(s.Name())], string(s.Methods().Get(j).Name()))
			}
		}
		return true
	})
	return out
}

func updateSessionFields() []string {
	var out []string
	fields := (&sessionv1.UpdateSessionRequest{}).ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		out = append(out, string(fields.Get(i).Name()))
	}
	return out
}

// T-RO-17
func TestUIRpcSurface_ShouldClassifyEveryRPCAsWriteOrNot_WhenReflectingSessionServiceDescriptors(t *testing.T) {
	desc := sessionV1Descriptors()
	if len(desc) != 14 {
		t.Fatalf("expected 14 services in session.v1, found %d; a new service must be classified", len(desc))
	}
	for _, f := range rpcClassificationFindings(desc, terminalWriteRPCs, notTerminalWriteRPCs) {
		t.Error(f)
	}
	for key, reason := range terminalWriteRPCs {
		if reason == "" {
			t.Errorf("terminal write %s has no recorded reason", key)
		}
	}
	for key := range notTerminalWriteReasons {
		svc, name, _ := strings.Cut(key, ".")
		found := false
		for _, n := range notTerminalWriteRPCs[svc] {
			found = found || n == name
		}
		if !found {
			t.Errorf("reason recorded for %s, which is not in notTerminalWriteRPCs", key)
		}
	}
	// UpdateSession is a terminal write by field: steer_message, program, auto_approve.
	got := map[string]bool{}
	for _, f := range updateSessionFields() {
		got[f] = true
		if _, ok := updateSessionFieldClass[f]; !ok {
			t.Errorf("UpdateSessionRequest.%s is not classified in updateSessionFieldClass", f)
		}
	}
	var writeFields []string
	for f, class := range updateSessionFieldClass {
		if !got[f] {
			t.Errorf("updateSessionFieldClass lists %s, which UpdateSessionRequest no longer has", f)
		}
		if strings.HasPrefix(class, "WRITE") {
			writeFields = append(writeFields, f)
		}
	}
	sort.Strings(writeFields)
	if strings.Join(writeFields, ",") != "auto_approve,program,steer_message" {
		t.Errorf("UpdateSession terminal-write fields = %v, want auto_approve, program, steer_message", writeFields)
	}
}

// T-RO-17 negative control: an unclassified RPC and a stale row both fail.
func TestRPCClassification_ShouldFail_WhenAnRPCIsUnclassifiedOrARowIsStale(t *testing.T) {
	desc := map[string][]string{"SessionService": {"WriteToSession", "BrandNewRPC"}}
	f := rpcClassificationFindings(desc, map[string]string{"SessionService.WriteToSession": "x"},
		map[string][]string{"SessionService": {"GoneRPC"}})
	if !containsSubstring(f, "unclassified RPC (add it to terminalWriteRPCs or notTerminalWriteRPCs): SessionService.BrandNewRPC") {
		t.Errorf("an unclassified RPC was not flagged: %v", f)
	}
	if !containsSubstring(f, "stale classification, no such RPC: SessionService.GoneRPC") {
		t.Errorf("a stale row was not flagged: %v", f)
	}
}

// ---- check (c) ---------------------------------------------------------

// callerKind says why a unit may call the surface.
type callerKind string

const (
	kindUI                 callerKind = "ui"
	kindAcquirer           callerKind = "acquirer"
	kindReceiver           callerKind = "receiver"
	kindLifecycle          callerKind = "lifecycle"
	kindNone               callerKind = "none"
	kindReceiverViaWrapper callerKind = "receiver-via-wrapper"
	kindChain              callerKind = "chain"
)

type callerRow struct {
	kind     callerKind
	acquires bool // also takes the write lease (counted by the closed-acquirer test)
	reason   string
}

// callerConfig is the surface the pinned-caller test watches.
type callerConfig struct {
	// surface: "Recv.Method" for methods (Recv is the named receiver type),
	// "name" for package-level functions in the module.
	surface map[string]bool
	// leaseConstructors are the acquire entry points, pinned as the acquirer list.
	leaseConstructors map[string]bool
	// accessConstructors are the unary capability constructors.
	accessConstructors map[string]bool
	// lifecycle are the lifecycle methods whose callers are pinned (T-RO-46).
	lifecycle map[string]bool
	// exemptPkgs and exemptFiles are the PTY layer, exempt by name.
	exemptPkgs  []string
	exemptFiles map[string]bool
	// exemptUnits are the primitives' own definitions and delegations.
	exemptUnits map[string]bool
}

func realCallerConfig() callerConfig {
	set := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	return callerConfig{
		surface: set(
			"Instance.SendKeys", "Instance.SendKeysN", "SubmitReplyOnce", "Instance.WriteToPTY", "Instance.TapEnter", "Instance.SendPrompt", "Instance.changeDirectory",
			"SubmitDriverContent", "SubmitContentWithEnter", "SendKeysWithTimeout",
			"ClaudeController.SendCommandImmediate", "ClaudeController.SendCommand",
			"SessionService.steerUnderLease", "SessionService.steerInternal", "SessionService.steerAuthorized",
			"SessionService.steerHiddenReviewViaBacklogLink", "SessionService.SteerActiveSession",
			"SessionService.SteerInstanceGuarded", "SessionService.SteerSessionGuarded",
		),
		leaseConstructors:  set("Instance.TryTerminalWriteLease", "Instance.AcquireTerminalWriteLease"),
		accessConstructors: set("AccessForUnary"),
		lifecycle:          set("Instance.Restart", "Instance.SwitchProgram", "Instance.SetAutoApprove", "Instance.changeDirectory"),
		exemptPkgs: []string{
			guardModulePath + "/session/tmux", guardModulePath + "/session/tymux", guardModulePath + "/session/mux",
		},
		exemptFiles: set("tmux_process_manager.go", "native_process_manager.go", "native_process_manager_windows.go",
			"tmux_backend.go", "backend_tymux.go", "instance_tmux.go"),
		exemptUnits: set("Instance.SendKeys", "Instance.SendKeysN", "Instance.SendPrompt", "Instance.TapEnter", "Instance.WriteToPTY",
			"SendKeysWithTimeout", "SubmitContentWithEnter", "SubmitDriverContent"),
	}
}

// callSite is one caller unit and what it calls.
type callSite struct {
	unit    string // "<pkg path relative to module>:<unit>"
	callees map[string]bool
}

// scanCallers returns, per caller unit, the set of watched callees (keys of
// the surface, lease constructors, access constructors and lifecycle sets).
func (c callerConfig) scanCallers(pkgs []*packages.Package, pkgFilter func(string) bool) map[string]*callSite {
	instMethods := c.surfaceMethodSignatures(pkgs)
	watched := func(key string) bool {
		return c.surface[key] || c.leaseConstructors[key] || c.accessConstructors[key] || c.lifecycle[key]
	}
	out := map[string]*callSite{}
	for _, p := range pkgs {
		if !pkgFilter(p.PkgPath) || c.pkgExempt(p.PkgPath) {
			continue
		}
		for _, f := range p.Syntax {
			file := p.Fset.Position(f.Pos()).Filename
			base := filepath.Base(file)
			if strings.HasSuffix(base, "_test.go") || c.exemptFiles[base] {
				continue
			}
			walkUnits(f, func(u unit, n ast.Node) {
				id, ok := n.(*ast.Ident)
				if !ok {
					return
				}
				key, ok := c.calleeKey(p.TypesInfo, id, instMethods)
				if !ok || !watched(key) {
					return
				}
				if c.exemptUnits[strings.SplitN(u.name, ".func", 2)[0]] && strings.HasSuffix(p.PkgPath, "/session") {
					return
				}
				name := strings.TrimPrefix(strings.TrimPrefix(p.PkgPath, guardModulePath), "/")
				if name == "" {
					name = "main"
				}
				unitKey := name + ":" + u.name
				cs := out[unitKey]
				if cs == nil {
					cs = &callSite{unit: unitKey, callees: map[string]bool{}}
					out[unitKey] = cs
				}
				cs.callees[key] = true
			})
		}
	}
	return out
}

func (c callerConfig) pkgExempt(path string) bool {
	for _, e := range c.exemptPkgs {
		if path == e || strings.HasPrefix(path, e+"/") {
			return true
		}
	}
	return false
}

// calleeKey names the function an identifier use resolves to. A call through an
// interface is matched by method name and signature against the watched
// concrete methods, so SessionAccessor.WriteToPTY is seen as Instance.WriteToPTY
// and PRNudger.SteerInstanceGuarded as SessionService.SteerInstanceGuarded.
func (c callerConfig) calleeKey(info *types.Info, id *ast.Ident, concrete map[string][]methodSig) (string, bool) {
	fn, ok := info.Uses[id].(*types.Func)
	if !ok || !isModulePkg(fn.Pkg()) {
		return "", false
	}
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil {
		return fn.Name(), true
	}
	recv := namedTypeName(sig.Recv().Type())
	if _, isIface := sig.Recv().Type().Underlying().(*types.Interface); isIface || recv == "" {
		if key, ok := processManagerKey(recv, fn.Name()); ok {
			return key, true
		}
		for _, m := range concrete[fn.Name()] {
			if types.Identical(stripRecv(m.sig), stripRecv(sig)) {
				return m.key, true
			}
		}
		return recv + "." + fn.Name(), true
	}
	return recv + "." + fn.Name(), true
}

// processManagerKey maps the typing methods of the session.ProcessManager
// interface onto the Instance primitives that wrap them, so a call through the
// interface from outside the PTY layer (the restart marker, changeDirectory) is
// seen as the primitive it is.
func processManagerKey(recv, name string) (string, bool) {
	if recv != "ProcessManager" {
		return "", false
	}
	switch name {
	case "SendKeys", "TapEnter":
		return "Instance." + name, true
	case "SendPromptWithEnter":
		return "Instance.SendPrompt", true
	}
	return "", false
}

func stripRecv(s *types.Signature) *types.Signature {
	return types.NewSignatureType(nil, nil, nil, s.Params(), s.Results(), s.Variadic())
}

// methodSig is a watched method's receiver type name and signature.
type methodSig struct {
	key string
	sig *types.Signature
}

// surfaceMethodSignatures collects, per method name, the signatures of every
// watched method on a module type, so an interface call is matched to the
// concrete method it dispatches to by name and signature.
func (c callerConfig) surfaceMethodSignatures(pkgs []*packages.Package) map[string][]methodSig {
	out := map[string][]methodSig{}
	for _, p := range pkgs {
		if !isModulePkg(p.Types) || p.Types == nil {
			continue
		}
		scope := p.Types.Scope()
		for _, n := range scope.Names() {
			tn, ok := scope.Lookup(n).(*types.TypeName)
			if !ok {
				continue
			}
			ms := types.NewMethodSet(types.NewPointer(tn.Type()))
			for i := 0; i < ms.Len(); i++ {
				fn, ok := ms.At(i).Obj().(*types.Func)
				if !ok {
					continue
				}
				key := n + "." + fn.Name()
				if c.surface[key] || c.leaseConstructors[key] || c.lifecycle[key] {
					out[fn.Name()] = append(out[fn.Name()], methodSig{key: key, sig: fn.Type().(*types.Signature)})
				}
			}
		}
	}
	return out
}

// diffUnits compares scanned units with the pinned rows, both directions.
func diffUnits(scanned map[string]*callSite, pinned map[string]callerRow, relevant func(*callSite) bool) []string {
	var out []string
	for unit, cs := range scanned {
		if !relevant(cs) {
			continue
		}
		if _, ok := pinned[unit]; !ok {
			out = append(out, "unlisted caller (add a row with a reason): "+unit+" calls "+joinKeys(cs.callees))
		}
	}
	for unit := range pinned {
		if cs, ok := scanned[unit]; !ok || !relevant(cs) {
			out = append(out, "stale row, no such caller: "+unit)
		}
	}
	sort.Strings(out)
	return out
}

func joinKeys(m map[string]bool) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}

func callsAny(cs *callSite, set map[string]bool) bool {
	for k := range cs.callees {
		if set[k] {
			return true
		}
	}
	return false
}

// T-RO-42
func TestPrimitiveCallers_ShouldEqualThePinnedTableInBothDirections_WhenScannedOverSessionServerDaemonCmdAndMain(t *testing.T) {
	cfg := realCallerConfig()
	scanned := cfg.scanCallers(realTreePackages(t), inModuleProduct)
	touchesSurface := func(cs *callSite) bool { return callsAny(cs, cfg.surface) || callsAny(cs, cfg.leaseConstructors) }
	for _, f := range diffUnits(scanned, pinnedCallers, touchesSurface) {
		t.Error(f)
	}
	for unit, row := range pinnedCallers {
		if row.reason == "" || row.kind == "" {
			t.Errorf("row %s needs a kind and a one-line reason", unit)
		}
	}
	t.Logf("check (c): %d scanned caller units, %d pinned rows", len(scanned), len(pinnedCallers))
}

// T-RO-45: the closed acquirer list.
func TestAcquirerList_ShouldBeClosedAndExcludeEveryLifecycleUnit_WhenCallersOfTryAndAcquireTerminalWriteLeaseAreScanned(t *testing.T) {
	cfg := realCallerConfig()
	scanned := cfg.scanCallers(realTreePackages(t), inModuleProduct)
	acquirers := map[string]callerRow{}
	for unit, row := range pinnedCallers {
		if row.kind == kindAcquirer || row.acquires {
			acquirers[unit] = row
		}
	}
	for _, f := range diffUnits(scanned, acquirers, func(cs *callSite) bool { return callsAny(cs, cfg.leaseConstructors) }) {
		t.Error(f)
	}
	for unit, row := range pinnedCallers {
		if row.kind == kindLifecycle && callsAny(scanned[unit], cfg.leaseConstructors) {
			t.Errorf("lifecycle unit %s must not acquire the write lease (Pause and Delete must not queue behind a wedged write)", unit)
		}
	}
}

// T-RO-46: lifecycle callers and AccessForUnary callers.
func TestLifecycleAndWrapperCallers_ShouldHaveOnlyTheListedCallers_WhenScanned(t *testing.T) {
	cfg := realCallerConfig()
	scanned := cfg.scanCallers(realTreePackages(t), inModuleProduct)
	for _, f := range diffUnits(scanned, pinnedLifecycleCallers, func(cs *callSite) bool { return callsAny(cs, cfg.lifecycle) }) {
		t.Error(f)
	}
	for _, f := range diffUnits(scanned, pinnedAccessCallers, func(cs *callSite) bool { return callsAny(cs, cfg.accessConstructors) }) {
		t.Error(f)
	}
}

// T-RO-44: the real tree passes with the table as written (the three tests
// above), and the same scan fails on a fixture with an unlisted caller, a stale
// row and a Pause-like acquirer.
func TestGuardSet_ShouldPassOnTheRealTreeWithTheTableAsWritten_AndFailWhenARowIsRemovedOrAnUnlistedCallerIsAdded_WhenScanned(t *testing.T) {
	cfg := fixtureCallerConfig()
	pkgs := allModulePackages(loadGuardPackages(t, "./server/services/testdata/terminalguard"))
	scanned := cfg.scanCallers(pkgs, inFixture)

	pinned := map[string]callerRow{
		"server/services/testdata/terminalguard:pinnedWriter": {kind: kindUI, reason: "fixture"},
		"server/services/testdata/terminalguard:goneCaller":   {kind: kindUI, reason: "fixture, stale"},
	}
	got := diffUnits(scanned, pinned, func(cs *callSite) bool { return callsAny(cs, cfg.surface) })
	if !containsSubstring(got, "unlisted caller (add a row with a reason): server/services/testdata/terminalguard:unlistedWriter") {
		t.Errorf("an unlisted caller was not flagged: %v", got)
	}
	if !containsSubstring(got, "stale row, no such caller: server/services/testdata/terminalguard:goneCaller") {
		t.Errorf("a stale row was not flagged: %v", got)
	}
	if containsSubstring(got, "pinnedWriter") {
		t.Errorf("a pinned caller was flagged: %v", got)
	}

	acq := diffUnits(scanned, map[string]callerRow{}, func(cs *callSite) bool { return callsAny(cs, cfg.leaseConstructors) })
	if !containsSubstring(acq, "pauseLike") {
		t.Errorf("a Pause-like unit that takes the lease was not flagged against the closed list: %v", acq)
	}
	acc := diffUnits(scanned, map[string]callerRow{}, func(cs *callSite) bool { return callsAny(cs, cfg.accessConstructors) })
	if !containsSubstring(acc, "streamSite") {
		t.Errorf("a stream site calling AccessForUnary was not flagged: %v", acc)
	}
}

func fixtureCallerConfig() callerConfig {
	c := realCallerConfig()
	c.surface = map[string]bool{"Inst.SendKeys": true, "Inst.WriteToPTY": true}
	c.leaseConstructors = map[string]bool{"Inst.TryTerminalWriteLease": true}
	c.accessConstructors = map[string]bool{"AccessForUnary": true}
	c.lifecycle = map[string]bool{}
	c.exemptUnits = map[string]bool{}
	return c
}
