package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// PostPRCommentREST posts body as a comment on pull request prNumber via the
// native REST API (POST /repos/{owner}/{repo}/issues/{n}/comments), resolving
// the token like the other native calls. Unlike PostPRComment (gh CLI) it needs
// no gh binary. Returns ErrNotAuthenticated when no token is configured.
func PostPRCommentREST(ctx context.Context, repo RepoRef, prNumber int, body string) error {
	token := getGHTokenForAccount(ctx, AccountRef{Host: repo.Host()})
	if token == "" {
		return ErrNotAuthenticated
	}
	return postPRCommentWithToken(ctx, repo, prNumber, body, token)
}

func postPRCommentWithToken(ctx context.Context, repo RepoRef, prNumber int, body, token string) error {
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return fmt.Errorf("encode PR comment: %w", err)
	}
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", url.PathEscape(repo.Owner()), url.PathEscape(repo.Repo()), prNumber)
	req, err := newGHRequestForHostWithTokenAndBody(ctx, repo.Host(), http.MethodPost, path, token, payload)
	if err != nil {
		return fmt.Errorf("build PR comment request: %w", err)
	}
	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("PR comment request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("post PR comment: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
