package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

// TestBacklogItemLink locks in the deep-link shape pushAndCreatePR and
// buildFallbackPRBody rely on to point a reviewer at the backlog item's
// detail view (web-app's `/backlog?item=` route).
func TestBacklogItemLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		itemID  string
		want    string
	}{
		{
			name:    "plain base URL and UUID",
			baseURL: "http://localhost:8543",
			itemID:  "b608ab1e-b86e-4130-8879-7328cd363063",
			want:    "http://localhost:8543/backlog?item=b608ab1e-b86e-4130-8879-7328cd363063",
		},
		{
			name:    "empty base URL",
			baseURL: "",
			itemID:  "b608ab1e-b86e-4130-8879-7328cd363063",
			want:    "/backlog?item=b608ab1e-b86e-4130-8879-7328cd363063",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, backlogItemLink(tt.baseURL, tt.itemID))
		})
	}
}

// TestBuildFallbackPRBody covers buildFallbackPRBody's pure composition of a
// PR body from the backlog item's own data: the item's description, a
// clickable deep link back to the backlog item, and an optional "## Test
// plan" checklist derived from acceptance criteria.
func TestBuildFallbackPRBody(t *testing.T) {
	t.Parallel()

	const dashboardBaseURL = "https://ssq.example.com"
	const itemID = "b608ab1e-b86e-4130-8879-7328cd363063"
	wantLink := "Backlog item: https://ssq.example.com/backlog?item=b608ab1e-b86e-4130-8879-7328cd363063"

	tests := []struct {
		name           string
		item           *BacklogItemData
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "no acceptance criteria — no test plan section",
			item: &BacklogItemData{
				ID:          itemID,
				Description: "Fixes the flaky retry loop in the push remediation path.",
			},
			wantContains: []string{
				"## Summary",
				"Fixes the flaky retry loop in the push remediation path.",
				wantLink,
			},
			wantNotContain: []string{"## Test plan"},
		},
		{
			name: "acceptance criteria present — test plan checklist with mixed statuses",
			item: &BacklogItemData{
				ID:          itemID,
				Description: "Adds a deep link to the fallback PR body.",
				AcceptanceCriteria: mustSerializeAcCriteria(t, []domain.AcCriterion{
					{Index: 0, Text: "Body contains the backlog item link", Status: domain.AcStatusDone},
					{Index: 1, Text: "Body contains the description", Status: domain.AcStatusPending},
				}),
			},
			wantContains: []string{
				"## Summary",
				"Adds a deep link to the fallback PR body.",
				wantLink,
				"## Test plan",
				"- [x] Body contains the backlog item link",
				"- [ ] Body contains the description",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildFallbackPRBody(tt.item, dashboardBaseURL)
			for _, want := range tt.wantContains {
				require.Contains(t, got, want)
			}
			for _, notWant := range tt.wantNotContain {
				require.NotContains(t, got, notWant)
			}
		})
	}
}

// TestBuildFallbackPRBody_SanitizesDescription confirms the description is
// routed through sanitizeField (HTML stripped, long text truncated) rather
// than dropped into the body verbatim.
func TestBuildFallbackPRBody_SanitizesDescription(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{
		ID:          "b608ab1e-b86e-4130-8879-7328cd363063",
		Description: "<script>alert(1)</script>" + strings.Repeat("a", 2000),
	}

	got := buildFallbackPRBody(item, "https://ssq.example.com")

	require.NotContains(t, got, "<script>")
	require.Contains(t, got, "[truncated]")
}

// TestBuildFallbackPRBody_OmitsBacklogFooter_WhenNoBaseURL: with no
// reviewer-reachable base URL the footer is dropped entirely, never replaced
// by a dead link.
func TestBuildFallbackPRBody_OmitsBacklogFooter_WhenNoBaseURL(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{ID: "b608ab1e-b86e-4130-8879-7328cd363063", Description: "Fixes the thing."}
	got := buildFallbackPRBody(item, "")

	require.Equal(t, "## Summary\nFixes the thing.\n", got)
	require.NotContains(t, got, "Backlog item")
	require.NotContains(t, got, "/backlog?item=")
}

func TestAppendBacklogFooter(t *testing.T) {
	t.Parallel()

	const itemID = "b608ab1e-b86e-4130-8879-7328cd363063"
	drafted := "## Summary\n\nBody text.\n\n"

	require.Equal(t,
		"## Summary\n\nBody text.\n\nBacklog item: https://ssq.example.com/backlog?item="+itemID+"\n",
		appendBacklogFooter(drafted, "https://ssq.example.com", itemID))
	require.Equal(t, "## Summary\n\nBody text.\n", appendBacklogFooter(drafted, "", itemID))
}

func TestReviewerReachableBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, in, want string }{
		{"public https", "https://ssq.example.com", "https://ssq.example.com"},
		{"trailing slash trimmed", "https://ssq.example.com/", "https://ssq.example.com"},
		{"lan hostname with port", "https://onyx.lan:8444", "https://onyx.lan:8444"},
		{"empty", "", ""},
		{"localhost", "http://localhost:8543", ""},
		{"loopback v4", "http://127.0.0.1:8543", ""},
		{"loopback v6", "http://[::1]:8543", ""},
		{"unspecified", "http://0.0.0.0:8543", ""},
		{"no scheme", "ssq.example.com", ""},
		{"custom scheme", "ssq://host/backlog", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, ReviewerReachableBaseURL(tt.in))
		})
	}
}

func TestListenerDashboardBaseURL_DropsLoopback(t *testing.T) {
	t.Parallel()

	l := &BacklogLifecycleListener{}
	require.Equal(t, "", l.getDashboardBaseURL(), "unset fn must yield no link")
	l.SetDashboardBaseURLFn(func() string { return "http://127.0.0.1:8543" })
	require.Equal(t, "", l.getDashboardBaseURL())
	l.SetDashboardBaseURLFn(func() string { return "https://ssq.example.com" })
	require.Equal(t, "https://ssq.example.com", l.getDashboardBaseURL())
}

func mustSerializeAcCriteria(t *testing.T, criteria []domain.AcCriterion) domain.AcCriteriaJSON {
	t.Helper()
	serialized, err := domain.SerializeAcCriteria(criteria)
	require.NoError(t, err)
	return serialized
}
