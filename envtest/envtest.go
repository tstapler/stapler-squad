// Package envtest holds tiny, dependency-free helpers shared by TestMain
// functions across packages. It intentionally imports nothing from this repo
// so it can be used from internal test files (package foo, not foo_test)
// without risking an import cycle back into the package under test.
package envtest

import "os"

// ClearAmbientGitHubTokenEnv clears GITHUB_TOKEN/GH_TOKEN for the life of a
// TestMain run and returns a func to restore their original values
// afterward. Without this, a developer machine's or CI runner's ambient
// token can leak into UserPRCache and trigger a real, unmocked dial to the
// GitHub API mid-test-suite.
func ClearAmbientGitHubTokenEnv() (restore func()) {
	origGithubToken, hadGithubToken := os.LookupEnv("GITHUB_TOKEN")
	origGhToken, hadGhToken := os.LookupEnv("GH_TOKEN")
	_ = os.Unsetenv("GITHUB_TOKEN")
	_ = os.Unsetenv("GH_TOKEN")
	return func() {
		if hadGithubToken {
			_ = os.Setenv("GITHUB_TOKEN", origGithubToken)
		}
		if hadGhToken {
			_ = os.Setenv("GH_TOKEN", origGhToken)
		}
	}
}

// ClearAmbientStaplerSquadStateEnv clears STAPLER_SQUAD_TEST_DIR and
// STAPLER_SQUAD_INSTANCE for the life of a TestMain run and returns a func to
// restore their original values afterward.
//
// config.GetConfigDirForDir checks these two env vars (Priorities 1-2) before
// its own IsTestMode() per-PID auto-isolation (Priority 3), so a value
// left ambient in the shell a `go test` binary inherits from — e.g. a
// developer's terminal that also ran the e2e harness or a manual
// `STAPLER_SQUAD_INSTANCE=... ./stapler-squad` invocation earlier in the same
// session — silently wins over that auto-isolation. LoadConfig() then reads
// (or races to create) that other process's shared config.json instead of a
// fresh per-test default, and a DefaultProgram left empty/unexpected there
// fails Session.program's NotEmpty validator with no indication the failure
// has nothing to do with the test itself. Confirmed in the field: an
// interactive Claude Code session's own shell had both vars set from a prior
// e2e run, and every `go test ./server/services/...` invocation in that
// shell failed the same way regardless of which test ran first.
func ClearAmbientStaplerSquadStateEnv() (restore func()) {
	origTestDir, hadTestDir := os.LookupEnv("STAPLER_SQUAD_TEST_DIR")
	origInstance, hadInstance := os.LookupEnv("STAPLER_SQUAD_INSTANCE")
	_ = os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
	_ = os.Unsetenv("STAPLER_SQUAD_INSTANCE")
	return func() {
		if hadTestDir {
			_ = os.Setenv("STAPLER_SQUAD_TEST_DIR", origTestDir)
		}
		if hadInstance {
			_ = os.Setenv("STAPLER_SQUAD_INSTANCE", origInstance)
		}
	}
}
