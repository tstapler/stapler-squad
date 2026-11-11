package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/waivers"
)

func okRun([]string) int { return 0 }

func env(run func([]string) int, ws ...waivers.Waiver) Env {
	return Env{Run: run, Attest: func(string) bool { return true }, Waivers: ws, Exists: func(string) bool { return false }}
}

func TestAllPassIsPlainPass(t *testing.T) {
	// No collector output files exist in this environment, so P10 is UNMEASURED even when every command would pass.
	res := Execute(Rows(), env(okRun))
	line, ok := Verdict(res, nil)
	if ok || line != "PARITY: FAIL P10" {
		t.Fatalf("%q", line)
	}
}

func TestDryRunPassesEverything(t *testing.T) {
	e := env(okRun)
	e.DryRun = true
	if line, ok := Verdict(Execute(Rows(), e), nil); !ok || line != "PARITY: PASS" {
		t.Fatal(line)
	}
}

func TestFailingRowNamed(t *testing.T) {
	run := func(cmd []string) int {
		if strings.Contains(strings.Join(cmd, " "), "compare-binary") {
			return 1
		}
		return 0
	}
	e := env(run)
	e.DryRun = false
	line, _ := Verdict(Execute(Rows(), e), nil)
	if !strings.Contains(line, "P5") {
		t.Fatal(line)
	}
}

func TestOwnerAttestationAbsentFailsP8(t *testing.T) {
	e := env(okRun)
	e.Attest = func(string) bool { return false }
	if line, _ := Verdict(Execute(Rows(), e), nil); !strings.Contains(line, "P8") {
		t.Fatal(line)
	}
}

func TestWaiverExcusesOnlyItsCheckAndIsReported(t *testing.T) {
	w := waivers.Waiver{Row: "P10.hit_ratio_floor", Reason: "owner accepts lower hit ratio for now", Granted: "2026-11-03", Expires: "2027-01-31"}
	rows := []Row{{"P10", "perf", []Check{{Name: "hit", Cmd: []string{"x"}, WaiverRow: "P10.hit_ratio_floor"}}}}
	res := Execute(rows, env(func([]string) int { return 1 }, w))
	line, ok := Verdict(res, []waivers.Waiver{w})
	if !ok || !strings.HasPrefix(line, "PARITY: PASS WITH WAIVERS (P10.hit_ratio_floor: owner accepts") {
		t.Fatalf("%q", line)
	}
	rows[0].Checks = append(rows[0].Checks, Check{Name: "other", Cmd: []string{"y"}})
	if _, ok := Verdict(Execute(rows, env(func([]string) int { return 1 }, w)), []waivers.Waiver{w}); ok {
		t.Fatal("a waiver must not excuse an unwaivable check")
	}
}

func TestDryRunCLIWithWaiverFixture(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-dry-run", "-today", "2026-11-04", "-waivers", "../testdata/waiver-ok.yaml", "-attestations", "../testdata/waiver-ok.yaml"}, &out, &errb)
	s := out.String()
	if code != 0 || strings.Count(s, "PARITY: PASS WITH WAIVERS") != 1 || !strings.HasPrefix(s, "WAIVER P10.hit_ratio_floor") {
		t.Fatalf("%d %s %s", code, s, errb.String())
	}
	if n := strings.Count(s, "\nP") + 0; n < 10 {
		t.Fatalf("want ten P rows, got %d in %s", n, s)
	}
}

func TestExpiredWaiverFailsGate(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-dry-run", "-today", "2026-11-04", "-waivers", "../testdata/waiver-expired.yaml", "-attestations", "../testdata/waiver-expired.yaml"}, &out, &errb)
	if code != 1 || !strings.Contains(out.String(), "PARITY: FAIL WAIVER P10.disk_ceiling") {
		t.Fatalf("%d %s", code, out.String())
	}
}

func TestP10UsesCollectorOutputWhenPresent(t *testing.T) {
	e := env(okRun)
	e.Exists = func(string) bool { return true }
	if line, ok := Verdict(Execute(Rows(), e), nil); !ok || line != "PARITY: PASS" {
		t.Fatal(line)
	}
}

func TestP10CollectorExitFourIsUnmeasuredNotFail(t *testing.T) {
	e := env(func(cmd []string) int {
		if strings.Contains(strings.Join(cmd, " "), "perf") {
			return 4
		}
		return 0
	})
	e.Exists = func(string) bool { return true }
	rs := Execute(Rows(), e)
	last := rs[len(rs)-1]
	if last.Row.ID != "P10" || last.Status != "UNMEASURED" {
		t.Fatalf("%+v", last)
	}
}

func TestP10DiskCeilingWaivableButWarmNot(t *testing.T) {
	e := env(func(cmd []string) int {
		if strings.Contains(strings.Join(cmd, " "), "-check warm,cold,ttg") {
			return 0
		}
		if strings.Contains(strings.Join(cmd, " "), "-check disk") {
			return 1
		}
		return 0
	}, waivers.Waiver{Row: "P10.disk_ceiling", Reason: "owner accepts a larger output base for now", Granted: "2026-11-03", Expires: "2027-01-31"})
	e.Exists = func(string) bool { return true }
	if line, _ := Verdict(Execute(Rows(), e), nil); line != "PARITY: PASS" {
		t.Fatal(line)
	}
}
