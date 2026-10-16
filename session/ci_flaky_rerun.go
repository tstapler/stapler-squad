package session

import (
	"context"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// ciRerunClient is the slice of GitHub Actions access maybeFlakyCheckRerun
// needs; tests substitute a fake.
type ciRerunClient interface {
	ChangedFiles(ctx context.Context, repoPath string, prNumber int) ([]string, error)
	FailedLogs(ctx context.Context, repoPath string, runID int64) (string, error)
	RerunFailed(ctx context.Context, repoPath string, runID int64) error
}

type ghCIRerunClient struct{}

func (ghCIRerunClient) ref(repoPath string) (github.RepoRef, error) {
	return github.GetOwnerRepoFromRemote(repoPath, enterpriseHostsForRemoteParsing())
}

func (c ghCIRerunClient) ChangedFiles(ctx context.Context, repoPath string, prNumber int) ([]string, error) {
	ref, err := c.ref(repoPath)
	if err != nil {
		return nil, err
	}
	return github.GetPRChangedFiles(ctx, ref, prNumber)
}

func (c ghCIRerunClient) FailedLogs(ctx context.Context, repoPath string, runID int64) (string, error) {
	ref, err := c.ref(repoPath)
	if err != nil {
		return "", err
	}
	return github.GetRunFailedLogs(ctx, ref, runID)
}

func (c ghCIRerunClient) RerunFailed(ctx context.Context, repoPath string, runID int64) error {
	ref, err := c.ref(repoPath)
	if err != nil {
		return err
	}
	return github.RerunFailedChecks(ctx, ref, runID)
}

// ciRerunLedger remembers which Actions runs were already re-run per item, so
// a check that still fails after its one free rerun falls through to a fix
// session. In-memory: a restart grants one extra free rerun, which is harmless.
type ciRerunLedger struct {
	mu    sync.Mutex
	rerun map[string]map[int64]struct{}
}

func (g *ciRerunLedger) attempted(itemID string, runID int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.rerun[itemID][runID]
	return ok
}

func (g *ciRerunLedger) record(itemID string, runID int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.rerun == nil {
		g.rerun = map[string]map[int64]struct{}{}
	}
	if g.rerun[itemID] == nil {
		g.rerun[itemID] = map[int64]struct{}{}
	}
	g.rerun[itemID][runID] = struct{}{}
}

func (g *ciRerunLedger) clear(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.rerun, itemID)
}

var (
	// Gradle/JUnit: "OutlinerRegressionTest > testBasicOutlinerFlow FAILED".
	jvmFailedRe = regexp.MustCompile(`(?m)(?:^|\s)([A-Za-z_][\w$]*) > .*\bFAILED\b`)
	// Go: "--- FAIL: TestCreateSession (0.03s)".
	goFailRe = regexp.MustCompile(`--- FAIL: (Test\w+)`)
	// pytest: "FAILED path/test_foo.py::TestBar::test_baz".
	pytestFailRe = regexp.MustCompile(`FAILED\s+(?:\S*/)?(\w+)\.py::`)
)

// extractTestClassesFromLog returns the distinct failing test identifiers
// (JVM class, Go test func, pytest module) found in a failed-step log. Empty
// means nothing identifiable — compile error, infra failure, etc.
func extractTestClassesFromLog(log string) []string {
	seen := map[string]bool{}
	var out []string
	for _, re := range []*regexp.Regexp{jvmFailedRe, goFailRe, pytestFailRe} {
		for _, m := range re.FindAllStringSubmatch(log, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// testFileInDiff reports whether the failing test identifier plausibly lives
// in a changed file. Conservative: a Go test func cannot be mapped to a file,
// so any changed _test.go file counts.
func testFileInDiff(class string, changedFiles []string) bool {
	for _, f := range changedFiles {
		base := strings.TrimSuffix(path.Base(f), path.Ext(f))
		if base == class {
			return true
		}
		if strings.HasPrefix(class, "Test") && strings.HasSuffix(f, "_test.go") {
			return true
		}
	}
	return false
}

// maybeFlakyCheckRerun re-runs failed Actions jobs instead of spawning a fix
// session when every identifiable failing test lies outside the PR's diff and
// the run was not already re-run. It reports true only when a rerun was
// issued; false always means "proceed to the fix-session path".
func (l *BacklogLifecycleListener) maybeFlakyCheckRerun(ctx context.Context, itemID, repoPath string, prNumber int, runIDs []int64) bool {
	client := l.ciRerunner
	if client == nil || len(runIDs) == 0 {
		return false
	}
	var pending []int64
	for _, id := range runIDs {
		if !l.ciReruns.attempted(itemID, id) {
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		return false
	}
	files, err := client.ChangedFiles(ctx, repoPath, prNumber)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] flaky-rerun item=%s: PR changed files: %v", itemID, err)
		return false
	}
	// Every failed run must yield identifiable, out-of-diff tests — including
	// already-rerun ones, which are still failing and so still need checking.
	for _, id := range runIDs {
		logText, logErr := client.FailedLogs(ctx, repoPath, id)
		if logErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] flaky-rerun item=%s run=%d: failed logs: %v", itemID, id, logErr)
			return false
		}
		classes := extractTestClassesFromLog(logText)
		if len(classes) == 0 {
			return false
		}
		for _, c := range classes {
			if testFileInDiff(c, files) {
				return false
			}
		}
	}
	for _, id := range pending {
		if rerr := client.RerunFailed(ctx, repoPath, id); rerr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] flaky-rerun item=%s run=%d: rerun: %v", itemID, id, rerr)
			return false
		}
		l.ciReruns.record(itemID, id)
		log.InfoLog().Printf("[BacklogLifecycle] flaky-rerun item=%s run=%d: failing tests are outside PR #%d's diff; re-ran failed jobs instead of spawning a fix session", itemID, id, prNumber)
	}
	return true
}
