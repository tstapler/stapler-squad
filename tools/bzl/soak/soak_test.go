package soak

import (
	"fmt"
	"strings"
	"testing"
)

func ledger(n, k int, runs ...Run) *Ledger {
	return &Ledger{Schema: 1, N: n, K: k, QueueTimeoutSeconds: 900, Runs: runs}
}

func pass(i int) Run { return Run{RunID: fmt.Sprint(i), Result: "pass"} }

func passes(from, to int) []Run {
	var out []Run
	for i := from; i < to; i++ {
		out = append(out, pass(i))
	}
	return out
}

func TestMetWithFlakesAttributed(t *testing.T) {
	runs := passes(0, 28)
	runs = append(runs, Run{RunID: "f1", Result: "fail", Attribution: Flake, Bug: "BUG-087"}, pass(99))
	l := ledger(30, 10, runs...)
	if errs := l.Validate(); len(errs) != 0 {
		t.Fatal(errs)
	}
	if rep := l.Evaluate(true); !rep.Met {
		t.Fatalf("%s", rep)
	}
}

func TestUnattributedFailureBlocks(t *testing.T) {
	runs := append(passes(0, 29), Run{RunID: "x", Result: "fail"})
	rep := ledger(30, 10, runs...).Evaluate(true)
	if rep.Met || rep.Unattributed != 1 {
		t.Fatalf("%s", rep)
	}
}

func TestBazelRealInLastKBlocksButOlderDoesNot(t *testing.T) {
	old := append([]Run{{RunID: "r", Result: "fail", Attribution: BazelReal}}, passes(0, 29)...)
	if rep := ledger(30, 10, old...).Evaluate(true); !rep.Met {
		t.Fatalf("old bazel-real is outside K: %s", rep)
	}
	recent := append(passes(0, 29), Run{RunID: "r", Result: "fail", Attribution: BazelReal})
	if rep := ledger(30, 10, recent...).Evaluate(true); rep.Met || rep.BazelReal != 1 {
		t.Fatalf("%s", rep)
	}
}

func TestFewerThanNRunsNotMet(t *testing.T) {
	if rep := ledger(30, 10, passes(0, 5)...).Evaluate(true); rep.Met {
		t.Fatal("5 runs cannot meet N=30")
	}
}

func TestParityMustPass(t *testing.T) {
	if rep := ledger(30, 10, passes(0, 30)...).Evaluate(false); rep.Met {
		t.Fatal("parity not passing")
	}
}

func TestOnlyLastNCounted(t *testing.T) {
	runs := append([]Run{{RunID: "bad", Result: "fail"}}, passes(0, 30)...)
	if rep := ledger(30, 10, runs...).Evaluate(true); !rep.Met {
		t.Fatalf("failure older than N must age out: %s", rep)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]Run{
		"flake without bug":     {RunID: "1", Result: "fail", Attribution: Flake},
		"unknown class":         {RunID: "1", Result: "fail", Attribution: "cosmic-ray"},
		"pass with attribution": {RunID: "1", Result: "pass", Attribution: Infra},
		"bad result":            {RunID: "1", Result: "maybe"},
		"missing id":            {Result: "pass"},
		"queue over timeout":    {RunID: "1", Result: "pass", QueueSeconds: 901},
		"bug on non-flake":      {RunID: "1", Result: "fail", Attribution: Infra, Bug: "BUG-1"},
	}
	for name, r := range cases {
		if errs := ledger(30, 10, r).Validate(); len(errs) == 0 {
			t.Errorf("%s: want a validation error", name)
		}
	}
	if errs := ledger(30, 10, pass(1), pass(1)).Validate(); len(errs) == 0 {
		t.Error("duplicate run_id must be rejected")
	}
}

func TestParseDefaultsAndStrictKeys(t *testing.T) {
	l, err := Parse(strings.NewReader("schema: 1\nruns: []\n"))
	if err != nil || l.N != 30 || l.K != 10 || l.QueueTimeoutSeconds != 900 {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := Parse(strings.NewReader("schema: 1\nrunz: []\n")); err == nil {
		t.Fatal("unknown key must be rejected")
	}
}

func TestReportSeparatesGeneratedFileRace(t *testing.T) {
	runs := append(passes(0, 29), Run{RunID: "g", Result: "fail", Attribution: GeneratedFileRace})
	s := ledger(30, 10, runs...).Evaluate(true).String()
	if !strings.Contains(s, "generated-file-race=1") {
		t.Fatal(s)
	}
}
