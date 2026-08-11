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
