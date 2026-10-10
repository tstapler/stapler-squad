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

	origin := PRFooterOrigin{BaseURL: "https://ssq.example.com", HostID: testHostID, Hostname: "onyx.lan"}
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
			got := buildFallbackPRBody(tt.item, origin)
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

	got := buildFallbackPRBody(item, PRFooterOrigin{BaseURL: "https://ssq.example.com"})

	require.NotContains(t, got, "<script>")
	require.Contains(t, got, "[truncated]")
}

var testHostID = func() HostID {
	id, err := ParseHostID("host_01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err != nil {
		panic(err)
	}
	return id
}()

// TestPRFooter_AlwaysIdentifiesItemAndHost covers the footer's purpose: from
// any machine the owner can tell which instance and item a PR came from, with
// a clickable https link only when a non-loopback address is known.
func TestPRFooter_AlwaysIdentifiesItemAndHost(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{ID: "b608ab1e-b86e-4130-8879-7328cd363063"}
	tests := []struct {
		name         string
		origin       PRFooterOrigin
		wantContains []string
		wantLink     bool
	}{
		{
			name:   "link, hostname and host ID",
			origin: PRFooterOrigin{BaseURL: "https://onyx.lan:8444", HostID: testHostID, Hostname: "onyx.lan"},
			wantContains: []string{
				"Backlog item: https://onyx.lan:8444/backlog?item=" + item.ID,
				"ssq://onyx.lan/backlog/v1/" + item.ID,
				testHostID.String(),
			},
			wantLink: true,
		},
		{
			name:   "no base URL falls back to text-only with host ID in the ssq link",
			origin: PRFooterOrigin{HostID: testHostID},
			wantContains: []string{
				"Backlog item: " + item.ID,
				"ssq://" + testHostID.String() + "/backlog/v1/" + item.ID,
				testHostID.String(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := prFooter(tt.origin, item)
			for _, want := range tt.wantContains {
				require.Contains(t, got, want)
			}
			require.Equal(t, tt.wantLink, strings.Contains(got, "https://"))
			require.NotContains(t, got, "127.0.0.1")
			require.NotContains(t, got, "localhost")

			prov, ok := ParsePRProvenanceComment(got)
			require.True(t, ok, "footer must be readable by ParsePRProvenanceComment")
			require.Equal(t, testHostID, prov.HostID)
			require.Equal(t, item.ID, prov.ItemID)
		})
	}
}

func TestPRFooter_WithoutHostStillNamesItem(t *testing.T) {
	t.Parallel()

	got := prFooter(PRFooterOrigin{}, &BacklogItemData{ID: "abc"})
	require.Equal(t, "Backlog item: abc\n", got)
}

func TestBuildFallbackPRBody_TextOnlyFooter_WhenNoBaseURL(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{ID: "b608ab1e-b86e-4130-8879-7328cd363063", Description: "Fixes the thing."}
	got := buildFallbackPRBody(item, PRFooterOrigin{HostID: testHostID})

	require.Contains(t, got, "Backlog item: "+item.ID)
	require.Contains(t, got, testHostID.String())
	require.NotContains(t, got, "/backlog?item=")
	require.NotContains(t, got, "http")
}

func TestAppendBacklogFooter(t *testing.T) {
	t.Parallel()

	item := &BacklogItemData{ID: "b608ab1e-b86e-4130-8879-7328cd363063"}
	origin := PRFooterOrigin{BaseURL: "https://ssq.example.com", HostID: testHostID, Hostname: "onyx.lan"}
	got := appendBacklogFooter("## Summary\n\nBody text.\n\n", origin, item)

	require.True(t, strings.HasPrefix(got, "## Summary\n\nBody text.\n\nBacklog item: https://ssq.example.com/backlog?item="+item.ID))
	require.Contains(t, got, testHostID.String())
}

func TestNonLoopbackBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, in, want string }{
		{"public https", "https://ssq.example.com", "https://ssq.example.com"},
		{"trailing slash trimmed", "https://ssq.example.com/", "https://ssq.example.com"},
		{"lan hostname with port", "https://onyx.lan:8444", "https://onyx.lan:8444"},
		{"lan IP", "http://192.168.1.5:8543", "http://192.168.1.5:8543"},
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
			require.Equal(t, tt.want, NonLoopbackBaseURL(tt.in))
		})
	}
}

func TestListenerPRFooterOrigin(t *testing.T) {
	t.Parallel()

	l := &BacklogLifecycleListener{}
	require.Equal(t, PRFooterOrigin{}, l.getPRFooterOrigin(), "unset fns must yield an empty origin")

	l.SetDashboardBaseURLFn(func() string { return "http://127.0.0.1:8543" })
	require.Equal(t, "", l.getPRFooterOrigin().BaseURL, "loopback must be dropped")

	l.SetDashboardBaseURLFn(func() string { return "https://ssq.example.com" })
	l.SetHostRefFn(func() (HostID, string) { return testHostID, "onyx.lan" })
	require.Equal(t, PRFooterOrigin{BaseURL: "https://ssq.example.com", HostID: testHostID, Hostname: "onyx.lan"}, l.getPRFooterOrigin())
}

func mustSerializeAcCriteria(t *testing.T, criteria []domain.AcCriterion) domain.AcCriteriaJSON {
	t.Helper()
	serialized, err := domain.SerializeAcCriteria(criteria)
	require.NoError(t, err)
	return serialized
}
