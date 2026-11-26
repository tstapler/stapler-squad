package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/internal/reporoot"
)

func pkgs(rel ...string) []string {
	for i, r := range rel {
		rel[i] = module + r
	}
	return rel
}

func fixture() []string {
	return pkgs("config", "server", "server/services", "session", "session/mux", "session/tmux",
		"session/tmux/embed", "session/artifacts", "session/backlog", "session/backlog/chains",
		"session/backlogstore", "session/unfinished", "session/unfinished/repostate", "session/brandnew")
}

func TestPartition_should_AssignEveryPackageExactlyOnce(t *testing.T) {
	t.Parallel()
	got, err := partition(fixture())
	if err != nil {
		t.Fatal(err)
	}
	var union []string
	for _, p := range got {
		union = append(union, p...)
	}
	want := fixture()
	slices.Sort(union)
	slices.Sort(want)
	if !slices.Equal(union, want) {
		t.Fatalf("union of shards = %v, want %v", union, want)
	}
	// New packages and lookalike prefixes fall to the catch-all.
	for _, p := range pkgs("session/brandnew", "session/backlogstore", "session/tmux/embed") {
		if !slices.Contains(got["rest"], p) {
			t.Errorf("%s not in rest: %v", p, got["rest"])
		}
	}
}

func TestPartition_should_Fail_When_APatternMatchesNothing(t *testing.T) {
	t.Parallel()
	without := slices.DeleteFunc(fixture(), func(p string) bool { return p == module+"session/mux" })
	if _, err := partition(without); err == nil || !strings.Contains(err.Error(), `"session/mux" matches no package`) {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_should_RejectAnUnknownShard(t *testing.T) {
	t.Parallel()
	list := func() ([]string, error) { return fixture(), nil }
	if code := run([]string{"nope"}, list, io.Discard, io.Discard); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// The partition only covers every package if the workflow runs every shard.
func TestMainCI_should_RunEveryShardExactlyOnce(t *testing.T) {
	t.Parallel()
	root, err := reporoot.Find()
	if err != nil {
		t.Fatal(err)
	}
	yml, err := os.ReadFile(filepath.Join(root, ".github/workflows/main-ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var invoked []string
	for _, m := range regexp.MustCompile(`go run \./tools/ci/testshards ([a-z-]+)`).FindAllStringSubmatch(string(yml), -1) {
		invoked = append(invoked, m[1])
	}
	var want []string
	for _, s := range shards {
		want = append(want, s.name)
	}
	slices.Sort(invoked)
	slices.Sort(want)
	if !slices.Equal(invoked, want) {
		t.Fatalf("main-ci.yml runs shards %v, want each of %v once", invoked, want)
	}
}
