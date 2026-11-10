// Command wfcachecheck is the cache-poisoning lint of Task 12.5d (ADR-004 control reused for bazel-e2e.yml): a job
// that holds the Bazel cache write credential, or uploads local results, must only run on pushes to main.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var writeMarker = regexp.MustCompile(`bazel-cache-write|BAZEL_CACHE_WRITE|--config=ci-cache-write|--remote_upload_local_results(=true|\s|$)`)

type job struct {
	If    string      `yaml:"if"`
	Steps []yaml.Node `yaml:"steps"`
	Needs yaml.Node   `yaml:"needs"`
}

type workflow struct {
	Jobs map[string]yaml.Node `yaml:"jobs"`
}

// mainOnly reports whether a job's `if` confines it to pushes to main.
func mainOnly(cond string) bool {
	return strings.Contains(cond, "github.event_name == 'push'") || strings.Contains(cond, "refs/heads/main")
}

// violations returns one message per job that holds a write marker without being main-only.
func violations(file string, src []byte) ([]string, error) {
	var w workflow
	if err := yaml.Unmarshal(src, &w); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	var out []string
	for name, node := range w.Jobs {
		var j job
		if err := node.Decode(&j); err != nil {
			return nil, err
		}
		raw, err := yaml.Marshal(&node)
		if err != nil {
			return nil, err
		}
		if writeMarker.Match(raw) && !mainOnly(j.If) {
			out = append(out, fmt.Sprintf("%s: job %q holds the cache write credential or uploads results but is not restricted to push on main", filepath.Base(file), name))
		}
	}
	return out, nil
}

func main() {
	files := os.Args[1:]
	if len(files) == 0 {
		files, _ = filepath.Glob(".github/workflows/*.yml")
	}
	bad := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		v, err := violations(f, b)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		for _, m := range v {
			fmt.Fprintln(os.Stderr, "wfcachecheck:", m)
			bad++
		}
	}
	if bad > 0 {
		os.Exit(1)
	}
}
