package services

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// surfaceID names one user-settable config surface, e.g. "quota.enabled",
// "flag:backlog", or "proto.CreateStageRequest.name".
type surfaceID string

// proofEntry says which test proves a surface changes runtime behavior, or why
// it needs none (exempt, with a written reason).
type proofEntry struct {
	testName   string
	entrypoint string // production entry point the proof drives (never the engine directly)
	exempt     string
}

// pendingSurface is a surface that has no proof yet. planned means the surface
// does not exist in code yet and is allowed to be absent until its story lands.
type pendingSurface struct {
	story   string
	planned bool
}

// assertConfigChangesBehavior is the shared proof helper: it runs observe with
// each setting value and fails unless the observable differs. Every proof test
// named in the registry must call it.
func assertConfigChangesBehavior(t testing.TB, observe func(setting any) any, before, after any) {
	t.Helper()
	b, a := observe(before), observe(after)
	if reflect.DeepEqual(b, a) {
		t.Fatalf("setting %v -> %v did not change the observable (%v): config has no runtime effect", before, after, b)
	}
}

func quotaSurfaces() []surfaceID {
	typ := reflect.TypeOf(config.QuotaConfig{})
	out := make([]surfaceID, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			tag = typ.Field(i).Name
		}
		out = append(out, surfaceID("quota."+tag))
	}
	return out
}

func featureFlagSurfaces() []surfaceID {
	out := make([]surfaceID, 0, len(knownFeatureFlags))
	for _, f := range knownFeatureFlags {
		out = append(out, surfaceID("flag:"+f.name))
	}
	return out
}

// stageConfigRequests are the stage-config CRUD write messages whose fields
// are user-settable.
var stageConfigRequests = []proto.Message{
	&sessionv1.CreateStageRequest{}, &sessionv1.UpdateStageRequest{},
	&sessionv1.CreateStageTransitionRequest{}, &sessionv1.UpdateStageTransitionRequest{},
	&sessionv1.CreateTransitionGateRequest{}, &sessionv1.UpdateTransitionGateRequest{},
}

func protoFieldSurfaces(msgs ...proto.Message) []surfaceID {
	var out []surfaceID
	for _, m := range msgs {
		d := m.ProtoReflect().Descriptor()
		for i := 0; i < d.Fields().Len(); i++ {
			out = append(out, surfaceID(fmt.Sprintf("proto.%s.%s", d.Name(), d.Fields().Get(i).Name())))
		}
	}
	return out
}

// allConfigSurfaces enumerates every surface that exists in code today.
func allConfigSurfaces() []surfaceID {
	out := quotaSurfaces()
	out = append(out, featureFlagSurfaces()...)
	out = append(out, protoFieldSurfaces(stageConfigRequests...)...)
	// Auto-merge status is a single output field on the stuck-item message.
	out = append(out, "proto.StuckBacklogItem.allow_auto_merge")
	return out
}

// testFuncBodies parses every _test.go file in dir and maps func name to its body.
func testFuncBodies(t testing.TB, dir string) map[string]*ast.BlockStmt {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.BlockStmt{}
	for _, f := range files {
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range parsed.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil {
				out[fn.Name.Name] = fn.Body
			}
		}
	}
	return out
}

func callsHelper(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "assertConfigChangesBehavior" {
				found = true
			}
		}
		return !found
	})
	return found
}

// validateConfigSurfaces returns every violation of the proof contract, sorted.
func validateConfigSurfaces(surfaces []surfaceID, registry map[surfaceID]proofEntry, pending map[surfaceID]pendingSurface, tests map[string]*ast.BlockStmt) []string {
	var errs []string
	exists := make(map[surfaceID]bool, len(surfaces))
	for _, s := range surfaces {
		exists[s] = true
		_, registered := registry[s]
		_, isPending := pending[s]
		switch {
		case registered && isPending:
			errs = append(errs, fmt.Sprintf("ConfigSurface %s: remove stale pending entry (it has a ConfigSurfaceProof)", s))
		case !registered && !isPending:
			errs = append(errs, fmt.Sprintf("ConfigSurface %s has no ConfigSurfaceProof", s))
		}
	}
	for s, p := range pending {
		if !exists[s] && !p.planned {
			errs = append(errs, fmt.Sprintf("ConfigSurface %s: pending entry names a surface that no longer exists", s))
		}
	}
	for s, e := range registry {
		if !exists[s] {
			errs = append(errs, fmt.Sprintf("ConfigSurface %s: ConfigSurfaceProof names a surface that does not exist", s))
		}
		if e.testName == "" {
			if e.exempt == "" {
				errs = append(errs, fmt.Sprintf("ConfigSurface %s: exempt requires a reason", s))
			}
			continue
		}
		body, ok := tests[e.testName]
		switch {
		case !ok:
			errs = append(errs, fmt.Sprintf("ConfigSurface %s: proof test not found: %s", s, e.testName))
		case !callsHelper(body):
			errs = append(errs, fmt.Sprintf("ConfigSurface %s: proof test %s never calls assertConfigChangesBehavior", s, e.testName))
		case e.entrypoint == "":
			errs = append(errs, fmt.Sprintf("ConfigSurface %s: proof entry for %s names no production entrypoint", s, e.testName))
		}
	}
	sort.Strings(errs)
	return errs
}

func TestConfigSurfaceProofMeta_FailsOnMissingEntryUnknownTestNoAssertCallAndEmptyExempt(t *testing.T) {
	t.Parallel()
	src := `package x
func TestGood(t *testing.T) { assertConfigChangesBehavior(t, nil, 1, 2) }
func TestNoCall(t *testing.T) {}`
	f, err := parser.ParseFile(token.NewFileSet(), "x_test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]*ast.BlockStmt{}
	for _, d := range f.Decls {
		fn := d.(*ast.FuncDecl)
		tests[fn.Name.Name] = fn.Body
	}
	cases := []struct {
		name     string
		surfaces []surfaceID
		registry map[surfaceID]proofEntry
		pending  map[surfaceID]pendingSurface
		want     string
	}{
		{"no entry", []surfaceID{"quota.new_knob"}, nil, nil, "ConfigSurface quota.new_knob has no ConfigSurfaceProof"},
		{"unknown test", []surfaceID{"a"}, map[surfaceID]proofEntry{"a": {testName: "TestDoesNotExist", entrypoint: "E"}}, nil, "proof test not found"},
		{"no helper call", []surfaceID{"a"}, map[surfaceID]proofEntry{"a": {testName: "TestNoCall", entrypoint: "E"}}, nil, "never calls assertConfigChangesBehavior"},
		{"empty exempt", []surfaceID{"a"}, map[surfaceID]proofEntry{"a": {}}, nil, "exempt requires a reason"},
		{"no entrypoint", []surfaceID{"a"}, map[surfaceID]proofEntry{"a": {testName: "TestGood"}}, nil, "no production entrypoint"},
		{"stale pending", []surfaceID{"a"}, map[surfaceID]proofEntry{"a": {exempt: "display only"}}, map[surfaceID]pendingSurface{"a": {}}, "remove stale pending entry"},
		{"pending gone", nil, nil, map[surfaceID]pendingSurface{"a": {}}, "no longer exists"},
	}
	for _, tc := range cases {
		got := strings.Join(validateConfigSurfaces(tc.surfaces, tc.registry, tc.pending, tests), "\n")
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: want error containing %q, got %q", tc.name, tc.want, got)
		}
	}
	ok := validateConfigSurfaces([]surfaceID{"a", "b"},
		map[surfaceID]proofEntry{"a": {testName: "TestGood", entrypoint: "E"}, "b": {exempt: "display only"}},
		map[surfaceID]pendingSurface{"future": {planned: true}}, tests)
	if len(ok) != 0 {
		t.Errorf("valid registry reported errors: %v", ok)
	}
}

func TestAssertConfigChangesBehavior_PassesWhenObservableDiffers(t *testing.T) {
	t.Parallel()
	assertConfigChangesBehavior(t, func(v any) any { return v.(int) > 5 }, 1, 10)
}
