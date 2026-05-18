package github

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"golang.org/x/sync/singleflight"
)

// ghHTTPClient is the shared HTTP client used for all native GitHub REST calls.
// The 30-second timeout matches the existing gh CLI call timeout.
var ghHTTPClient = &http.Client{Timeout: 30 * time.Second}

const ghTokenTTL = time.Hour

type tokenResult struct {
	token  string
	expiry time.Time
}

var (
	ghTokenState atomic.Value       // stores tokenResult
	ghTokenGroup singleflight.Group //nolint:exhaustruct
)

// getGHToken returns a GitHub personal access token for native HTTP calls.
// Precedence: GITHUB_TOKEN env → GH_TOKEN env → gh auth token (cached 1 hour).
// Returns an empty string (not an error) when no token source is available so
// callers can decide whether to degrade gracefully.
func getGHToken(ctx context.Context) string {
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		return tok
	}
	if tok := os.Getenv("GH_TOKEN"); tok != "" {
		return tok
	}

	if v := ghTokenState.Load(); v != nil {
		if r := v.(tokenResult); time.Now().Before(r.expiry) {
			return r.token
		}
	}

	res, _, _ := ghTokenGroup.Do("token", func() (interface{}, error) {
		cmd := safeexec.CommandContext(ctx, "gh", "auth", "token")
		out, err := cmd.Output()
		tok := ""
		if err == nil {
			tok = strings.TrimSpace(string(out))
		}
		ghTokenState.Store(tokenResult{token: tok, expiry: time.Now().Add(ghTokenTTL)})
		return tok, nil
	})
	if res == nil {
		return ""
	}
	return res.(string)
}

// newGHRequest creates an authenticated GET request to the GitHub REST API.
func newGHRequest(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/"+path, nil)
	if err != nil {
		return nil, err
	}
	if tok := getGHToken(ctx); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}
