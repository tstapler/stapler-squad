package session

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/jira"
)

func jiraIssueJSON(key, updated, prio, desc string) string {
	p := "null"
	if prio != "" {
		p = fmt.Sprintf(`{"name":%q}`, prio)
	}
	return fmt.Sprintf(`{"key":%q,"fields":{"summary":"S %s","description":%s,"labels":["l1"],"updated":%q,"priority":%s}}`, key, key, desc, updated, p)
}

// jiraCfg returns a config for ts and supplies credentials via the env
// fallback; callers must not be parallel (t.Setenv).
func jiraCfg(t *testing.T, ts *httptest.Server) PluginConfig {
	t.Helper()
	t.Setenv("JIRA_EMAIL", "a@b.c")
	t.Setenv("JIRA_API_TOKEN", "jira_secret")
	t.Setenv("JIRA_BASE_URL", "")
	return PluginConfig{Raw: fmt.Sprintf(`{"base_url":%q,"project_key":"ENG"}`, ts.URL)}
}

func clearJiraEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"JIRA_EMAIL", "JIRA_API_TOKEN", "JIRA_BASE_URL"} {
		t.Setenv(k, "")
	}
}

func TestJiraPlugin_PluginID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "jira", NewJiraPlugin().PluginID())
}

func TestJiraPlugin_Fetch_should_ReturnEmpty_When_CredentialsMissing(t *testing.T) {
	clearJiraEnv(t)
	cfg := PluginConfig{Raw: `{"base_url":"https://nocreds.atlassian.net","project_key":"ENG"}`}
	items, cursor, err := NewJiraPlugin().Fetch(context.Background(), cfg, "old")
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, "old", cursor)
}

func TestJiraPlugin_Fetch_should_Error_When_RequiredConfigMissingOrInvalid(t *testing.T) {
	clearJiraEnv(t)
	t.Setenv("JIRA_EMAIL", "a@b.c")
	t.Setenv("JIRA_API_TOKEN", "t")
	for _, raw := range []string{
		`{"project_key":"ENG"}`,
		`{"base_url":"https://x.atlassian.net"}`,
		`{"base_url":"https://x.atlassian.net","project_key":"ENG OR 1=1"}`,
	} {
		_, _, err := NewJiraPlugin().Fetch(context.Background(), PluginConfig{Raw: raw}, "")
		require.Error(t, err, raw)
	}
}

func TestJiraPlugin_Fetch_should_BuildJQLFromCursor_And_UseBasicAuth(t *testing.T) {
	var gotJQL, gotUser, gotPass, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotJQL = r.URL.Query().Get("jql")
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		fmt.Fprint(w, `{"issues":[],"isLast":true}`)
	}))
	defer ts.Close()
	_, cursor, err := NewJiraPlugin().Fetch(context.Background(), jiraCfg(t, ts), "2026-07-01 00:00")
	require.NoError(t, err)
	require.Equal(t, `project = ENG AND updated >= "2026-07-01 00:00" ORDER BY updated ASC`, gotJQL)
	require.Equal(t, "/rest/api/3/search/jql", gotPath)
	require.Equal(t, "a@b.c", gotUser)
	require.Equal(t, "jira_secret", gotPass)
	require.Equal(t, "2026-07-01 00:00", cursor)

	_, _, err = NewJiraPlugin().Fetch(context.Background(), jiraCfg(t, ts), "")
	require.NoError(t, err)
	require.Equal(t, `project = ENG ORDER BY updated ASC`, gotJQL)
}

func TestJiraPlugin_Fetch_should_FollowNextPageToken_AcrossAllPages(t *testing.T) {
	var tokens []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("nextPageToken")
		tokens = append(tokens, tok)
		switch tok {
		case "":
			fmt.Fprintf(w, `{"issues":[%s],"nextPageToken":"p2","isLast":false}`, jiraIssueJSON("ENG-1", "2026-08-01T10:00:00.000+0000", "High", "null"))
		case "p2":
			fmt.Fprintf(w, `{"issues":[%s],"nextPageToken":"p3","isLast":false}`, jiraIssueJSON("ENG-2", "2026-08-02T10:00:00.000+0000", "", "null"))
		default:
			fmt.Fprintf(w, `{"issues":[%s],"isLast":true}`, jiraIssueJSON("ENG-3", "2026-08-03T10:30:00.000+0000", "Lowest", "null"))
		}
	}))
	defer ts.Close()
	items, cursor, err := NewJiraPlugin().Fetch(context.Background(), jiraCfg(t, ts), "")
	require.NoError(t, err)
	require.Equal(t, []string{"", "p2", "p3"}, tokens)
	require.Len(t, items, 3)
	require.Equal(t, "2026-08-03 10:30", cursor)
	require.Equal(t, []int{4, DefaultBacklogPriority, 1}, []int{items[0].Priority, items[1].Priority, items[2].Priority})
	require.Equal(t, ts.URL+"/browse/ENG-1", items[0].URL)
}

func TestJiraPlugin_Fetch_should_ConvertADFDescriptionToPlainText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adf := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Fix the login bug"}]}]}`
		fmt.Fprintf(w, `{"issues":[%s],"isLast":true}`, jiraIssueJSON("ENG-1", "2026-08-01T10:00:00.000+0000", "Medium", adf))
	}))
	defer ts.Close()
	items, _, err := NewJiraPlugin().Fetch(context.Background(), jiraCfg(t, ts), "")
	require.NoError(t, err)
	got := NewJiraPlugin().MapToBacklogItem(items[0], "src")
	require.Equal(t, "Fix the login bug", got.Description)
	require.Equal(t, "ENG-1", got.ExternalID)
	require.Equal(t, "src", got.SourceID)
}

func TestJiraPlugin_Fetch_should_ClassifyHTTPErrors(t *testing.T) {
	cases := map[int]string{401: "authentication failed (401)", 429: "rate limited", 500: "unexpected status 500"}
	for status, want := range cases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		_, cursor, err := NewJiraPlugin().Fetch(context.Background(), jiraCfg(t, ts), "c0")
		ts.Close()
		require.ErrorContains(t, err, want)
		require.NotContains(t, err.Error(), "jira_secret")
		require.Equal(t, "c0", cursor)
	}
}

func TestJiraPlugin_Fetch_should_NotReadCredentialsFromConfigJSON(t *testing.T) {
	clearJiraEnv(t)
	cfg := PluginConfig{Raw: `{"base_url":"https://plain.atlassian.net","project_key":"ENG","email":"a@b.c","token":"plaintext"}`}
	items, _, err := NewJiraPlugin().Fetch(context.Background(), cfg, "c0")
	require.NoError(t, err)
	require.Empty(t, items, "credentials in Raw must not enable the source")
}

func TestJiraPlugin_Fetch_should_PreferKeychainCredentials(t *testing.T) {
	clearJiraEnv(t)
	t.Setenv("JIRA_EMAIL", "env@x.com")
	t.Setenv("JIRA_API_TOKEN", "env_tok")
	var gotUser string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _, _ = r.BasicAuth()
		fmt.Fprint(w, `{"issues":[],"isLast":true}`)
	}))
	defer ts.Close()
	require.NoError(t, jira.SetKeychainTokenForHost(ts.URL, "kc@x.com", "kc_tok"))
	t.Cleanup(func() { _ = jira.DeleteKeychainTokenForHost(ts.URL) })
	cfg := PluginConfig{Raw: fmt.Sprintf(`{"base_url":%q,"project_key":"ENG"}`, ts.URL)}
	_, _, err := NewJiraPlugin().Fetch(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Equal(t, "kc@x.com", gotUser)
}

func TestJiraPlugin_MapToBacklogItem_should_TruncateLongText(t *testing.T) {
	t.Parallel()
	got := NewJiraPlugin().MapToBacklogItem(ExternalItem{Title: strings.Repeat("a", 300), Description: strings.Repeat("b", 3000)}, "s")
	require.Len(t, got.Title, 200)
	require.Len(t, got.Description, 2000)
}

func TestParseJiraIssueRef(t *testing.T) {
	t.Parallel()
	base, key, err := ParseJiraIssueRef("https://acme.atlassian.net/browse/ENG-123?focusedCommentId=1")
	require.NoError(t, err)
	require.Equal(t, "https://acme.atlassian.net", base)
	require.Equal(t, "ENG-123", key)
	base, key, err = ParseJiraIssueRef("eng-7")
	require.NoError(t, err)
	require.Equal(t, "", base)
	require.Equal(t, "ENG-7", key)
	for _, bad := range []string{"", "ftp://x/browse/ENG-1", "https://acme.atlassian.net/projects/ENG", "nope"} {
		_, _, err := ParseJiraIssueRef(bad)
		require.Error(t, err, bad)
	}
}

func TestJiraPlugin_FetchIssue_should_ReturnItem_And_ClassifyErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/ENG-404") {
			w.WriteHeader(404)
			return
		}
		adf := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`
		fmt.Fprint(w, jiraIssueJSON("ENG-5", "2026-08-01T10:00:00.000+0000", "High", adf))
	}))
	defer ts.Close()
	cfg := jiraCfg(t, ts)
	item, err := NewJiraPlugin().FetchIssue(context.Background(), cfg, "ENG-5")
	require.NoError(t, err)
	require.Equal(t, "hello", item.Description)
	require.Equal(t, ts.URL+"/browse/ENG-5", item.URL)
	_, err = NewJiraPlugin().FetchIssue(context.Background(), cfg, "ENG-404")
	require.ErrorContains(t, err, "not found")
	clearJiraEnv(t)
	_, err = NewJiraPlugin().FetchIssue(context.Background(), PluginConfig{Raw: `{"base_url":"https://nocreds.atlassian.net"}`}, "ENG-5")
	require.ErrorContains(t, err, "not configured")
	_, err = NewJiraPlugin().FetchIssue(context.Background(), PluginConfig{}, "ENG-5")
	require.ErrorContains(t, err, "base_url")
}
