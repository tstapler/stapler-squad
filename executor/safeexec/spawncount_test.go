//go:build !windows

package safeexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// backstopSum reads the cumulative backstop counter for one subcommand from the
// process-lifetime reader installed in safeexec_pg_test.go. Callers diff against a baseline.
func backstopSum(t *testing.T, subcommand string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := testMetricReader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || m.Name != MetricGitSpawnBackstop {
				continue
			}
			for _, dp := range sum.DataPoints {
				if v, ok := dp.Attributes.Value("subcommand"); ok && v.AsString() == subcommand {
					total += dp.Value
				}
				for _, kv := range dp.Attributes.ToSlice() {
					if kv.Key != "subcommand" {
						t.Fatalf("backstop carries label %q; only subcommand is allowed", kv.Key)
					}
				}
			}
		}
	}
	return total
}

func TestGitSubcommand(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"status", "--porcelain"}, "status"},
		{[]string{"-C", "/some/repo", "status"}, "status"},
		{[]string{"-c", "core.quotepath=off", "diff", "--numstat"}, "diff"},
		{[]string{"--git-dir", "/r/.git", "--work-tree", "/r", "log"}, "log"},
		{[]string{"--git-dir=/r/.git", "--no-pager", "-C", "x", "-c", "a=b", "worktree", "list"}, "worktree"},
		{[]string{"--exec-path", "status"}, "status"}, // bare --exec-path takes no value
		{[]string{"-C", "status", "fetch"}, "fetch"},  // "status" is the -C value, not the subcommand
		{[]string{"-c", "diff", "show"}, "show"},
		{[]string{"--no-optional-locks", "-p", "rev-parse", "HEAD"}, "rev-parse"},
		{[]string{"https://u:tok@host/x.git"}, "other"},
		{[]string{"frobnicate"}, "other"},
		{[]string{"--version"}, "none"},
		{nil, "none"},
		{[]string{"-C"}, "none"}, // dangling option value
	}
	for _, c := range cases {
		if got := GitSubcommand(c.args); got != c.want {
			t.Errorf("GitSubcommand(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestIsGit(t *testing.T) {
	for name, want := range map[string]bool{"git": true, "/usr/bin/git": true, "/opt/homebrew/bin/git": true, "tmux": false, "git-lfs": false, "digit": false} {
		if got := isGit(name); got != want {
			t.Errorf("isGit(%q) = %v, want %v", name, got, want)
		}
	}
}

// Plan Story 1.3.2 AC2: building a git command through safeexec bumps the backstop by one,
// keyed on the subcommand; building anything else does not.
func TestCommandContextCountsGitSpawnsOnly(t *testing.T) {
	before := backstopSum(t, "status")
	_ = CommandContext(context.Background(), "git", "-C", "/x", "status")
	_ = CommandContextPG(context.Background(), "/usr/bin/git", "status")
	if got := backstopSum(t, "status") - before; got != 2 {
		t.Fatalf("backstop{status} grew by %d, want 2 (CommandContext and CommandContextPG, one each)", got)
	}
	beforeOther := backstopSum(t, "other") + backstopSum(t, "none")
	_ = CommandContext(context.Background(), "tmux", "status")
	if got := backstopSum(t, "status") - before; got != 2 {
		t.Fatalf("a non-git command moved the backstop to +%d", got)
	}
	if backstopSum(t, "other")+backstopSum(t, "none") != beforeOther {
		t.Fatal("a non-git command was counted under other/none")
	}
}

// Plan Story 1.3.2 AC3: with SSQ_GIT_SPAWN_DUMP set each spawn's file:line is dumped.
func TestSpawnDumpRecordsCallerFileAndLine(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "d.log")
	t.Setenv(SpawnDumpEnv, dump)
	_ = CommandContext(context.Background(), "git", "status")
	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "exec ") || !strings.Contains(line, "spawncount_test.go:") || !strings.HasSuffix(line, "subcommand=status") {
		t.Fatalf("dump line = %q", line)
	}
}
