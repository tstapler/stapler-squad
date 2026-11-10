package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// shardInclude is one GitHub Actions matrix row.
type shardInclude struct {
	Shard int      `json:"shard"`
	Total int      `json:"total"`
	Tours []string `json:"tours"`
	// Only is Tours joined for `record-website-demos.sh --only`.
	Only string `json:"only"`
}

type shardMatrix struct {
	Include []shardInclude `json:"include"`
}

// tourIDs lists the scene ids in dir: every *.ts except tests and files
// starting with "_" (shared helpers), sorted so the plan is deterministic.
func tourIDs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".ts" || strings.HasSuffix(name, ".test.ts") || strings.HasPrefix(name, "_") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, ".ts"))
	}
	sort.Strings(ids)
	return ids, nil
}

// planShards deals the sorted, de-duplicated ids round-robin into n shards.
// Every id lands in exactly one shard; shards may be empty when ids < n.
func planShards(ids []string, n int) shardMatrix {
	uniq := map[string]bool{}
	var sorted []string
	for _, id := range ids {
		if !uniq[id] {
			uniq[id] = true
			sorted = append(sorted, id)
		}
	}
	sort.Strings(sorted)
	m := shardMatrix{Include: make([]shardInclude, n)}
	for i := range m.Include {
		m.Include[i] = shardInclude{Shard: i, Total: n, Tours: []string{}}
	}
	for i, id := range sorted {
		s := &m.Include[i%n]
		s.Tours = append(s.Tours, id)
	}
	for i := range m.Include {
		m.Include[i].Only = strings.Join(m.Include[i].Tours, ",")
	}
	return m
}

func runShardPlan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("shard-plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("tours-dir", "tests/e2e/website-demos/tours", "directory of scene files, one <tour-id>.ts each")
	shards := fs.Int("shards", 2, "number of record shards")
	var extra stringList
	fs.Var(&extra, "extra", "additional recordable id outside tours-dir, e.g. a legacy clip (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *shards < 1 {
		fmt.Fprintln(stderr, "mediatool shard-plan: -shards must be >= 1")
		return 2
	}
	ids, err := tourIDs(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "mediatool shard-plan:", err)
		return 2
	}
	out, err := json.Marshal(planShards(append(ids, extra...), *shards))
	if err != nil {
		fmt.Fprintln(stderr, "mediatool shard-plan:", err)
		return 2
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}
