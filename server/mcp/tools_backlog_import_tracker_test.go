package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func importedItemID(t *testing.T, text string) string {
	t.Helper()
	var id string
	_, err := fmt.Sscanf(text, "Imported backlog item %s", &id)
	require.NoError(t, err)
	return strings.TrimSuffix(id, ":")
}

func TestImportLinearIssue_should_CreateItemFromIssue(t *testing.T) {
	storage := newTestBacklogStorage(t)
	var gotID string
	h := &backlogHandlers{storage: storage, fetchLinearIssue: func(_ context.Context, id string) (session.ExternalItem, error) {
		gotID = id
		return session.ExternalItem{ExternalID: "ENG-123", Title: "Fix login", Description: "body", Labels: []string{"bug"}, Priority: 5, URL: "https://linear.app/acme/issue/ENG-123/fix-login"}, nil
	}}
	res, err := h.importLinearIssue(context.Background(), makeToolReq(map[string]interface{}{"issue_url": "https://linear.app/acme/issue/ENG-123/fix-login"}))
	require.NoError(t, err)
	assert.Equal(t, "ENG-123", gotID)

	item, err := storage.GetBacklogItem(context.Background(), importedItemID(t, toolText(t, res)))
	require.NoError(t, err)
	assert.Equal(t, "Fix login", item.Title)
	assert.Equal(t, "ENG-123", item.ExternalID)
	assert.Equal(t, "https://linear.app/acme/issue/ENG-123/fix-login", item.ExternalURL)
	assert.Equal(t, 5, item.Priority)
	assert.Contains(t, item.Notes, "Linear")
}

func TestImportLinearIssue_should_RejectBadInput_And_SurfaceFetchErrors(t *testing.T) {
	h := &backlogHandlers{storage: newTestBacklogStorage(t), fetchLinearIssue: func(context.Context, string) (session.ExternalItem, error) {
		return session.ExternalItem{}, errors.New("linear: authentication failed (401)")
	}}
	for _, args := range []map[string]interface{}{{}, {"issue_url": "https://example.com/x"}} {
		res, err := h.importLinearIssue(context.Background(), makeToolReq(args))
		require.NoError(t, err)
		assert.False(t, parseResult(t, res)["success"].(bool), "%v", args)
	}
	res, err := h.importLinearIssue(context.Background(), makeToolReq(map[string]interface{}{"issue_url": "ENG-1"}))
	require.NoError(t, err)
	assert.False(t, parseResult(t, res)["success"].(bool))
	assert.Contains(t, toolText(t, res), "authentication failed")
}

func TestImportJiraIssue_should_CreateItemFromURL_And_BareKeyWithBaseURL(t *testing.T) {
	storage := newTestBacklogStorage(t)
	var gotBase, gotKey string
	h := &backlogHandlers{storage: storage, fetchJiraIssue: func(_ context.Context, base, key string) (session.ExternalItem, error) {
		gotBase, gotKey = base, key
		return session.ExternalItem{ExternalID: key, Title: "Crash", Description: "stack", Priority: 4, URL: base + "/browse/" + key}, nil
	}}

	res, err := h.importJiraIssue(context.Background(), makeToolReq(map[string]interface{}{"issue_url": "https://acme.atlassian.net/browse/ENG-9"}))
	require.NoError(t, err)
	assert.Equal(t, "https://acme.atlassian.net", gotBase)
	assert.Equal(t, "ENG-9", gotKey)
	item, err := storage.GetBacklogItem(context.Background(), importedItemID(t, toolText(t, res)))
	require.NoError(t, err)
	assert.Equal(t, "https://acme.atlassian.net/browse/ENG-9", item.ExternalURL)

	res, err = h.importJiraIssue(context.Background(), makeToolReq(map[string]interface{}{"issue_url": "ENG-10", "base_url": "https://other.atlassian.net"}))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), "Imported backlog item")
	assert.Equal(t, "https://other.atlassian.net", gotBase)

	res, err = h.importJiraIssue(context.Background(), makeToolReq(map[string]interface{}{"issue_url": "ENG-11"}))
	require.NoError(t, err)
	assert.False(t, parseResult(t, res)["success"].(bool), "bare key without base_url must be rejected")
}
