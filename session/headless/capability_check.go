package headless

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// capabilityCheckMarkerValue is written to a throwaway temp file and asked to be
// echoed back by the headless call under test. Distinct/unlikely-to-collide so a
// substring match on the result is unambiguous evidence the file was actually read.
const capabilityCheckMarkerValue = "STAPLER_SQUAD_CAPABILITY_CHECK_9f3a2b71"

// capabilityCheckTimeout bounds the one-off smoke-test call so a hung/degraded
// claude CLI doesn't block the first real codebase-read review indefinitely.
const capabilityCheckTimeout = 30 * time.Second

// capabilityCheckFailureCacheWindow bounds how long a FAILED smoke-test result is
// trusted before Ensure re-attempts the probe. A transient hiccup (subprocess
// spawn flake, momentary env issue) must not permanently poison every future
// codebase-read review for the rest of the process's lifetime — see the incident
// that motivated this: bd337ac9 stuck UNVERIFIABLE with no real check attempted
// after one early failure. 20 minutes is long enough that a real, persistent
// misconfiguration doesn't cause the probe subprocess to run on every single
// review (reviews fire far more often than every 20m), but short enough that a
// genuinely transient failure self-heals within roughly one engineer's coffee
// break rather than requiring a destructive full server restart.
//
// A cached SUCCESS is trusted for the life of the process (no expiry): once a
// WorkDir+AllowedTools+PermissionMode call is empirically shown to grant read
// access, that capability is a property of the process's fixed claude CLI/config,
// which is not expected to regress mid-process without an explicit config change —
// and any such change is worth re-verifying via a fresh check.Checked()-aware
// process restart, not a background timer.
const capabilityCheckFailureCacheWindow = 20 * time.Minute

// CodebaseReadCapabilitySelfCheck lazily verifies that a WorkDir+AllowedTools+
// PermissionMode headless call actually grants read access — the same empirical
// fact TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess checks in CI,
// re-verified here against the actual running process's claude CLI/config.
//
// A zero-value CodebaseReadCapabilitySelfCheck is ready to use. A successful smoke
// test result is cached for the life of the process; a failed one is cached only
// for capabilityCheckFailureCacheWindow, after which Ensure re-runs the probe
// rather than trusting a stale failure forever. Construct a fresh instance (rather
// than reusing DefaultCapabilitySelfCheck) when a test needs to exercise the check
// logic in isolation from other tests/production callers.
type CodebaseReadCapabilitySelfCheck struct {
	mu        sync.Mutex
	ok        atomic.Bool
	checked   atomic.Bool
	checkedAt atomic.Int64 // UnixNano of the last completed probe; 0 if never run.

	// now is overridable in tests so failure-window expiry can be exercised
	// deterministically without a real sleep. Nil means time.Now.
	now func() time.Time
}

func (c *CodebaseReadCapabilitySelfCheck) clockNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// resultFresh reports whether the currently cached result (if any) is still
// trusted: a cached success always is; a cached failure only within
// capabilityCheckFailureCacheWindow of when it was recorded.
func (c *CodebaseReadCapabilitySelfCheck) resultFresh() bool {
	if !c.checked.Load() {
		return false
	}
	if c.ok.Load() {
		return true
	}
	checkedAt := time.Unix(0, c.checkedAt.Load())
	return c.clockNow().Sub(checkedAt) < capabilityCheckFailureCacheWindow
}

// DefaultCapabilitySelfCheck is the package-level singleton shared by production
// callers (ReviewGateRunner and TriggerReReview) so a failure discovered via one
// call site short-circuits the other too. Callers that need test isolation should
// hold their own *CodebaseReadCapabilitySelfCheck field defaulting to this value
// instead of calling through the package var directly.
var DefaultCapabilitySelfCheck = &CodebaseReadCapabilitySelfCheck{}

// Ensure returns the cached result if it is still fresh (see resultFresh), or
// else runs the marker-file smoke test — blocking concurrent callers until it
// resolves — and caches the new result. pool is accepted as the narrow
// PoolClient interface so both *Pool (ReviewGateRunner) and interface-typed
// fields (BacklogService.headlessPool) can call it without an adapter.
func (c *CodebaseReadCapabilitySelfCheck) Ensure(ctx context.Context, pool PoolClient) bool {
	if c.resultFresh() {
		return c.ok.Load()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-check under the lock: another goroutine may have just refreshed the
	// result while we were waiting for it.
	if c.resultFresh() {
		return c.ok.Load()
	}

	ok := c.run(pool)
	c.ok.Store(ok)
	c.checked.Store(true)
	c.checkedAt.Store(c.clockNow().UnixNano())
	if ok {
		log.InfoLog().Printf("[headless] codebase-read capability self-check passed")
	} else {
		log.WarningLog().Printf("[headless] codebase-read capability self-check FAILED (cached for %s)", capabilityCheckFailureCacheWindow)
	}
	return ok
}

// Checked reports whether the self-check has run (successfully or not) yet.
func (c *CodebaseReadCapabilitySelfCheck) Checked() bool {
	return c.checked.Load()
}

// NewPassedCapabilitySelfCheckForTesting returns a CodebaseReadCapabilitySelfCheck
// pre-marked as passed, so Ensure returns true immediately without invoking
// pool.CallBlocking at all. For tests exercising the codebase-read call itself
// (ReviewGateRunner / TriggerReReview) that would otherwise have their mocked pool's
// canned response consumed by (and very likely fail) the capability smoke test —
// since a scripted verdict response generally won't happen to contain the
// self-check's marker string.
func NewPassedCapabilitySelfCheckForTesting() *CodebaseReadCapabilitySelfCheck {
	c := &CodebaseReadCapabilitySelfCheck{}
	c.ok.Store(true)
	c.checked.Store(true)
	c.checkedAt.Store(time.Now().UnixNano())
	return c
}

// NewFailedCapabilitySelfCheckForTesting returns a CodebaseReadCapabilitySelfCheck
// pre-marked as failed, so Ensure returns false immediately without invoking
// pool.CallBlocking. For tests exercising the capability-self-check-failure
// degrade path without needing to script a failing fake claude subprocess.
func NewFailedCapabilitySelfCheckForTesting() *CodebaseReadCapabilitySelfCheck {
	c := &CodebaseReadCapabilitySelfCheck{}
	c.ok.Store(false)
	c.checked.Store(true)
	c.checkedAt.Store(time.Now().UnixNano())
	return c
}

// run performs the actual marker-file smoke test: write a marker file to a
// throwaway temp dir, ask the model to read it back verbatim via a
// WorkDir+AllowedTools+PermissionMode call (the same call shape
// BuildReviewCallOptions grants on the empty-diff codebase-read path), and check
// the marker content round-trips.
//
// run takes no ctx parameter deliberately: each result run() produces is cached
// (permanently on success, for capabilityCheckFailureCacheWindow on failure) and
// seeds every caller that races to invoke Ensure while that cache is fresh. If the
// probe's context were derived from the winning caller's (possibly short-lived,
// per-review) ctx, a transient cancellation/deadline on that one caller would
// poison the cached verdict for every other caller sharing it, even though the
// underlying capability is fine. Deriving from context.Background() (bounded only
// by capabilityCheckTimeout) ensures the cached verdict reflects a real capability
// determination, not an artifact of one caller's context lifetime — so there is no
// legitimate use for a ctx parameter here.
func (c *CodebaseReadCapabilitySelfCheck) run(pool PoolClient) bool {
	if pool == nil {
		return false
	}

	tempDir, err := os.MkdirTemp("", "capability-check-*")
	if err != nil {
		log.WarningLog().Printf("[headless] codebase-read capability self-check: MkdirTemp failed: %v", err)
		return false
	}
	defer os.RemoveAll(tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, "marker.txt"), []byte(capabilityCheckMarkerValue), 0o600); err != nil {
		log.WarningLog().Printf("[headless] codebase-read capability self-check: WriteFile failed: %v", err)
		return false
	}

	checkCtx, cancel := context.WithTimeout(context.Background(), capabilityCheckTimeout)
	defer cancel()

	result, err := pool.CallBlocking(checkCtx, FeatureKeyCustom, "",
		"Read the file marker.txt in your current working directory and output ONLY its exact contents, nothing else.",
		CallOptions{
			WorkDir:        tempDir,
			AllowedTools:   CodebaseReadAllowedTools,
			PermissionMode: "bypassPermissions",
		}, DiscardCost)
	if err != nil {
		log.WarningLog().Printf("[headless] codebase-read capability self-check: CallBlocking failed: %v", err)
		return false
	}
	return strings.Contains(result, capabilityCheckMarkerValue)
}
