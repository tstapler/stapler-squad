package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
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

	const dashboardBaseURL = "http://localhost:8543"
	const itemID = "b608ab1e-b86e-4130-8879-7328cd363063"
	wantLink := "Backlog item: http://localhost:8543/backlog?item=b608ab1e-b86e-4130-8879-7328cd363063"

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

	got := buildFallbackPRBody(item, "http://localhost:8543")

	require.NotContains(t, got, "<script>")
	require.Contains(t, got, "[truncated]")
}

func mustSerializeAcCriteria(t *testing.T, criteria []domain.AcCriterion) domain.AcCriteriaJSON {
	t.Helper()
	serialized, err := domain.SerializeAcCriteria(criteria)
	require.NoError(t, err)
	return serialized
}

// failingCIListener wires a listener over a still-CI-failing PR and returns it
// with its stamped-capable notifier and fix spawner.
func failingCIListener(t *testing.T, storage *Storage, prNumber int) (*BacklogLifecycleListener, *autoRemediatingNotifier, *BacklogItemData) {
	t.Helper()
	item := newPRPendingTestItem(t, storage, prNumber)
	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{CIFailing: true, FeedbackText: "## Failing CI checks\n- build FAILED\n"},
	})
	listener.SetPRFixSpawner(&fakePRFixSpawner{})
	n := &autoRemediatingNotifier{}
	listener.SetNotifier(n)
	return listener, n, item
}

// T-AR-01: the first sighting of a PR that needs a fix publishes the WARNING
// through the auto-remediating (stamped) path, exactly once.
func TestPRNeedsAttention_ShouldStampAutoRemediating_WhenWarningPublishedAtBackoffGate(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	listener, n, _ := failingCIListener(t, storage, 9301)

	listener.ReconcilePRPending(context.Background(), storage.repo)
	listener.ReconcilePRPending(context.Background(), storage.repo)

	assert.Equal(t, []string{"PR needs attention"}, n.stamped, "one stamped WARNING on first sighting, none on the next tick")
	assert.Empty(t, n.titles(), "the automation-is-acting WARNING must not also go out unstamped")
}

// T-AR-02: when the automation gives up (the fifth attempt parks the row), the
// escalation is a plain, UNSTAMPED notification, so it counts as a pending
// decision and the stamped WARNING never masks it.
func TestPRNeedsAttention_ShouldHaveUnstampedEscalation_WhenAutomationFails(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	listener, n, item := failingCIListener(t, storage, 9302)
	er := storage.repo

	for attempt := 1; attempt <= 5; attempt++ {
		listener.ReconcilePRPending(context.Background(), er)
		backdateNextRemediationAt(t, er, item.ID, domain.StuckReasonPRNeedsFix, time.Now().Add(-time.Second))
	}

	assert.Equal(t, []string{"PR needs attention"}, n.stamped)
	require.Equal(t, []string{"Auto-rework paused"}, n.titles(), "the dead-end escalation goes through plain Notify")
	assert.True(t, n.calls[0].Urgent, "the escalation is urgent")
	assert.EqualValues(t, 8, n.calls[0].NotificationType)
}
