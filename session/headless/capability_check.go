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

// CodebaseReadCapabilitySelfCheck lazily verifies, once per process lifetime, that a
// WorkDir+AllowedTools+PermissionMode headless call actually grants read access — the
// same empirical fact TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess checks
// in CI, re-verified here against the actual running process's claude CLI/config.
//
// A zero-value CodebaseReadCapabilitySelfCheck is ready to use. Each instance runs its
// underlying smoke test at most once (guarded by sync.Once); construct a fresh instance
// (rather than reusing DefaultCapabilitySelfCheck) when a test needs to exercise the
// check logic more than once within a process.
type CodebaseReadCapabilitySelfCheck struct {
	once    sync.Once
	ok      atomic.Bool
	checked atomic.Bool
}

// DefaultCapabilitySelfCheck is the package-level singleton shared by production
// callers (ReviewGateRunner and TriggerReReview) so a failure discovered via one
// call site short-circuits the other too. Callers that need test isolation should
// hold their own *CodebaseReadCapabilitySelfCheck field defaulting to this value
// instead of calling through the package var directly.
var DefaultCapabilitySelfCheck = &CodebaseReadCapabilitySelfCheck{}

// Ensure runs the once-guarded marker-file smoke test on first call (blocking
// concurrent callers until it resolves) and returns the cached result on every
// subsequent call. pool is accepted as the narrow PoolClient interface so both
// *Pool (ReviewGateRunner) and interface-typed fields (BacklogService.headlessPool)
// can call it without an adapter.
func (c *CodebaseReadCapabilitySelfCheck) Ensure(ctx context.Context, pool PoolClient) bool {
	c.once.Do(func() {
		ok := c.run(pool)
		c.ok.Store(ok)
		c.checked.Store(true)
		if ok {
			log.InfoLog.Printf("[headless] codebase-read capability self-check passed")
		} else {
			log.WarningLog.Printf("[headless] codebase-read capability self-check FAILED")
		}
	})
	return c.ok.Load()
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
	c.once.Do(func() {})
	c.ok.Store(true)
	c.checked.Store(true)
	return c
}

// NewFailedCapabilitySelfCheckForTesting returns a CodebaseReadCapabilitySelfCheck
// pre-marked as failed, so Ensure returns false immediately without invoking
// pool.CallBlocking. For tests exercising the capability-self-check-failure
// degrade path without needing to script a failing fake claude subprocess.
func NewFailedCapabilitySelfCheckForTesting() *CodebaseReadCapabilitySelfCheck {
	c := &CodebaseReadCapabilitySelfCheck{}
	c.once.Do(func() {})
	c.ok.Store(false)
	c.checked.Store(true)
	return c
}

// run performs the actual marker-file smoke test: write a marker file to a
// throwaway temp dir, ask the model to read it back verbatim via a
// WorkDir+AllowedTools+PermissionMode call (the same call shape
// BuildReviewCallOptions grants on the empty-diff codebase-read path), and check
// the marker content round-trips.
//
// run takes no ctx parameter deliberately: Ensure's result is cached for the
// lifetime of the process (sync.Once), seeded by whichever caller wins the race to
// run this method first. If the probe's context were derived from that caller's
// (possibly short-lived, per-review) ctx, a transient cancellation/deadline on the
// FIRST caller would permanently poison the process-lifetime cached verdict for
// every later caller, even though the underlying capability is fine. Deriving from
// context.Background() (bounded only by capabilityCheckTimeout) ensures the cached
// verdict reflects a real capability determination, not an artifact of the first
// caller's context lifetime — so there is no legitimate use for a ctx parameter here.
func (c *CodebaseReadCapabilitySelfCheck) run(pool PoolClient) bool {
	if pool == nil {
		return false
	}

	tempDir, err := os.MkdirTemp("", "capability-check-*")
	if err != nil {
		log.WarningLog.Printf("[headless] codebase-read capability self-check: MkdirTemp failed: %v", err)
		return false
	}
	defer os.RemoveAll(tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, "marker.txt"), []byte(capabilityCheckMarkerValue), 0o644); err != nil {
		log.WarningLog.Printf("[headless] codebase-read capability self-check: WriteFile failed: %v", err)
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
		log.WarningLog.Printf("[headless] codebase-read capability self-check: CallBlocking failed: %v", err)
		return false
	}
	return strings.Contains(result, capabilityCheckMarkerValue)
}
