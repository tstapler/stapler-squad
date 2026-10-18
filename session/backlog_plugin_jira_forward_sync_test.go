package session

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJiraPlugin_UpdateIssueState_should_TransitionByNameAndReturnUpdated(t *testing.T) {
	var posted string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/ENG-1/transitions":
			_, _ = w.Write([]byte(`{"transitions":[{"id":"21","name":"Start","to":{"name":"In Progress"}},{"id":"31","name":"Finish","to":{"name":"Done"}}]}`))
		case r.Method == http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			posted = string(b)
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{"key":"ENG-1","fields":{"updated":"2026-10-16T12:00:00.000+0000"}}`))
		}
	}))
	defer ts.Close()
	cfg := jiraCfg(t, ts)

	got, err := NewJiraPlugin().UpdateIssueState(context.Background(), cfg, "ENG-1", "done")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), got)
	require.JSONEq(t, `{"transition":{"id":"31"}}`, posted)
}

func TestJiraPlugin_UpdateIssueState_should_Error_When_TransitionUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"transitions":[{"id":"21","name":"Start","to":{"name":"In Progress"}}]}`))
	}))
	defer ts.Close()
	_, err := NewJiraPlugin().UpdateIssueState(context.Background(), jiraCfg(t, ts), "ENG-1", "Shipped")
	require.ErrorContains(t, err, `no transition named "Shipped"`)
}

func TestJiraPlugin_UpdateIssueState_should_Error_When_CredentialsMissing(t *testing.T) {
	clearJiraEnv(t)
	_, err := NewJiraPlugin().UpdateIssueState(context.Background(),
		PluginConfig{Raw: `{"base_url":"https://nocreds.atlassian.net"}`}, "ENG-1", "Done")
	require.ErrorContains(t, err, "credentials not configured")
}

func TestJiraPlugin_PostIssueComment_should_PostADFBody(t *testing.T) {
	var path, body string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()
	err := NewJiraPlugin().PostIssueComment(context.Background(), jiraCfg(t, ts), "ENG-1", "hello")
	require.NoError(t, err)
	require.Equal(t, "/rest/api/3/issue/ENG-1/comment", path)
	require.Contains(t, body, `"type":"doc"`)
	require.Contains(t, body, "hello")
}
