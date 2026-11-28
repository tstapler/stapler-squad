package sessioncreate

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestModeGitHubResolver wraps session.ResolveGitHubInputCtxWithHosts with a
// small artificial floor on how fast it can return, but ONLY under
// config.IsIsolatedInstance() -- a no-op (returns the real resolver
// directly) in every production/dev run. Deliberately IsIsolatedInstance(),
// not the narrower IsTestMode(): the e2e harness's `stapler-squad --test-mode
// --test-dir ...` process is a real compiled binary (STAPLER_SQUAD_TEST_DIR
// set, not a `go test` binary), which IsTestMode() alone does not detect --
// confirmed the hard way, this wrapper was a silent no-op against the e2e
// server on the first attempt. Fixes a genuine e2e flake: the async creation
// pipeline's Creating -> Failed transition for a GitHub-404 (accessibility.spec.ts's
// "Cancel and Retry controls are keyboard-reachable" test, session-creation-async.spec.ts's
// Epic 5.3 toast test) is a REAL network round trip, and on a fast/warm
// connection it can complete in well under a second -- faster than a
// Playwright test can reliably observe the Creating state and drive real
// keyboard interaction against it (repro: 0-2 of 3 runs failed with the
// Cancel button already gone by the time the test got to use it, even after
// several rounds of test-side ordering/timing fixes). Those tests' own doc
// comments already flagged "no backend test-mode hook exists yet to simulate
// an actually-slow GitHub host resolution end-to-end" as a known gap -- this
// is that hook. Safe for `go test` runs too: every existing unit test that
// exercises this path overrides svc.githubResolver directly after
// construction (see session_creation_pipeline_test.go, session_service_test.go,
// session_service_create_test.go), which replaces this default outright, so
// none of them see the added delay. STAPLER_SQUAD_TEST_GITHUB_RESOLVE_MIN_MS
// overrides the floor (0 disables it entirely); unset defaults to 3000ms,
// comfortably inside every existing e2e timeout that waits on this
// transition (all >= 10s, most 30s).
func TestModeGitHubResolver() func(ctx context.Context, input string, enterpriseHosts []string) (string, *instance.GitHubRef, error) {
	if !config.IsIsolatedInstance() {
		return instance.ResolveGitHubInputCtxWithHosts
	}
	minDelay := 3 * time.Second
	if raw := os.Getenv("STAPLER_SQUAD_TEST_GITHUB_RESOLVE_MIN_MS"); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 0 {
			minDelay = time.Duration(ms) * time.Millisecond
		}
	}
	if minDelay == 0 {
		return instance.ResolveGitHubInputCtxWithHosts
	}
	return func(ctx context.Context, input string, enterpriseHosts []string) (string, *instance.GitHubRef, error) {
		start := time.Now()
		localPath, ref, err := instance.ResolveGitHubInputCtxWithHosts(ctx, input, enterpriseHosts)
		if elapsed := time.Since(start); elapsed < minDelay {
			select {
			case <-time.After(minDelay - elapsed):
			case <-ctx.Done():
			}
		}
		return localPath, ref, err
	}
}
