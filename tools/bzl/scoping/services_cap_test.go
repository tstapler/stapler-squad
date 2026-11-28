package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tstapler/stapler-squad/internal/reporoot"
)

func TestServicesCap_ShouldFailOnlyAboveTheCap(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go toolchain, unavailable in the Bazel sandbox")
	}
	limit := DefaultT9Targets.T9dServicesLOCMax
	for name, tc := range map[string]struct {
		lines, code int
		want        string
	}{
		"at the cap":   {limit, 0, "PASS"},
		"over the cap": {limit + 1, 1, "FAIL"},
	} {
		root := t.TempDir()
		write(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.21\n")
		write(t, root, "server/services/a.go", "package services\n"+strings.Repeat("\n", tc.lines-1))
		// Neither a leaf package nor a test file counts towards the cap.
		write(t, root, "server/services/leaf/b.go", "package leaf\n"+strings.Repeat("\n", limit))
		write(t, root, "server/services/a_test.go", "package services\n"+strings.Repeat("\n", limit))
		var out strings.Builder
		if code := runServicesCap([]string{"-root", root}, &out); code != tc.code || !strings.Contains(out.String(), tc.want) {
			t.Errorf("%s: code=%d output=%s", name, code, out.String())
		}
	}
	var out strings.Builder
	empty := t.TempDir()
	write(t, empty, "go.mod", "module "+modulePath+"\n\ngo 1.21\n")
	if code := runServicesCap([]string{"-root", empty}, &out); code != 1 {
		t.Errorf("a missing package must not pass: code=%d output=%s", code, out.String())
	}
}

// The cap is enforced by this step alone: it must run on every pull request in a
// job that can fail it.
func TestServicesCap_ShouldBeABlockingStepOfBazelGuardsOnEveryPullRequest(t *testing.T) {
	root, err := reporoot.Find()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github/workflows/bazel.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On struct {
			PullRequest map[string]any `yaml:"pull_request"`
		} `yaml:"on"`
		Jobs map[string]struct {
			If              string `yaml:"if"`
			ContinueOnError any    `yaml:"continue-on-error"`
			Steps           []struct {
				Run             string `yaml:"run"`
				If              string `yaml:"if"`
				ContinueOnError any    `yaml:"continue-on-error"`
			}
		}
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	if wf.On.PullRequest == nil || wf.On.PullRequest["paths"] != nil || wf.On.PullRequest["paths-ignore"] != nil {
		t.Errorf("bazel.yml must run on every pull request, got %v", wf.On.PullRequest)
	}
	job := wf.Jobs["bazel-guards"]
	if job.If != "" || job.ContinueOnError != nil {
		t.Errorf("bazel-guards must be blocking and unconditional: if=%q continue-on-error=%v", job.If, job.ContinueOnError)
	}
	var steps int
	for _, step := range job.Steps {
		if step.Run != "go run ./tools/bzl/scoping services-cap" {
			continue
		}
		steps++
		if step.If != "${{ !cancelled() }}" || step.ContinueOnError != nil {
			t.Errorf("the cap step must run after earlier failures and block: if=%q continue-on-error=%v", step.If, step.ContinueOnError)
		}
	}
	if steps != 1 {
		t.Errorf("bazel-guards runs services-cap %d times, want 1", steps)
	}
}
