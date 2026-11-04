package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeGraph is a tiny repo: //session and //config packages, //web-app, with explicit rdeps.
type fakeGraph struct {
	pkgs    []string
	files   map[string]bool
	rdeps   map[string][]string // seed -> test targets
	failAll bool
}

func (f fakeGraph) PackageOf(p string) (string, bool) {
	for _, pk := range f.pkgs {
		if strings.HasPrefix(p, pk+"/") {
			return "//" + pk, true
		}
	}
	return "", false
}
func (f fakeGraph) InGraph(l string) bool { return f.files[l] }
func (f fakeGraph) Tests(seeds []string) ([]string, error) {
	if f.failAll {
		return nil, errors.New("query failed")
	}
	var out []string
	for _, s := range seeds {
		out = append(out, f.rdeps[s]...)
	}
	return out, nil
}

var rules = Rules{
	Global:   []string{"MODULE.bazel", "go.mod", "go.sum", "tools/bzl/**"},
	NonBazel: []NonBazelInput{{"tests/e2e/**", []string{"e2e"}}, {"docs/registry/**", []string{"registry"}}},
}

func graph() fakeGraph {
	return fakeGraph{
		pkgs:  []string{"session", "config", "web-app/src"},
		files: map[string]bool{"//session:health.go": true, "//config:config.go": true, "//web-app/src:app.tsx": true},
		rdeps: map[string][]string{
			"//session:health.go":   {"//session:session_test", "//server/services:services_test"},
			"//session:all":         {"//session:session_test", "//server/services:services_test"},
			"//web-app/src:app.tsx": {"//web-app:jest"},
		},
	}
}

func sel(cs ...Change) Result { return Select(cs, rules, graph()) }

func TestPureFunctionSelectsRdepsTestsNoWeb(t *testing.T) {
	r := sel(Change{"session/health.go", Modified})
	want := []string{"//server/services:services_test", "//session:session_test"}
	if !reflect.DeepEqual(r.Targets, want) {
		t.Fatalf("got %v want %v", r.Targets, want)
	}
	for _, x := range r.Targets {
		if strings.HasPrefix(x, "//web-app") {
			t.Fatalf("unexpected web target %s", x)
		}
	}
}

func TestGlobalInputsForceEverything(t *testing.T) {
	for _, f := range []string{"MODULE.bazel", "go.mod", "tools/bzl/deep/x.bzl"} {
		if r := sel(Change{"session/health.go", Modified}, Change{f, Modified}); !reflect.DeepEqual(r.Targets, []string{"//..."}) {
			t.Fatalf("%s: %v", f, r.Targets)
		}
	}
}

func TestDeletedSelectsPackageRdeps(t *testing.T) {
	r := sel(Change{"session/old.go", Deleted})
	if len(r.Targets) != 2 || r.Targets[1] != "//session:session_test" {
		t.Fatalf("got %v", r.Targets)
	}
}

func TestDeletedPackageGoneSelectsAll(t *testing.T) {
	if r := sel(Change{"gone/old.go", Deleted}); !reflect.DeepEqual(r.Targets, []string{"//..."}) {
		t.Fatalf("got %v", r.Targets)
	}
}

func TestRenameDeletesOldAndAddsNew(t *testing.T) {
	cs, err := ParseNameStatus(strings.NewReader("R100\tsession/a.go\tsession/health.go\nA\tconfig/config.go\n"))
	if err != nil || len(cs) != 3 || cs[0].Status != Deleted || cs[1].Path != "session/health.go" {
		t.Fatalf("%v %v", cs, err)
	}
	if r := Select(cs, rules, graph()); len(r.Targets) == 0 || r.Targets[0] == "//..." {
		t.Fatalf("got %v", r.Targets)
	}
}

func TestNonGraphPathSelectsAll(t *testing.T) {
	if r := sel(Change{"random/unmapped.txt", Modified}); !reflect.DeepEqual(r.Targets, []string{"//..."}) {
		t.Fatalf("got %v", r.Targets)
	}
}

func TestNonBazelInputMapsJobs(t *testing.T) {
	r := sel(Change{"tests/e2e/a.spec.ts", Modified}, Change{"docs/registry/features/x.json", Modified})
	if len(r.Targets) != 0 || !reflect.DeepEqual(r.Jobs, []string{"e2e", "registry"}) {
		t.Fatalf("got %+v", r)
	}
}

func TestUnreferencedFileInPackageFallsBackToPackage(t *testing.T) {
	r := sel(Change{"session/NOTES.md", Modified})
	if len(r.Targets) != 2 {
		t.Fatalf("got %v", r.Targets)
	}
}

func TestWebChangeAddsE2EPrereqs(t *testing.T) {
	r := sel(Change{"web-app/src/app.tsx", Modified})
	for _, want := range []string{"//:stapler-squad", "//web-app:dist", "//web-app:jest"} {
		if !contains(r.Targets, want) {
			t.Fatalf("missing %s in %v", want, r.Targets)
		}
	}
}

func TestQueryErrorWidensNeverFails(t *testing.T) {
	g := graph()
	g.failAll = true
	if r := Select([]Change{{"session/health.go", Modified}}, rules, g); !reflect.DeepEqual(r.Targets, []string{"//..."}) {
		t.Fatalf("got %v", r.Targets)
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		g, p string
		ok   bool
	}{
		{"tools/bzl/**", "tools/bzl/a/b.bzl", true},
		{"tools/bzl/**", "tools/other", false},
		{"**/BUILD.bazel", "BUILD.bazel", true},
		{"**/BUILD.bazel", "a/b/BUILD.bazel", true},
		{"*.go", "a/b.go", false},
		{".bazelrc*", ".bazelrc.ci", true},
	}
	for _, c := range cases {
		if GlobMatch(c.g, c.p) != c.ok {
			t.Errorf("GlobMatch(%q,%q) != %v", c.g, c.p, c.ok)
		}
	}
}

func TestParseNonBazelRejectsMalformed(t *testing.T) {
	if _, err := ParseNonBazel(strings.NewReader("only-one-field\n")); err == nil {
		t.Fatal("want error")
	}
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
