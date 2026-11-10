package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestPathsMatch(t *testing.T) {
	t.Parallel()
	pats := []string{"server/mcp/**", "server/services/**", "main.go", "go.mod"}
	cases := map[string]bool{
		"server/mcp/x.go":              true,
		"server/services/a/b/c.go":     true,
		"main.go":                      true,
		"sub/main.go":                  false,
		"server/other/x.go":            false,
		"web-app/src/app/page.tsx":     false,
		"server/services/wsconn/a.go":  true,
		".github/scripts/mcp-suite.sh": false,
	}
	for f, want := range cases {
		if got := PathsMatch(pats, []string{f}); got != want {
			t.Errorf("%s: got %v want %v", f, got, want)
		}
	}
	if !PathsMatch([]string{"*.go"}, []string{"a.go"}) || PathsMatch([]string{"*.go"}, []string{"d/a.go"}) {
		t.Error("`*` must not cross directories")
	}
}

func TestRegexFromWorkflow(t *testing.T) {
	t.Parallel()
	re, err := RegexFromWorkflow("run: |\n  if echo \"$C\" | grep -qE '\\.go$|^Makefile$|^proto/'; then\n")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("a/b.go") || !re.MatchString("proto/x.proto") || re.MatchString("docs/a.md") {
		t.Errorf("%v", re)
	}
	if _, err := RegexFromWorkflow("nothing here"); err == nil {
		t.Error("want error")
	}
}

func TestParseSelection(t *testing.T) {
	t.Parallel()
	s, err := ParseSelection(strings.NewReader("//a:t\n\njob:e2e\n//server/mcp:t\n"))
	if err != nil || len(s.Targets) != 2 || len(s.Jobs) != 1 || s.Jobs[0] != "e2e" {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestMisses(t *testing.T) {
	t.Parallel()
	empty := Selection{}
	if got := Misses(Fired{Regex: true, Script: true, Paths: true}, empty); len(got) != 3 {
		t.Errorf("nothing selected must miss all three: %v", got)
	}
	all := Selection{Targets: []string{"//..."}}
	if got := Misses(Fired{Regex: true, Script: true, Paths: true}, all); len(got) != 0 {
		t.Errorf("//... covers everything: %v", got)
	}
	cases := []struct {
		name  string
		fired Fired
		sel   Selection
		want  []string
	}{
		{"regex needs a target or build job", Fired{Regex: true}, Selection{Jobs: []string{"workflow-lint"}}, []string{Regex}},
		{"regex covered by e2e job", Fired{Regex: true}, Selection{Jobs: []string{"e2e"}}, nil},
		{"script covered by registry job", Fired{Script: true}, Selection{Jobs: []string{"registry-freshness"}}, nil},
		{"script not covered by go tests", Fired{Script: true}, Selection{Targets: []string{"//session:t"}}, []string{Script}},
		{"paths needs mcp or services target", Fired{Paths: true}, Selection{Targets: []string{"//session:t"}}, []string{Paths}},
		{"paths covered by services subpackage", Fired{Paths: true}, Selection{Targets: []string{"//server/services/wsconn:t"}}, nil},
		{"paths covered by the mcp-integration job", Fired{Paths: true}, Selection{Jobs: []string{"mcp-integration"}}, nil},
		{"mechanism silent means no miss", Fired{}, empty, nil},
	}
	for _, c := range cases {
		if got := Misses(c.fired, c.sel); strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestReplayCountsAndReport(t *testing.T) {
	t.Parallel()
	rp := Replayer{
		Regex:    regexp.MustCompile(`\.go$`),
		Patterns: []string{"server/mcp/**"},
		Affected: func(files []string) (Selection, error) {
			if files[0] == "session/a.go" {
				return Selection{Targets: []string{"//session:t"}}, nil
			}
			return Selection{}, nil
		},
		Script: func(files []string) (bool, error) { return files[0] == "server/mcp/x.go", nil },
	}
	sum, err := rp.Replay([]PR{{1, []string{"session/a.go"}}, {2, []string{"server/mcp/x.go"}}, {3, []string{"docs/a.md"}}})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Misses[Regex] != 1 || sum.Misses[Script] != 1 || sum.Misses[Paths] != 1 {
		t.Fatalf("%+v", sum.Misses)
	}
	var b strings.Builder
	sum.Write(&b)
	for _, want := range []string{"prs: 3", "misses_regex: 1", "misses_script: 1", "misses_paths: 1", "MISS paths PR #2"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, b.String())
		}
	}
}

func TestLoadSample(t *testing.T) {
	t.Parallel()
	prs, err := LoadSample([]byte(`{"partial":[{"number":7,"paths":["a.go","b.go"]}],"all":[]}`), "partial")
	if err != nil || len(prs) != 1 || prs[0].Number != 7 || len(prs[0].Files) != 2 {
		t.Fatalf("%v %+v", err, prs)
	}
	if _, err := LoadSample([]byte(`{}`), "partial"); err == nil {
		t.Error("want error for a missing list")
	}
}
