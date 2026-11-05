package svctest

import (
	"net/http/httptest"

	gh "github.com/tstapler/stapler-squad/github"
)

// resetGhBaseURL overrides GhBaseURL for a test and returns a restore func.
func ResetGhBaseURL(ts *httptest.Server) func() {
	return gh.SetGhBaseURLForTest(ts.URL + "/")
}
