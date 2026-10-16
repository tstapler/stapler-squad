package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/jira"
)

// UpdateIssueState transitions the issue to the workflow transition whose name
// (or destination status name) matches targetState, case-insensitively, and
// returns JIRA's post-write `updated` time for the loop-prevention watermark.
// JIRA transitions are workflow-specific, so an unknown name is an ordinary
// error rather than a guess.
func (j *JiraPlugin) UpdateIssueState(ctx context.Context, config PluginConfig, externalID, targetState string) (time.Time, error) {
	cfg, err := decodeJiraIssueConfig(config)
	if err != nil {
		return time.Time{}, err
	}
	issuePath := "/rest/api/3/issue/" + url.PathEscape(externalID)

	data, err := jiraIssueRequest(ctx, cfg, http.MethodGet, issuePath+"/transitions", nil)
	if err != nil {
		return time.Time{}, err
	}
	var list struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return time.Time{}, fmt.Errorf("jira: decode transitions: %w", err)
	}
	transitionID := ""
	for _, t := range list.Transitions {
		if strings.EqualFold(t.Name, targetState) || strings.EqualFold(t.To.Name, targetState) {
			transitionID = t.ID
			break
		}
	}
	if transitionID == "" {
		return time.Time{}, fmt.Errorf("jira: no transition named %q available for %s", targetState, externalID)
	}
	if _, err := jiraIssueRequest(ctx, cfg, http.MethodPost, issuePath+"/transitions",
		map[string]any{"transition": map[string]string{"id": transitionID}}); err != nil {
		return time.Time{}, err
	}

	// The transition POST returns 204 with no body, so read `updated` back.
	data, err = jiraIssueRequest(ctx, cfg, http.MethodGet, issuePath+"?fields=updated", nil)
	if err != nil {
		return time.Time{}, err
	}
	var got jiraIssue
	if err := json.Unmarshal(data, &got); err != nil {
		return time.Time{}, fmt.Errorf("jira: decode issue: %w", err)
	}
	// An unparseable timestamp yields the zero time; the subscriber falls back
	// to local time for the watermark in that case.
	ts, _ := parseJiraTime(got.Fields.Updated)
	return ts.UTC(), nil
}

// PostIssueComment adds a comment (converted to ADF) to the issue.
func (j *JiraPlugin) PostIssueComment(ctx context.Context, config PluginConfig, externalID, body string) error {
	cfg, err := decodeJiraIssueConfig(config)
	if err != nil {
		return err
	}
	_, err = jiraIssueRequest(ctx, cfg, http.MethodPost,
		"/rest/api/3/issue/"+url.PathEscape(externalID)+"/comment",
		map[string]any{"body": jira.PlainTextToADF(body)})
	return err
}
