package main

import (
	"strings"
	"testing"
)

func TestPeelLeavesStopsAtFilesTheCoreStillUses(t *testing.T) {
	t.Parallel()
	// core.go uses shared.go; leafA.go uses shared.go and leafB.go; leafB.go uses nothing.
	edges := map[string]map[string]bool{
		"core.go":   {"shared.go": true},
		"shared.go": {},
		"leafA.go":  {"shared.go": true, "leafB.go": true},
		"leafB.go":  {},
	}
	loc := map[string]int{"core.go": 10, "shared.go": 20, "leafA.go": 30, "leafB.go": 40}
	got := PeelLeaves(edges, loc)
	// nothing uses core.go or leafA.go, so they peel first; that frees shared.go and leafB.go, so all four peel as one component.
	if len(got) != 1 || got[0].LOC != 100 || strings.Join(got[0].Files, ",") != "core.go,leafA.go,leafB.go,shared.go" {
		t.Fatalf("got %+v", got)
	}
	cyc := map[string]map[string]bool{"a.go": {"b.go": true}, "b.go": {"a.go": true}, "leaf.go": {"a.go": true}}
	got = PeelLeaves(cyc, map[string]int{"a.go": 1, "b.go": 1, "leaf.go": 5})
	if len(got) != 1 || strings.Join(got[0].Files, ",") != "leaf.go" {
		t.Fatalf("a cycle must stay: %+v", got)
	}
}

func TestReverseEdgesPeelsLowerLayers(t *testing.T) {
	t.Parallel()
	// app uses lib; lib uses nothing: peeling the reversed graph removes lib first, then app.
	edges := map[string]map[string]bool{"app.go": {"lib.go": true}, "lib.go": {}, "alone.go": {}}
	got := PeelLeaves(reverseEdges(edges), map[string]int{"app.go": 1, "lib.go": 2, "alone.go": 4})
	if len(got) != 2 || got[0].Files[0] != "alone.go" || strings.Join(got[1].Files, ",") != "app.go,lib.go" {
		t.Fatalf("got %+v", got)
	}
}
