package session

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const linearStatesJSON = `{"data":{"issue":{"id":"uuid-1","team":{"states":{"nodes":[{"id":"s1","name":"In Review"},{"id":"s2","name":"Done"}]}}}}}`

func TestLinearPlugin_UpdateIssueState_should_ResolveStateAndReturnLinearTimestamp(t *testing.T) {
	setLinearKey(t, "k")
	var mutationBody string
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "issueUpdate") {
			mutationBody = string(b)
			_, _ = w.Write([]byte(`{"data":{"issueUpdate":{"success":true,"issue":{"updatedAt":"2026-10-16T12:00:00Z"}}}}`))
			return
		}
		_, _ = w.Write([]byte(linearStatesJSON))
	})
	ts, err := NewLinearPlugin().UpdateIssueState(context.Background(), PluginConfig{}, "ENG-1", "done")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), ts)
	require.Contains(t, mutationBody, `"stateId":"s2"`)
	require.Contains(t, mutationBody, `"id":"uuid-1"`)
}

func TestLinearPlugin_UpdateIssueState_should_Error_When_StateNameUnknown(t *testing.T) {
	setLinearKey(t, "k")
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(linearStatesJSON))
	})
	_, err := NewLinearPlugin().UpdateIssueState(context.Background(), PluginConfig{}, "ENG-1", "Shipped")
	require.ErrorContains(t, err, `no workflow state named "Shipped" found`)
}

func TestLinearPlugin_UpdateIssueState_should_Error_When_KeyMissing(t *testing.T) {
	setLinearKey(t, "")
	_, err := NewLinearPlugin().UpdateIssueState(context.Background(), PluginConfig{}, "ENG-1", "Done")
	require.ErrorContains(t, err, "API key not configured")
}

func TestLinearPlugin_PostIssueComment_should_CreateComment(t *testing.T) {
	setLinearKey(t, "k")
	var commentBody string
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "commentCreate") {
			commentBody = string(b)
			_, _ = w.Write([]byte(`{"data":{"commentCreate":{"success":true}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"issue":{"id":"uuid-1"}}}`))
	})
	require.NoError(t, NewLinearPlugin().PostIssueComment(context.Background(), PluginConfig{}, "ENG-1", "hello"))
	require.Contains(t, commentBody, `"issueId":"uuid-1"`)
	require.Contains(t, commentBody, `"body":"hello"`)
}

func TestLinearPlugin_PostIssueComment_should_Error_When_IssueNotFound(t *testing.T) {
	setLinearKey(t, "k")
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"issue":null}}`))
	})
	require.ErrorContains(t, NewLinearPlugin().PostIssueComment(context.Background(), PluginConfig{}, "ENG-9", "x"), "not found")
}
