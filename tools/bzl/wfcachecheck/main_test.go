package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestViolations(t *testing.T) {
	cases := map[string]int{
		"ok-main-only":   0,
		"bad-pr-writes":  1,
		"ok-no-write":    0,
		"bad-upload-all": 1,
	}
	for name, want := range cases {
		b, err := os.ReadFile(filepath.Join("testdata", name+".yml"))
		if err != nil {
			t.Fatal(err)
		}
		v, err := violations(name, b)
		if err != nil || len(v) != want {
			t.Errorf("%s: got %v err %v, want %d", name, v, err, want)
		}
	}
}

func TestRepoWorkflowsAreClean(t *testing.T) {
	files, _ := filepath.Glob("../../../.github/workflows/bazel-e2e.yml")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if v, err := violations(f, b); err != nil || len(v) != 0 {
			t.Errorf("%v %v", v, err)
		}
	}
}
