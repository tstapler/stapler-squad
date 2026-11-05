package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const mod = modulePath

func fixtureManifest() *Manifest {
	return &Manifest{GeneratedAt: "abc", Specs: []Entry{
		{Spec: "tests/e2e/alias.spec.ts", GoPkgs: []string{mod + "/session", mod + "/server/services"}, WebFiles: []string{"web-app/src/a.tsx"}},
		{Spec: "tests/e2e/perf.spec.ts", Excluded: "perf project runs only in nightly"},
		{Spec: "tests/e2e/toast.spec.ts", GoPkgs: []string{mod + "/server/services"}, WebFiles: []string{"web-app/src/toast.tsx"}},
	}}
}

var fixtureSpecs = []string{"tests/e2e/alias.spec.ts", "tests/e2e/perf.spec.ts", "tests/e2e/toast.spec.ts"}
var fixtureGlobals = []string{"go.mod", "go.sum", "proto/**", "tests/e2e/helpers/**", "tests/e2e/playwright.config.ts"}

func TestCheckManifest(t *testing.T) {
	if p := checkManifest(fixtureManifest(), fixtureSpecs); len(p) != 0 {
		t.Fatalf("valid manifest rejected: %v", p)
	}
	cases := map[string]func(m *Manifest) []string{
		"unmanaged": func(m *Manifest) []string { return append(fixtureSpecs, "tests/e2e/new.spec.ts") },
		"duplicate": func(m *Manifest) []string { m.Specs = append(m.Specs, m.Specs[2]); return fixtureSpecs },
		"empty go_pkgs": func(m *Manifest) []string {
			m.Specs[2].GoPkgs = nil
			return fixtureSpecs
		},
		"unsorted": func(m *Manifest) []string { m.Specs[0], m.Specs[1] = m.Specs[1], m.Specs[0]; return fixtureSpecs },
		"no_server needs reason": func(m *Manifest) []string {
			m.Specs[2].GoPkgs, m.Specs[2].NoServer = nil, true
			return fixtureSpecs
		},
		"removed spec": func(m *Manifest) []string { return fixtureSpecs[:2] },
		"no commit":    func(m *Manifest) []string { m.GeneratedAt = ""; return fixtureSpecs },
	}
	for name, mutate := range cases {
		m := fixtureManifest()
		specs := mutate(m)
		if len(checkManifest(m, specs)) == 0 {
			t.Errorf("%s: not rejected", name)
		}
	}
}

func TestMergeUnionsRunsAndSorts(t *testing.T) {
	m := merge("sha", []Entry{
		{Spec: "b.spec.ts", GoPkgs: []string{"z", "a"}, Routes: []string{"/x"}},
		{Spec: "a.spec.ts", GoPkgs: []string{"q"}},
		{Spec: "b.spec.ts", GoPkgs: []string{"a", "m"}, Routes: []string{"/y"}},
	})
	if m.Specs[0].Spec != "a.spec.ts" || !reflect.DeepEqual(m.Specs[1].GoPkgs, []string{"a", "m", "z"}) || !reflect.DeepEqual(m.Specs[1].Routes, []string{"/x", "/y"}) {
		t.Fatalf("bad merge: %+v", m.Specs)
	}
}

func TestSelect(t *testing.T) {
	defer func(v int) { maxGroup = v }(maxGroup)
	maxGroup = 1 // one spec per group so selection is observable per spec
	s := newSelector(fixtureManifest(), fixtureGlobals, append(fixtureSpecs, "tests/e2e/fresh.spec.ts"), nil)
	cases := []struct {
		name    string
		changed []string
		all     bool
		has     []string
		hasNot  []string
	}{
		{"core file selects both", []string{"server/services/x.go"}, false, []string{"alias.spec.ts", "toast.spec.ts"}, nil},
		{"leaf file selects one", []string{"session/instance.go"}, false, []string{"alias.spec.ts"}, []string{"toast.spec.ts"}},
		{"web file", []string{"web-app/src/toast.tsx"}, false, []string{"toast.spec.ts"}, []string{"alias.spec.ts"}},
		{"go.mod is global", []string{"go.mod"}, true, nil, nil},
		{"proto is global", []string{"proto/session/v1/session.proto"}, true, nil, nil},
		{"helper is global", []string{"tests/e2e/helpers/wait.ts"}, true, nil, nil},
		{"deleted file selects by manifest path", []string{"web-app/src/a.tsx"}, false, []string{"alias.spec.ts"}, []string{"toast.spec.ts"}},
		{"docs only: only always-run specs", []string{"docs/how-to/x.md"}, false, []string{"perf.spec.ts", "fresh.spec.ts"}, []string{"alias.spec.ts", "toast.spec.ts"}},
		{"test file is not prod", []string{"session/instance_test.go"}, false, nil, []string{"alias.spec.ts"}},
		{"unobserved package", []string{"github/client.go"}, false, nil, []string{"alias.spec.ts", "toast.spec.ts"}},
		{"embedded asset in observed dir", []string{"session/schema.sql"}, false, []string{"alias.spec.ts"}, []string{"toast.spec.ts"}},
		{"spec itself", []string{"tests/e2e/toast.spec.ts"}, false, []string{"toast.spec.ts"}, []string{"alias.spec.ts"}},
	}
	for _, c := range cases {
		got := s.Select(c.changed)
		if got.All != c.all {
			t.Errorf("%s: All=%v want %v", c.name, got.All, c.all)
		}
		in := strings.Join(got.Specs, " ")
		for _, h := range c.has {
			if !strings.Contains(in, h) {
				t.Errorf("%s: %s not selected (%v)", c.name, h, got.Specs)
			}
		}
		for _, h := range c.hasNot {
			if strings.Contains(in, h) {
				t.Errorf("%s: %s selected unexpectedly", c.name, h)
			}
		}
	}
	if u := s.Select(nil).Unmanaged; !reflect.DeepEqual(u, []string{"tests/e2e/fresh.spec.ts"}) {
		t.Errorf("unmanaged = %v", u)
	}
}

func TestStaticClosureWidensSelection(t *testing.T) {
	g := parseGraph(mod + "/server " + mod + "/session fmt\n" + mod + "/session " + mod + "/config\n" + mod + "/config\n")
	m := &Manifest{GeneratedAt: "x", Specs: []Entry{{Spec: "tests/e2e/a.spec.ts", GoPkgs: []string{mod + "/server"}}}}
	narrow := newSelector(m, nil, []string{"tests/e2e/a.spec.ts"}, nil).Select([]string{"config/c.go"})
	wide := newSelector(m, nil, []string{"tests/e2e/a.spec.ts"}, g.Closure).Select([]string{"config/c.go"})
	if len(narrow.Specs) != 0 || len(wide.Specs) != 1 {
		t.Fatalf("narrow=%v wide=%v", narrow.Specs, wide.Specs)
	}
}

func TestGroupBounds(t *testing.T) {
	var specs []string
	for _, f := range []struct {
		n string
		c int
	}{{"backlog", 29}, {"session", 19}, {"scroll", 13}} {
		for i := 0; i < f.c; i++ {
			specs = append(specs, fmt.Sprintf("tests/e2e/%s-%02d.spec.ts", f.n, i))
		}
	}
	for i := 0; i < 40; i++ {
		specs = append(specs, fmt.Sprintf("tests/e2e/solo%02d.spec.ts", i))
	}
	seen := map[string]bool{}
	for _, g := range groupSpecs(specs) {
		if len(g.Specs) > maxGroup {
			t.Errorf("%s has %d specs", g.Name, len(g.Specs))
		}
		for _, s := range g.Specs {
			if seen[s] {
				t.Errorf("%s in two groups", s)
			}
			seen[s] = true
		}
	}
	if len(seen) != len(specs) {
		t.Errorf("assigned %d of %d", len(seen), len(specs))
	}
}

func TestPlanShardsBalanced(t *testing.T) {
	d := map[string]float64{}
	var ts []string
	for i := 0; i < 24; i++ {
		n := fmt.Sprintf("t%02d", i)
		ts = append(ts, n)
		d[n] = float64(100 + (i*37)%400)
	}
	ts = append(ts, "unknown")
	sh := planShards(ts, d, 5)
	var sum, max float64
	count := 0
	for _, s := range sh {
		sum += s.Seconds
		max = maxf(max, s.Seconds)
		count += len(s.Targets)
	}
	if count != len(ts) {
		t.Fatalf("assigned %d of %d", count, len(ts))
	}
	if mean := sum / 5; max > 1.25*mean {
		t.Fatalf("max %.0f > 1.25 x mean %.0f", max, mean)
	}
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func TestNoBinaryDep(t *testing.T) {
	if len(checkNoBinaryDep("//session:session\n//tests/e2e:alias\n")) != 0 {
		t.Fatal("clean graph flagged")
	}
	if len(checkNoBinaryDep("//session\n//:stapler-squad\n//server/web:dist\n")) != 2 {
		t.Fatal("seeded binary dependency not flagged")
	}
}

func TestGenBuildDeterministicAndExcludesBinary(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"session/BUILD.bazel", "web-app/BUILD.bazel", "web-app/src/BUILD.bazel"} {
		mustWrite(t, filepath.Join(root, p), "")
	}
	m := fixtureManifest()
	m.Specs = append(m.Specs, Entry{Spec: "tests/e2e/root.spec.ts", GoPkgs: []string{mod, mod + "/server/web", mod + "/session"}})
	a := genBuild(root, m, fixtureGlobals, map[string]bool{"tests/e2e/toast.spec.ts": true})
	if b := genBuild(root, m, fixtureGlobals, map[string]bool{"tests/e2e/toast.spec.ts": true}); a != b {
		t.Fatal("not deterministic")
	}
	for _, want := range []string{`"//session"`, `"//web-app/src:a.tsx"`, `tags = ["no-cache"]`, "# gazelle:ignore", `"helpers/**"`} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %q in:\n%s", want, a)
		}
	}
	if strings.Contains(a, `"//server/web`) || strings.Contains(a, `"//:`) || strings.Contains(a, "stapler-squad") {
		t.Errorf("binary seam leaked into the slice:\n%s", a)
	}
}

func TestSpecFeatureIDs(t *testing.T) {
	src := "// @feature session:create, alias:invoke\nFEATURE_CATALOG['backlog-get-item'],\n  // FEATURE_CATALOG['commented'],\n"
	if got := specFeatureIDs(src); !reflect.DeepEqual(got, []string{"alias:invoke", "backlog-get-item", "session:create"}) {
		t.Fatalf("%v", got)
	}
}

func TestRegistryCoverage(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "tests/e2e/a.spec.ts"), "// @feature known:one, gone:two\n")
	mustWrite(t, filepath.Join(root, "tests/e2e/b.spec.ts"), "// @feature gone:three\n")
	mustWrite(t, filepath.Join(root, "docs/registry/features/k.json"), `{"id":"known:one","handlerFile":"server/h.go"}`)
	mustWrite(t, filepath.Join(root, "server/h.go"), "package server")
	idx, err := loadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := registryCoverage(root, []string{"tests/e2e/a.spec.ts", "tests/e2e/b.spec.ts"}, idx)
	if err != nil {
		t.Fatal(err)
	}
	if c.Specs != 2 || c.WithResolvableID != 1 || c.AllIDsResolve != 0 || c.WithExistingFile != 1 || c.WithGoFile != 1 || !reflect.DeepEqual(c.UnresolvedIDs, []string{"gone:three", "gone:two"}) {
		t.Fatalf("%+v", c)
	}
}

func TestReplayOnTempRepo(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		c := exec.Command("git", append([]string{"-c", "user.email=a@b", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	commit := func(file string) {
		mustWrite(t, filepath.Join(root, file), file)
		git("add", file)
		git("commit", "-q", "-m", file)
	}
	commit("docs/a.md")
	commit("session/a.go")
	commit("github/b.go")
	commit("go.mod")
	mustWrite(t, filepath.Join(root, "tests/e2e/alias.spec.ts"), "")
	mustWrite(t, filepath.Join(root, "tests/e2e/toast.spec.ts"), "")
	mustWrite(t, filepath.Join(root, "tests/e2e/perf.spec.ts"), "")
	m := fixtureManifest()
	s := newSelector(m, fixtureGlobals, fixtureSpecs, nil)
	cs, err := gitLog(root, "main", 100)
	if err != nil {
		t.Fatal(err)
	}
	st := replay(cs, s.Select)
	// relevant: session/a.go (alias group), github/b.go (nothing beyond always-run) ; go.mod is not source
	if st.Commits != 4 || st.Relevant != 2 || st.ByTopDir["session"] != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestRunCheckManifestCLI(t *testing.T) {
	root := t.TempDir()
	for _, s := range fixtureSpecs {
		mustWrite(t, filepath.Join(root, s), "")
	}
	b, _ := json.Marshal(fixtureManifest())
	mustWrite(t, filepath.Join(root, manifestPath), string(b))
	var out, errw bytes.Buffer
	if code := run([]string{"check-manifest", "-root", root}, nil, &out, &errw); code != 0 {
		t.Fatalf("code %d: %s", code, errw.String())
	}
	mustWrite(t, filepath.Join(root, "tests/e2e/new.spec.ts"), "")
	errw.Reset()
	if code := run([]string{"check-manifest", "-root", root}, nil, &out, &errw); code != 1 || !strings.Contains(errw.String(), "unmanaged") {
		t.Fatalf("code %d: %s", code, errw.String())
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGlobs(t *testing.T) {
	for g, f := range map[string]string{"proto/**": "proto/a/b.proto", "tests/e2e/helpers/**": "tests/e2e/helpers/x.ts", "go.*": "go.mod", "MODULE.bazel*": "MODULE.bazel.lock"} {
		if !matchesAny([]string{g}, f) {
			t.Errorf("%s !~ %s", g, f)
		}
	}
	if matchesAny([]string{"go.mod"}, "xgo.mod") || matchesAny([]string{"proto/**"}, "protos/a") {
		t.Error("false match")
	}
}

func TestListGroupsAndSpecsOfCLI(t *testing.T) {
	root := t.TempDir()
	for _, s := range fixtureSpecs {
		mustWrite(t, filepath.Join(root, s), "")
	}
	b, _ := json.Marshal(fixtureManifest())
	mustWrite(t, filepath.Join(root, manifestPath), string(b))
	mustWrite(t, filepath.Join(root, globalsPath), "go.mod\n")
	var out, errw bytes.Buffer
	if code := run([]string{"list-groups", "-root", root}, nil, &out, &errw); code != 0 || strings.TrimSpace(out.String()) != "//tests/e2e:misc" {
		t.Fatalf("list-groups: %d %q %s", code, out.String(), errw.String())
	}
	out.Reset()
	if code := run([]string{"specs-of", "-root", root, "-group", "//tests/e2e:misc"}, nil, &out, &errw); code != 0 || !strings.Contains(out.String(), "alias.spec.ts") {
		t.Fatalf("specs-of: %d %q", code, out.String())
	}
}

func TestUniversalShareCountsClosureHits(t *testing.T) {
	g := parseGraph(mod + "/server " + mod + "/session\n" + mod + "/session\n" + mod + "/github\n")
	cl := map[string]bool{}
	for _, p := range g.Closure([]string{mod + "/server"}) {
		cl[p] = true
	}
	if !cl[mod+"/session"] || cl[mod+"/github"] {
		t.Fatalf("closure %v", cl)
	}
}

func TestCheckQuarantine(t *testing.T) {
	ok := QuarantineRow{Spec: "a", Owner: "o", Issue: "#1", Date: "2099-01-01", Expiry: "2099-01-30"}
	if bad := checkQuarantine([]QuarantineRow{ok}, ""); len(bad) != 0 {
		t.Fatalf("valid row rejected: %v", bad)
	}
	long := ok
	long.Expiry = "2099-03-01"
	noOwner := ok
	noOwner.Owner = ""
	for name, c := range map[string]struct {
		rows  []QuarantineRow
		build string
	}{
		"expiry over 30 days": {[]QuarantineRow{long}, ""},
		"no owner":            {[]QuarantineRow{noOwner}, ""},
		"flaky=True":          {nil, "e2e_test(flaky = True)"},
		"flaky attempts":      {nil, "--flaky_test_attempts=3"},
	} {
		if len(checkQuarantine(c.rows, c.build)) == 0 {
			t.Errorf("%s not rejected", name)
		}
	}
}

func TestCheckSlowSuites(t *testing.T) {
	good := "| Suite | p50 s | Classification | Status |\n|---|---|---|---|\n| a | 900 | rdeps | x |\n| b | 400 | full: benchmarks | x |\n| c | 100 | | x |\n| d | - | | x |\n"
	if bad := checkSlowSuites(good); len(bad) != 0 {
		t.Fatalf("%v", bad)
	}
	for _, row := range []string{"| e | 301 | | x |", "| e | 301 | full: | x |", "| e | 301 | maybe | x |"} {
		if len(checkSlowSuites(good+row+"\n")) != 1 {
			t.Errorf("%q not flagged", row)
		}
	}
}

func TestGrowthOfGaps(t *testing.T) {
	if g := growthOfGaps([]string{"a", "b"}, []string{"a", "b", "c"}); len(g) != 0 {
		t.Fatalf("shrinking list flagged: %v", g)
	}
	if g := growthOfGaps([]string{"a", "new"}, []string{"a"}); !reflect.DeepEqual(g, []string{"new"}) {
		t.Fatalf("%v", g)
	}
}

func TestParseHunksNewSideRanges(t *testing.T) {
	d := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -3,2 +3,4 @@\n@@ -10 +12,0 @@\n"
	got := parseHunks(d)["x.go"]
	if len(got) != 2 || got[0] != (Line{3, 6}) || got[1] != (Line{12, 12}) {
		t.Fatalf("%v", got)
	}
}

func fineFixture(gran Granularity) *FineSelector {
	caps := []FineCapture{
		{Spec: "tests/e2e/a.spec.ts", GoPkgs: []string{modulePath + "/p"}, GoFiles: []string{"p/x.go"},
			GoLines: map[string][]Line{"p/x.go": {{10, 20}}}, GoAll: map[string][]Line{"p/x.go": {{10, 20}, {30, 40}}}},
		{Spec: "tests/e2e/b.spec.ts", GoPkgs: []string{modulePath + "/p", modulePath + "/q"}, GoFiles: []string{"p/x.go", "p/y.go"},
			GoLines: map[string][]Line{"p/x.go": {{30, 40}}}, GoAll: map[string][]Line{"p/x.go": {{10, 20}, {30, 40}}}},
	}
	hunks := func(string, []string) map[string][]Line { return map[string][]Line{"p/x.go": {{12, 13}}} }
	return newFineSelector(caps, []string{"tests/e2e/a.spec.ts", "tests/e2e/b.spec.ts"}, []string{"go.mod"}, gran, hunks)
}

func TestFineSelectorGranularities(t *testing.T) {
	c := Commit{SHA: "s", Files: []string{"p/x.go"}}
	if got := fineFixture(GranPackage).Select(c).PerSpec; got != 2 {
		t.Fatalf("package: %d", got)
	}
	if got := fineFixture(GranFile).Select(c).PerSpec; got != 2 {
		t.Fatalf("file: %d", got)
	}
	if got := fineFixture(GranLine).Select(c).PerSpec; got != 1 { // only a.spec ran lines 10-20
		t.Fatalf("line: %d", got)
	}
	if s := fineFixture(GranLine).Select(Commit{Files: []string{"go.mod"}}); !s.All {
		t.Fatal("global input must select all")
	}
	if got := fineFixture(GranFile).Select(Commit{Files: []string{"p/y.go"}}).PerSpec; got != 1 {
		t.Fatalf("y.go only b: %d", got)
	}
}

func TestFineSelectorHunkOutsideBlocksIsConservative(t *testing.T) {
	s := fineFixture(GranLine)
	s.hunks = func(string, []string) map[string][]Line { return map[string][]Line{"p/x.go": {{25, 26}}} }
	if got := s.Select(Commit{SHA: "s", Files: []string{"p/x.go"}}).PerSpec; got != 2 {
		t.Fatalf("a hunk in no statement block must keep every spec that ran the file: %d", got)
	}
}

func TestLineStrictIgnoresHunksOutsideBlocks(t *testing.T) {
	s := fineFixture(GranLineStrict)
	s.hunks = func(string, []string) map[string][]Line { return map[string][]Line{"p/x.go": {{25, 26}}} }
	if got := s.Select(Commit{SHA: "s", Files: []string{"p/x.go"}}).PerSpec; got != 0 {
		t.Fatalf("strict mode drops a hunk in no block: %d", got)
	}
}

func TestPackageGranularityDerivesPackagesFromExecutedFiles(t *testing.T) {
	caps := []FineCapture{{Spec: "tests/e2e/a.spec.ts", GoFiles: []string{"config/x.go"}}} // go_pkgs rounded config away
	s := newFineSelector(caps, []string{"tests/e2e/a.spec.ts"}, nil, GranPackage, nil)
	if got := s.Select(Commit{Files: []string{"config/other.go"}}).PerSpec; got != 1 {
		t.Fatalf("a package whose files ran must select the spec: %d", got)
	}
}
