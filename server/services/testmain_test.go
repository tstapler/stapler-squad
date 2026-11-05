package services

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// TestMain pre-seeds headless.DefaultCapabilitySelfCheck as passed before any test
// runs. NewBacklogService defaults every instance's capabilityCheck field to that
// package-level singleton, which caches a successful result for the whole process
// lifetime in production (a cached failure is only trusted for a bounded window —
// see capability_check.go). Left unseeded, the first test in this binary to reach
// the codebase-read gate without calling SetCapabilityCheck resolves the
// singleton based on whether ITS OWN fakeHeadlessPool response happens to contain
// the capability marker string (it doesn't — the fakes return scripted verdict
// JSON) — poisoning it to failed for every other test in the package that runs
// within the failure-cache window, regardless of test order or -count. That was
// the actual root cause behind TestAutoRespawnReview_DeadWorkSession_TombstonedThenRespawns'
// order-dependent flake (reliably 1-pass-then-every-subsequent-run-fails under
// -count=N in one process). Tests that specifically exercise the capability-check
// failure/success path still override it per-instance via SetCapabilityCheck.
//
// It also clears GITHUB_TOKEN/GH_TOKEN for the whole test run, mirroring
// github/main_test.go's TestMain: this package drives UserPRCache directly
// (e.g. TestPreviewDestinationPath_GitHubURL_EnterpriseHostViaCachedAccount_ReturnsExactPath),
// and collectAllTokens reads both env vars straight from the environment, so
// a developer machine or CI runner with either set would otherwise leak a
// real token into the cache and dial the real GitHub API mid-suite.
//
// It also clears STAPLER_SQUAD_TEST_DIR/STAPLER_SQUAD_INSTANCE for the whole
// test run — see envtest.ClearAmbientStaplerSquadStateEnv's doc comment for
// why an ambient value of either silently defeats config.GetConfigDirForDir's
// own per-PID test isolation and fails session creation with an unrelated
// "Session.program" validator error.
func TestMain(m *testing.M) {
	os.Exit(svctest.Main(m, func() { lookupIPAddr = svctest.HermeticLookupIPAddr }))
}
