package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// fixture: app -> svc -> core (library edges); core's tests import svc (a layering inversion).
func fixture() *Model {
	return &Model{
		Lib:     map[string]map[string]bool{"core": {}, "svc": set("core"), "app": set("svc"), "leaf": {}},
		Test:    map[string]map[string]bool{"core": set("svc"), "svc": {}, "app": {}, "leaf": {}},
		HasTest: set("core", "svc", "leaf"),
		LOC:     map[string]int{"core": 100, "svc": 100, "app": 100, "leaf": 100},
		TestLOC: map[string]int{"core": 50, "svc": 50, "leaf": 50},
	}
}

func TestTestOnlyImportDoesNotEnlargeDependentsInTargetModel(t *testing.T) {
	m := fixture()
	libs, _ := m.Affected(set("core"), nil)
	if libs["leaf"] || len(libs) != 3 {
		t.Fatalf("library closure of core = %v, want core,svc,app", libs)
	}
	// a change in svc must not invalidate core's library, only core's test target
	libs, tests := m.Affected(set("svc"), nil)
	if libs["core"] || !tests["core"] {
		t.Fatalf("svc change: libs=%v tests=%v, want core test but not core lib", libs, tests)
	}
}

func TestMergedModelLetsTestEdgesPropagate(t *testing.T) {
	m := fixture()
	if got := m.MergedShare(set("svc")); got != 0.75 { // svc, app, core (via core's test edge)
		t.Fatalf("merged share = %v, want 0.75", got)
	}
}

func TestInversions(t *testing.T) {
	inv := fixture().Inversions()
	if len(inv) != 1 || inv[0] != (Inversion{Pkg: "core", Imported: "svc"}) {
		t.Fatalf("inversions = %v", inv)
	}
	m := fixture()
	m.Test["core"] = set()
	m.Lib["inttest"] = set("core", "svc")
	m.Test["inttest"] = set("core", "svc")
	m.HasTest["inttest"] = true
	if inv := m.Inversions(); len(inv) != 0 {
		t.Fatalf("moving the inverted test into a leaf package must clear it, got %v", inv)
	}
}

func TestReplayFollowsRenames(t *testing.T) {
	m := fixture()
	m.Lib["svc/sub"] = set("core")
	m.Test["svc/sub"] = set()
	m.LOC["svc/sub"] = 10
	changes := []Change{{ID: "a", Paths: []string{"svc/x.go"}}, {ID: "b", Paths: []string{"README.md"}}}
	plain := Replay(m, Resolver{Model: m}, changes)
	moved := Replay(m, Resolver{Model: m, Renames: map[string]string{"svc/x.go": "svc/sub/x.go"}}, changes)
	if plain.NChanges != 1 || moved.NChanges != 1 {
		t.Fatalf("n = %d/%d, want 1/1 (non-Go change ignored)", plain.NChanges, moved.NChanges)
	}
	if moved.LibLOCByChange["a"] >= plain.LibLOCByChange["a"] {
		t.Fatalf("a moved leaf must invalidate less: %d vs %d", moved.LibLOCByChange["a"], plain.LibLOCByChange["a"])
	}
	if got := LowerShare(map[string]int{"a": 1000}, map[string]int{"a": 995}, 100000); got != 0 {
		t.Fatalf("a 5-line drop is below the materiality threshold, got %v", got)
	}
	if plain.UniversalTouch != 0 {
		t.Fatalf("svc is not a universal package")
	}
	if got := LowerShare(plain.LibLOCByChange, moved.LibLOCByChange, total(m.LOC)); got != 1 {
		t.Fatalf("lower share = %v", got)
	}
}

func TestUniversalTouchCountsOnlyGoProduction(t *testing.T) {
	m := &Model{
		Lib: map[string]map[string]bool{"session": {}, "server/web": {}}, Test: map[string]map[string]bool{"session": {}, "server/web": {}},
		HasTest: set(), LOC: map[string]int{"session": 1, "server/web": 1}, TestLOC: map[string]int{},
	}
	changes := []Change{{ID: "a", Paths: []string{"session/a.go"}}, {ID: "b", Paths: []string{"web-app/x.tsx"}}}
	if got := Replay(m, Resolver{Model: m}, changes).UniversalTouch; got != 1 {
		t.Fatalf("universal touch = %v, want 1 (web-app commit is not a Go production commit)", got)
	}
}

func TestParseGitLogAndRenames(t *testing.T) {
	cs := ParseGitLog(strings.NewReader("@@abc\na.go\nb/c.go\n\n@@def\nd.go\n"))
	if len(cs) != 2 || len(cs[0].Paths) != 2 || cs[1].ID != "def" {
		t.Fatalf("parsed %+v", cs)
	}
	r := ParseRenames("R100\told/a.go\tnew/a.go\nM\tx.go\n")
	if len(r) != 1 || r["old/a.go"] != "new/a.go" {
		t.Fatalf("renames %v", r)
	}
}

func TestBuildModelDropsGeneratedAndExternal(t *testing.T) {
	js := `{"ImportPath":"github.com/tstapler/stapler-squad/a","Dir":"/nonexistent","GoFiles":["a.go"],"TestGoFiles":["a_test.go"],
"Imports":["github.com/tstapler/stapler-squad/gen/proto","github.com/tstapler/stapler-squad/b","fmt"]}
{"ImportPath":"github.com/tstapler/stapler-squad/b","Dir":"/nonexistent"}
{"ImportPath":"github.com/tstapler/stapler-squad/gen/proto"}`
	pkgs, err := decodeList(bytes.NewBufferString(js))
	if err != nil {
		t.Fatal(err)
	}
	m := BuildModel(pkgs)
	if len(m.Lib) != 2 || !m.Lib["a"]["b"] || m.Lib["a"]["gen/proto"] || !m.HasTest["a"] {
		t.Fatalf("model = %+v", m)
	}
}

func TestInversionsSubcommandExitCodes(t *testing.T) {
	var out bytes.Buffer
	if code := runInversions([]string{"-root", "/definitely/not/a/repo"}, &out); code != 1 {
		t.Fatalf("exit %d for an unloadable root, want 1", code)
	}
}

func TestPrintT9(t *testing.T) {
	good := &Report{
		T9aTestLOCShare500: 0.44, T9bUniversalTouch500: 0.58, T9cMeanLibLOCShare: 0.385, T9cChangesWithLowerLib: 0.148,
		T9dServicesLOC: 67000, T9dSessionLOC: 68000, T9eTargetModelShare: 0.07,
	}
	var out bytes.Buffer
	if !PrintT9(&out, good, DefaultT9Targets, T9fLine(t.TempDir())) || strings.Count(out.String(), ": PASS") != 5 || !strings.Contains(out.String(), "T9f: UNMEASURED") {
		t.Fatalf("good report: %s", out.String())
	}
	bad := *good
	bad.T9dTestInversions = 12
	out.Reset()
	if PrintT9(&out, &bad, DefaultT9Targets, T9fLine(t.TempDir())) || !strings.Contains(out.String(), "T9d: FAIL") {
		t.Fatalf("inversions must fail T9d: %s", out.String())
	}
}

func TestRatchet(t *testing.T) {
	found := []Inversion{{"core", "svc"}, {"a", "b"}}
	allowed := ParseAllowlist("core svc # known\n\nx y\n")
	violations, stale := Ratchet(found, allowed)
	if len(violations) != 1 || violations[0] != (Inversion{"a", "b"}) {
		t.Fatalf("violations = %v, want the new a->b inversion", violations)
	}
	if len(stale) != 1 || stale[0] != (Inversion{"x", "y"}) {
		t.Fatalf("stale = %v, want the resolved x->y entry", stale)
	}
	if v, s := Ratchet(found, ParseAllowlist("core svc\na b")); len(v)+len(s) != 0 {
		t.Fatalf("exact allowlist must pass, got %v %v", v, s)
	}
}

func TestT9fLine(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if got := T9fLine(root); !strings.HasPrefix(got, "T9f: UNMEASURED") {
		t.Fatalf("no attestations file: %s", got)
	}
	path := filepath.Join(root, attestationsRel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("adr_009_e2e:\n  decision: Selective e2e\n  date: 2026-11-05\n")
	if got := T9fLine(root); !strings.HasPrefix(got, "T9f: UNMEASURED") {
		t.Fatalf("other decision must stay UNMEASURED: %s", got)
	}
	write("other:\n  decision: Full e2e run\nadr_009_e2e:\n  decision: Full e2e run\n  date: 2026-11-05\n")
	if got := T9fLine(root); !strings.HasPrefix(got, "T9f: N/A (owner decision 2026-11-05: Full e2e run") {
		t.Fatalf("full e2e run must be N/A: %s", got)
	}
	write("other:\n  decision: Full e2e run\n")
	if got := T9fLine(root); !strings.HasPrefix(got, "T9f: UNMEASURED") {
		t.Fatalf("decision under another section must not count: %s", got)
	}
}
