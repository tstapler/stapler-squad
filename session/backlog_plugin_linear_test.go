package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/linear"
)

// Relies on TestMain in integration_test.go calling keyring.MockInit().

// withLinearTestServer points linearGraphQLURL at a server for the test.
// Tests using it must not run with t.Parallel().
func withLinearTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	ts := httptest.NewServer(handler)
	orig := linearGraphQLURL
	linearGraphQLURL = ts.URL
	t.Cleanup(func() {
		linearGraphQLURL = orig
		ts.Close()
	})
}

func linearIssuesResponse(nodes string, hasNext bool, end string) string {
	return fmt.Sprintf(`{"data":{"issues":{"nodes":[%s],"pageInfo":{"hasNextPage":%v,"endCursor":%q}}}}`, nodes, hasNext, end)
}

func TestLinearPlugin_PluginID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "linear", NewLinearPlugin().PluginID())
}

func TestNewDefaultRegistry_should_IncludeLinearAndJira(t *testing.T) {
	t.Parallel()
	r := NewDefaultRegistry()
	p, ok := r.Get("linear")
	require.True(t, ok)
	require.IsType(t, &LinearPlugin{}, p)
	p, ok = r.Get("jira")
	require.True(t, ok)
	require.IsType(t, &JiraPlugin{}, p)
}

func TestLinearPlugin_Fetch_should_ReturnEmpty_When_TokenMissing(t *testing.T) {
	t.Parallel()
	items, cursor, err := NewLinearPlugin().Fetch(context.Background(), PluginConfig{Raw: `{"team_key":"ENG"}`}, "old")
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, "old", cursor)
}

func TestLinearPlugin_Fetch_should_Error_When_ConfigIsInvalidJSON(t *testing.T) {
	t.Parallel()
	_, _, err := NewLinearPlugin().Fetch(context.Background(), PluginConfig{Raw: `{nope`}, "")
	require.Error(t, err)
}

func TestLinearPlugin_Fetch_should_MapIssuesAndAdvanceCursor(t *testing.T) {
	var gotAuth string
	var gotVars map[string]any
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		b, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(b, &req))
		gotVars = req.Variables
		fmt.Fprint(w, linearIssuesResponse(`
{"identifier":"ENG-1","title":"One","description":"d1","url":"https://linear.app/o/issue/ENG-1","priority":1,"updatedAt":"2026-08-01T00:00:00.000Z","state":{"name":"Todo","type":"unstarted"},"labels":{"nodes":[{"name":"bug"}]}},
{"identifier":"ENG-2","title":"Two","description":"","url":"https://linear.app/o/issue/ENG-2","priority":0,"updatedAt":"2026-08-03T00:00:00.000Z","state":{"name":"Todo","type":"unstarted"},"labels":{"nodes":[]}}`, false, ""))
	})
	cfg := PluginConfig{Raw: `{"token":"lin_key","team_key":"ENG"}`}
	items, cursor, err := NewLinearPlugin().Fetch(context.Background(), cfg, "2026-07-01T00:00:00.000Z")
	require.NoError(t, err)
	require.Equal(t, "lin_key", gotAuth, "raw key, no Bearer prefix")
	require.Equal(t, "2026-08-03T00:00:00.000Z", cursor)
	require.Len(t, items, 2)
	require.Equal(t, "ENG-1", items[0].ExternalID)
	require.Equal(t, []string{"bug"}, items[0].Labels)
	require.Equal(t, 5, items[0].Priority)
	require.Equal(t, DefaultBacklogPriority, items[1].Priority)
	require.False(t, items[0].IssueUpdatedAt.IsZero())

	filter := gotVars["filter"].(map[string]any)
	require.Equal(t, map[string]any{"gt": "2026-07-01T00:00:00.000Z"}, filter["updatedAt"])
	require.Contains(t, filter, "team")
}

func TestLinearPlugin_Fetch_should_PreferKeychainToken_OverConfigToken(t *testing.T) {
	require.NoError(t, linear.SetKeychainToken("from_keychain"))
	t.Cleanup(func() { _ = linear.DeleteKeychainToken() })
	var gotAuth string
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, linearIssuesResponse("", false, ""))
	})
	_, _, err := NewLinearPlugin().Fetch(context.Background(), PluginConfig{Raw: `{"token":"from_config"}`}, "")
	require.NoError(t, err)
	require.Equal(t, "from_keychain", gotAuth)
}

func TestLinearPlugin_Fetch_should_FollowPagination(t *testing.T) {
	calls := 0
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, linearIssuesResponse(`{"identifier":"ENG-9","title":"n","priority":3,"updatedAt":"2026-08-09T00:00:00.000Z","labels":{"nodes":[]}}`, true, "c1"))
			return
		}
		fmt.Fprint(w, linearIssuesResponse(`{"identifier":"ENG-8","title":"o","priority":3,"updatedAt":"2026-08-08T00:00:00.000Z","labels":{"nodes":[]}}`, false, ""))
	})
	items, cursor, err := NewLinearPlugin().Fetch(context.Background(), PluginConfig{Raw: `{"token":"k"}`}, "")
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, items, 2)
	require.Equal(t, "2026-08-09T00:00:00.000Z", cursor)
}

func TestLinearPlugin_Fetch_should_NotAdvanceCursor_When_PageCapHit(t *testing.T) {
	orig := maxLinearFetchPages
	maxLinearFetchPages = 2
	t.Cleanup(func() { maxLinearFetchPages = orig })
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, linearIssuesResponse(`{"identifier":"ENG-1","title":"t","priority":3,"updatedAt":"2026-08-09T00:00:00.000Z","labels":{"nodes":[]}}`, true, "more"))
	})
	items, cursor, err := NewLinearPlugin().Fetch(context.Background(), PluginConfig{Raw: `{"token":"k"}`}, "c0")
	require.Error(t, err)
	require.Empty(t, items)
	require.Equal(t, "c0", cursor)
}

func TestLinearPlugin_Fetch_should_ClassifyErrors(t *testing.T) {
	cases := []struct {
		name, body, want string
		status           int
	}{
		{"auth", `{"errors":[{"message":"bad key","extensions":{"type":"authentication error"}}]}`, "(401)", 400},
		{"ratelimit", `{"errors":[{"message":"slow down","extensions":{"code":"RATELIMITED"}}]}`, "rate limited", 400},
		{"http 429", `{}`, "rate limited", 429},
		{"other", `{"errors":[{"message":"boom"}]}`, "graphql error", 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				fmt.Fprint(w, c.body)
			})
			_, cursor, err := NewLinearPlugin().Fetch(context.Background(), PluginConfig{Raw: `{"token":"secret_key"}`}, "c0")
			require.ErrorContains(t, err, c.want)
			require.NotContains(t, err.Error(), "secret_key")
			require.Equal(t, "c0", cursor)
		})
	}
}

func TestLinearPlugin_MapToBacklogItem_should_MapPriorityTable(t *testing.T) {
	t.Parallel()
	want := map[int]int{0: DefaultBacklogPriority, 1: 5, 2: 4, 3: 3, 4: 2}
	for native, backlog := range want {
		require.Equal(t, backlog, linearPriorityToBacklog(native), "native %d", native)
	}
	got := NewLinearPlugin().MapToBacklogItem(ExternalItem{ExternalID: "ENG-123", Title: "T", URL: "https://linear.app/o/issue/ENG-123", Priority: 5, Labels: []string{"x"}}, "src1")
	require.Equal(t, "ENG-123", got.ExternalID)
	require.Equal(t, "https://linear.app/o/issue/ENG-123", got.ExternalURL)
	require.Equal(t, "src1", got.SourceID)
	require.Equal(t, string(BacklogStatusIdea), got.Status)
}

func TestParseLinearIssueRef(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"https://linear.app/acme/issue/ENG-123":           "ENG-123",
		"https://linear.app/acme/issue/eng-123/fix-a-bug": "ENG-123",
		"ENG-9": "ENG-9",
	} {
		got, err := ParseLinearIssueRef(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got)
	}
	for _, bad := range []string{"", "https://example.com/issue/ENG-1", "https://linear.app/acme/projects/x", "notanissue"} {
		_, err := ParseLinearIssueRef(bad)
		require.Error(t, err, bad)
	}
}

func TestLinearPlugin_FetchIssue_should_ReturnItem_And_ErrorWhenNotFoundOrNoKey(t *testing.T) {
	withLinearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"ENG-404"`) {
			fmt.Fprint(w, `{"data":{"issue":null}}`)
			return
		}
		fmt.Fprint(w, `{"data":{"issue":{"identifier":"ENG-5","title":"Five","description":"d","url":"https://linear.app/o/issue/ENG-5","priority":2,"updatedAt":"2026-08-01T00:00:00.000Z","labels":{"nodes":[]}}}}`)
	})
	cfg := PluginConfig{Raw: `{"token":"k"}`}
	item, err := NewLinearPlugin().FetchIssue(context.Background(), cfg, "ENG-5")
	require.NoError(t, err)
	require.Equal(t, "ENG-5", item.ExternalID)
	require.Equal(t, 4, item.Priority)
	_, err = NewLinearPlugin().FetchIssue(context.Background(), cfg, "ENG-404")
	require.ErrorContains(t, err, "not found")
	_, err = NewLinearPlugin().FetchIssue(context.Background(), PluginConfig{}, "ENG-5")
	require.ErrorContains(t, err, "not configured")
}
