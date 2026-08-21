package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// CommitResult is the domain return type for GetCommit.
type CommitResult struct {
	SHA     string
	HTMLURL string
	Message string
	Author  string
}

// ghCommitJSON matches the minimal fields this package needs from the GitHub
// REST API /repos/{owner}/{repo}/commits/{sha} response.
type ghCommitJSON struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commit"`
}

// GetCommit fetches a single commit by SHA via the GitHub REST API (native
// net/http, same auth/error-classification mechanism as GetIssue/GetPR).
// account.Host "" means github.com; account.Username, when non-empty, selects
// among multiple connected accounts for that host (see getGHTokenForAccount).
// Returns ErrNotAuthenticated when no token is configured.
func GetCommit(ctx context.Context, account AccountRef, repo RepoRef, sha string) (*CommitResult, error) {
	token := getGHTokenForAccount(ctx, account)
	if token == "" {
		return nil, ErrNotAuthenticated
	}

	apiPath := fmt.Sprintf("repos/%s/%s/commits/%s", url.PathEscape(repo.Owner()), url.PathEscape(repo.Repo()), url.PathEscape(sha))

	req, err := newGHRequestForHostWithToken(ctx, account.Host, apiPath, token)
	if err != nil {
		return nil, fmt.Errorf("build commit request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("commit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "commit not found (404)", true)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read commit response: %w", err)
	}

	var item ghCommitJSON
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("parse commit response: %w", err)
	}

	return &CommitResult{
		SHA:     item.SHA,
		HTMLURL: item.HTMLURL,
		Message: item.Commit.Message,
		Author:  item.Commit.Author.Name,
	}, nil
}
