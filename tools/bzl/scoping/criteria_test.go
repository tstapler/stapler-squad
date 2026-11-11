package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestLowerPackageSC1CountsRelocationsNotUses(t *testing.T) {
	t.Parallel()
	c := Candidate{Lower: true, Relocated: 0, StayingUses: 13}
	if c.SC1Count() != 0 || !c.SC1OK() {
		t.Fatalf("lower package with 13 uses and 0 relocations must pass SC1, count=%d", c.SC1Count())
	}
	if c.Relocated = 11; c.SC1OK() {
		t.Fatal("lower package relocating 11 declarations must fail SC1")
	}
}

func TestUpperPackageSC1StillCountsUses(t *testing.T) {
	t.Parallel()
	if (Candidate{StayingUses: 13}).SC1OK() {
		t.Fatal("non-lower cluster with 13 staying uses must fail SC1")
	}
	if !(Candidate{StayingUses: 10, Relocated: 50}).SC1OK() {
		t.Fatal("non-lower cluster with 10 staying uses must pass SC1")
	}
}

func TestLowerPackageSC3IgnoresNaturalCallers(t *testing.T) {
	t.Parallel()
	imps := []string{".", "server", "server/auth", "server/services", "session/backlog", "a", "b"}
	c := Candidate{Lower: true, Importers: imps, NaturalCallers: imps[:5]}
	if got := c.SC3Importers(); len(got) != 2 || !c.SC3OK() {
		t.Fatalf("natural callers must not count: %v", got)
	}
	c.NaturalCallers = nil
	if c.SC3OK() {
		t.Fatal("7 new importers must fail SC3")
	}
	c.Lower = false
	c.NaturalCallers = imps
	if c.SC3OK() {
		t.Fatal("a non-lower cluster counts every importer (7 > 5)")
	}
}

func TestPrintCriteriaAndExitCode(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if code := runCriteria([]string{"--lower", "--relocated", "2", "--uses", "13", "--importers", "a,b", "--natural", "a,b"}, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "SC1: PASS (lower package") {
		t.Fatalf("output: %s", out.String())
	}
	out.Reset()
	if code := runCriteria([]string{"--uses", "13"}, &out); code != 1 || !strings.Contains(out.String(), "SC1: FAIL") {
		t.Fatalf("exit %d: %s", code, out.String())
	}
}
