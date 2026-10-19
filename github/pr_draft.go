package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	mutationConvertToDraft = `mutation($id: ID!) { convertPullRequestToDraft(input: {pullRequestId: $id}) { pullRequest { isDraft } } }`
	mutationMarkReady      = `mutation($id: ID!) { markPullRequestReadyForReview(input: {pullRequestId: $id}) { pullRequest { isDraft } } }`
)

// ConvertPRToDraft marks the PR with the given GraphQL node ID as draft.
// There is no gh CLI equivalent, so this goes straight to GraphQL with token.
func ConvertPRToDraft(ctx context.Context, host, nodeID, token string) error {
	return runPRMutation(ctx, host, token, mutationConvertToDraft, nodeID)
}

// MarkPRReadyForReview marks the draft PR with the given node ID ready for review.
func MarkPRReadyForReview(ctx context.Context, host, nodeID, token string) error {
	return runPRMutation(ctx, host, token, mutationMarkReady, nodeID)
}

func runPRMutation(ctx context.Context, host, token, query, nodeID string) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": map[string]string{"id": nodeID}})
	if err != nil {
		return fmt.Errorf("marshal GraphQL mutation: %w", err)
	}
	req, err := newGHGraphQLRequestForHostWithToken(ctx, host, body, token)
	if err != nil {
		return fmt.Errorf("build GraphQL request: %w", err)
	}
	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()
	if isGHRateLimited(resp) {
		_, _ = io.Copy(io.Discard, resp.Body)
		return errAccountRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("GraphQL API returned status %d", resp.StatusCode)
	}
	var out struct {
		Errors []graphQLError `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode GraphQL response: %w", err)
	}
	if len(out.Errors) > 0 {
		msgs := make([]string, len(out.Errors))
		for i, e := range out.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("GitHub rejected the mutation: %s", strings.Join(msgs, "; "))
	}
	return nil
}
