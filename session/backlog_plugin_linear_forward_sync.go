package session

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Both queries select issue.updatedAt so the response carries Linear's own
// timestamp for the loop-prevention watermark (ADR-001), not local wall-clock.
const linearIssueStatesQuery = `query IssueStates($id: String!) {
  issue(id: $id) { id team { states { nodes { id name type } } } }
}`

const linearIssueUpdateMutation = `mutation IssueUpdate($id: String!, $stateId: String!) {
  issueUpdate(id: $id, input: {stateId: $stateId}) { success issue { updatedAt } }
}`

const linearCommentCreateMutation = `mutation CommentCreate($issueId: String!, $body: String!) {
  commentCreate(input: {issueId: $issueId, body: $body}) { success }
}`

func linearForwardSyncToken(config PluginConfig) (string, error) {
	cfg, disabled, err := decodeLinearFetchConfig(config)
	if err != nil {
		return "", err
	}
	if disabled {
		return "", fmt.Errorf("linear: API key not configured (store it in the keychain)")
	}
	return cfg.Token, nil
}

// UpdateIssueState moves the issue (identifier like "ENG-123") to the team
// workflow state named targetState (case-insensitive) and returns Linear's
// post-write updatedAt. An unknown state name is an ordinary error.
func (l *LinearPlugin) UpdateIssueState(ctx context.Context, config PluginConfig, externalID, targetState string) (time.Time, error) {
	token, err := linearForwardSyncToken(config)
	if err != nil {
		return time.Time{}, err
	}
	var states struct {
		Issue *struct {
			ID   string `json:"id"`
			Team struct {
				States struct {
					Nodes []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"states"`
			} `json:"team"`
		} `json:"issue"`
	}
	if err := doLinearGraphQL(ctx, token, linearIssueStatesQuery, map[string]any{"id": externalID}, &states); err != nil {
		return time.Time{}, err
	}
	if states.Issue == nil || states.Issue.ID == "" {
		return time.Time{}, fmt.Errorf("linear: issue %s not found", externalID)
	}
	stateID := ""
	for _, s := range states.Issue.Team.States.Nodes {
		if strings.EqualFold(s.Name, targetState) {
			stateID = s.ID
			break
		}
	}
	if stateID == "" {
		return time.Time{}, fmt.Errorf("linear: no workflow state named %q found", targetState)
	}

	var upd struct {
		IssueUpdate struct {
			Success bool `json:"success"`
			Issue   struct {
				UpdatedAt string `json:"updatedAt"`
			} `json:"issue"`
		} `json:"issueUpdate"`
	}
	if err := doLinearGraphQL(ctx, token, linearIssueUpdateMutation, map[string]any{"id": states.Issue.ID, "stateId": stateID}, &upd); err != nil {
		return time.Time{}, err
	}
	if !upd.IssueUpdate.Success {
		return time.Time{}, fmt.Errorf("linear: issueUpdate for %s reported success=false", externalID)
	}
	// An unparseable timestamp yields the zero time; the subscriber falls back
	// to local time for the watermark in that case.
	ts, _ := time.Parse(time.RFC3339, upd.IssueUpdate.Issue.UpdatedAt)
	return ts.UTC(), nil
}

// PostIssueComment adds a Markdown comment to the issue (identifier or id).
func (l *LinearPlugin) PostIssueComment(ctx context.Context, config PluginConfig, externalID, body string) error {
	token, err := linearForwardSyncToken(config)
	if err != nil {
		return err
	}
	var idOut struct {
		Issue *struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := doLinearGraphQL(ctx, token, `query($id: String!) { issue(id: $id) { id } }`, map[string]any{"id": externalID}, &idOut); err != nil {
		return err
	}
	if idOut.Issue == nil || idOut.Issue.ID == "" {
		return fmt.Errorf("linear: issue %s not found", externalID)
	}
	var out struct {
		CommentCreate struct {
			Success bool `json:"success"`
		} `json:"commentCreate"`
	}
	if err := doLinearGraphQL(ctx, token, linearCommentCreateMutation, map[string]any{"issueId": idOut.Issue.ID, "body": body}, &out); err != nil {
		return err
	}
	if !out.CommentCreate.Success {
		return fmt.Errorf("linear: commentCreate for %s reported success=false", externalID)
	}
	return nil
}
