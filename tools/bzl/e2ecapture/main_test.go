package main

import (
	"reflect"
	"testing"
)

func TestExecutedPackagesKeepsOnlyRunStatements(t *testing.T) {
	out := "\tgithub.com/tstapler/stapler-squad/session\tcoverage: 12.5% of statements\n" +
		"\tgithub.com/tstapler/stapler-squad/server/services\tcoverage: 3.0% of statements\n" +
		"\tgithub.com/tstapler/stapler-squad/github\tcoverage: 0.0% of statements\n" +
		"\tgolang.org/x/sys\tcoverage: 50.0% of statements\n" +
		"noise line\n"
	want := []string{"github.com/tstapler/stapler-squad/server/services", "github.com/tstapler/stapler-squad/session"}
	if got := executedPackages(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFilterEnvDropsUserAgent(t *testing.T) {
	got := filterEnv([]string{"A=1", "npm_config_user_agent=npm/9", "B=2"}, "npm_config_user_agent")
	if !reflect.DeepEqual(got, []string{"A=1", "B=2"}) {
		t.Fatal(got)
	}
}

func TestParseTextfmtKeepsExecutedRangesAndMerges(t *testing.T) {
	in := "mode: set\n" +
		modulePath + "/session/a.go:10.2,12.3 2 1\n" +
		modulePath + "/session/a.go:13.1,14.2 1 1\n" +
		modulePath + "/session/a.go:20.1,21.2 1 0\n" +
		modulePath + "/session/b.go:1.1,2.2 1 0\n" +
		"golang.org/x/y/z.go:1.1,2.2 1 1\n"
	got := parseTextfmt(in)
	want := map[string][]Line{"session/a.go": {{10, 14}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestVLQAndWebSources(t *testing.T) {
	// "AAAA" = (0,0,0,0); the second line maps col 0 to source 1 ("AACA;AACA" shifts source by +1).
	js := []byte("aaaa\nbbbb\n")
	mp := []byte(`{"sources":["../src/a.ts","../src/b.ts","../../node_modules/x.js"],"mappings":"AAAA;ACAA"}`)
	cov := make([]bool, len(js)+1)
	for i := 5; i < 9; i++ { // only line 2 executed
		cov[i] = true
	}
	got, err := webSources(js, mp, cov)
	if err != nil || !reflect.DeepEqual(got, []string{"web-app/src/b.ts"}) {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestCoveredOffsetsInnerRangeOverrides(t *testing.T) {
	var f v8Func
	f.Ranges = append(f.Ranges, struct {
		Start int `json:"startOffset"`
		End   int `json:"endOffset"`
		Count int `json:"count"`
	}{0, 10, 1}, struct {
		Start int `json:"startOffset"`
		End   int `json:"endOffset"`
		Count int `json:"count"`
	}{3, 6, 0})
	cov := coveredOffsets([]v8Func{f}, 10)
	if !cov[2] || cov[3] || cov[5] || !cov[6] {
		t.Fatal(cov)
	}
}

func TestRPCsOf(t *testing.T) {
	got := rpcsOf([]string{"/session.v1.SessionService/ListSessions", "/api/session.v1.SessionService/GetSession", "/", "/_next/x.js"})
	want := []string{"/session.v1.SessionService/GetSession", "/session.v1.SessionService/ListSessions"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

func TestWithFileDirsAddsRoundedAwayPackages(t *testing.T) {
	got := withFileDirs([]string{modulePath + "/session"}, []string{"config/x.go", "main.go", "session/a.go"})
	want := []string{modulePath, modulePath + "/config", modulePath + "/session"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
