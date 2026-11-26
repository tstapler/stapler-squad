package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/internal/reporoot"
	"gopkg.in/yaml.v3"
)

// Stand-ins for the tools main-ci.yml's test steps call. Like `go test`, a failing shard still
// writes its coverage profile, so only the exit status can fail the step.
var stubs = map[string]string{
	"go": `#!/bin/sh
echo "pkg/$3"
`,
	"gotestsum": `#!/bin/sh
shard=rest
while [ $# -gt 0 ]; do
  case "$1" in
    --jsonfile) json="$2"; shift ;;
    -coverprofile=*) cover="${1#*=}" ;;
    pkg/*) echo "$1" >> "$STUB_DIR/tested" ;;
  esac
  shift
done
[ -n "$json" ] || exit 1
case "$json" in *go-test-*) shard="${json##*go-test-}"; shard="${shard%.json}" ;; esac
echo '{}' > "$json"
printf 'mode: atomic\nline-%s\n' "$shard" > "$cover"
[ "$shard" != "$FAIL_SHARD" ]
`,
	"jq": "#!/bin/sh\n",
}

// testStep returns the script of a job's "Run tests with coverage" step.
func testStep(t *testing.T, job string) string {
	t.Helper()
	root, err := reporoot.Find()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github/workflows/main-ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct{ Name, Run string }
		}
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	for _, s := range wf.Jobs[job].Steps {
		if strings.HasPrefix(s.Name, "Run tests with coverage") {
			return s.Run
		}
	}
	t.Fatalf("job %q has no test step", job)
	return ""
}

type stepResult struct {
	err error
	dir string
	log string
}

func (r stepResult) lines(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, name))
	if err != nil {
		t.Fatalf("%v\n%s", err, r.log)
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

// runStep runs a step's script the way the runner does, with failShard's tests failing.
func runStep(t *testing.T, job, failShard string) stepResult {
	t.Helper()
	for _, tool := range []string{"bash", "timeout"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	dir := t.TempDir()
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", testStep(t, job))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RUNNER_TEMP="+dir, "STUB_DIR="+dir, "FAIL_SHARD="+failShard,
		"GITHUB_STEP_SUMMARY="+filepath.Join(dir, "summary"))
	out, err := cmd.CombinedOutput()
	return stepResult{err: err, dir: dir, log: string(out)}
}

func TestTmuxStep_should_TestAndMergeBothShards(t *testing.T) {
	t.Parallel()
	r := runStep(t, "test-tmux", "")
	if r.err != nil {
		t.Fatalf("%v\n%s", r.err, r.log)
	}
	if got, want := r.lines(t, "coverage.tmux.out"), "mode: atomic line-tmux line-session-slice"; got != want {
		t.Errorf("merged profile = %q, want %q", got, want)
	}
	if got, want := r.lines(t, "tested"), "pkg/tmux pkg/session-slice"; got != want {
		t.Errorf("tested = %q, want %q", got, want)
	}
}

func TestTestSteps_should_Fail_When_AShardFails(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ job, shard, tested string }{
		{"test-tmux", "tmux", "pkg/tmux pkg/session-slice"},
		{"test-tmux", "session-slice", "pkg/tmux pkg/session-slice"},
		{"test-rest", "rest", "pkg/rest"},
	} {
		t.Run(tc.shard, func(t *testing.T) {
			t.Parallel()
			r := runStep(t, tc.job, tc.shard)
			if r.err == nil {
				t.Fatalf("step passed with %s failing\n%s", tc.shard, r.log)
			}
			if got := r.lines(t, "tested"); got != tc.tested {
				t.Errorf("tested = %q, want %q", got, tc.tested)
			}
		})
	}
}
