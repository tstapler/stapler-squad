// Command testshards prints the packages of one shard of main-ci.yml's full-suite test run.
// Usage: go run ./tools/ci/testshards <shard>. The last shard is a catch-all, so a new package
// under the roots is always tested. Balance numbers: docs/reference/ci-job-budget.md.
package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const module = "github.com/tstapler/stapler-squad/"

// roots is everything main-ci.yml's coverage gate tests.
var roots = []string{"./server/...", "./session/...", "./config/..."}

type shard struct {
	name string
	// patterns are module-relative packages; a "/..." suffix includes subpackages.
	// A shard with no patterns takes every package the others did not.
	patterns []string
}

var shards = []shard{
	// Fork real tmux servers at high fan-out, so they run in their own go test invocation.
	{name: "tmux", patterns: []string{"session", "session/mux", "session/sshtransport", "session/tmux"}},
	// Runs after "tmux" in the same job, which has the spare budget.
	{name: "session-slice", patterns: []string{"session/artifacts/...", "session/backlog/...", "session/unfinished/..."}},
	{name: "rest"},
}

func matches(pattern, pkg string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "/..."); ok {
		return pkg == module+prefix || strings.HasPrefix(pkg, module+prefix+"/")
	}
	return pkg == module+pattern
}

// partition assigns every package to exactly one shard. A pattern that matches nothing is an
// error: a renamed package would otherwise move to the catch-all and unbalance the jobs.
func partition(pkgs []string) (map[string][]string, error) {
	out := map[string][]string{}
	used := map[string]bool{}
	catchAll := ""
	for _, s := range shards {
		if len(s.patterns) == 0 {
			catchAll = s.name
		}
	}
	for _, pkg := range pkgs {
		dest := catchAll
		for _, s := range shards {
			if i := slices.IndexFunc(s.patterns, func(p string) bool { return matches(p, pkg) }); i >= 0 {
				dest, used[s.patterns[i]] = s.name, true
				break
			}
		}
		out[dest] = append(out[dest], pkg)
	}
	var errs []error
	for _, s := range shards {
		for _, p := range s.patterns {
			if !used[p] {
				errs = append(errs, fmt.Errorf("shard %q: pattern %q matches no package", s.name, p))
			}
		}
		if len(out[s.name]) == 0 {
			errs = append(errs, fmt.Errorf("shard %q is empty", s.name))
		}
	}
	return out, errors.Join(errs...)
}
