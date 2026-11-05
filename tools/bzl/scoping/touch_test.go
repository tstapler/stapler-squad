package main

import "testing"

func TestSoleGainCountsOnlyChangesFullyInsideTheSet(t *testing.T) {
	t.Parallel()
	changes := []map[string]bool{
		set("session/a.go"),
		set("session/a.go", "session/b.go"),
		set("session/a.go", "session/c.go"),
		{},
	}
	if got := SoleGain(changes, set("session/a.go", "session/b.go")); got != 2 {
		t.Fatalf("gain = %d, want 2", got)
	}
	if got := SoleGain(changes, set("session/a.go")); got != 1 {
		t.Fatalf("gain = %d, want 1", got)
	}
}

func TestUniversalFilesKeepsNonUniversalProdChangesAsEmptySets(t *testing.T) {
	t.Parallel()
	m := &Model{Lib: map[string]map[string]bool{"session": {}, "leaf": {}}}
	r := Resolver{Model: m, Renames: map[string]string{"session/old.go": "session/new.go"}}
	got := UniversalFiles(r, []Change{
		{ID: "1", Paths: []string{"session/old.go", "session/x_test.go"}},
		{ID: "2", Paths: []string{"leaf/l.go"}},
		{ID: "3", Paths: []string{"session/x_test.go", "README.md"}},
	})
	if len(got) != 2 || !got[0]["session/new.go"] || len(got[1]) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestFamilyGainsRanksByGain(t *testing.T) {
	t.Parallel()
	changes := []map[string]bool{
		set("server/services/ws_a.go", "server/services/ws_b.go"),
		set("server/services/ws_a.go"),
		set("server/services/hook_a.go"),
	}
	rows := FamilyGains(changes, false)
	if rows[0].Name != "server/services/ws" || rows[0].Gain != 2 || rows[0].Files != 2 {
		t.Fatalf("rows[0] = %+v", rows[0])
	}
}
