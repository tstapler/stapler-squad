package github

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// ghRunOutput runs `gh <args...> --repo <ref>` and returns stdout, folding
// stderr into the error so callers see why gh refused.
func ghRunOutput(ctx context.Context, ref RepoRef, callSite string, timeout time.Duration, args ...string) ([]byte, error) {
	if err := CheckGHAuthForHost(ctx, ref.Host()); err != nil {
		return nil, err
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := safeexec.CommandContext(cmdCtx, "gh", append(args, "--repo", ghRepoArg(ref))...)
	setGHHostEnv(cmd, ref.Host())
	out, err := runGHCLICommand(cmdCtx, callSite, func() ([]byte, error) { return cmd.Output() })
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %s", callSite, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("%s: %w", callSite, err)
	}
	return out, nil
}

// GetPRChangedFiles returns the repo-relative paths a pull request changes.
func GetPRChangedFiles(ctx context.Context, ref RepoRef, prNumber int) ([]string, error) {
	out, err := ghRunOutput(ctx, ref, "pr.diff.name-only", 30*time.Second, "pr", "diff", strconv.Itoa(prNumber), "--name-only")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// GetRunFailedLogs returns the log output of the failed steps of a workflow run.
func GetRunFailedLogs(ctx context.Context, ref RepoRef, runID int64) (string, error) {
	out, err := ghRunOutput(ctx, ref, "run.view.log-failed", 60*time.Second, "run", "view", strconv.FormatInt(runID, 10), "--log-failed")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// RerunFailedChecks re-runs only the failed jobs of a workflow run.
func RerunFailedChecks(ctx context.Context, ref RepoRef, runID int64) error {
	_, err := ghRunOutput(ctx, ref, "run.rerun.failed", 30*time.Second, "run", "rerun", strconv.FormatInt(runID, 10), "--failed")
	return err
}
