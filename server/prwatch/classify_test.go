package prwatch

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

func ev(kind, action, details string) inbox.Event {
	e := inbox.Event{Version: 1, Kind: kind, Action: action, Repository: "o/r", PullRequest: 1}
	if details != "" {
		e.Details = json.RawMessage(details)
	}
	return e
}

func TestClassify_should_SteerOrDropPerKindAndAction_When_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		ev     inbox.Event
		kinds  []Kind
		reason DropReason
	}{
		{"comment created", ev("comment", "created", ""), []Kind{KindComment}, ""},
		{"comment edited", ev("comment", "edited", ""), nil, DropEditedOrDeleted},
		{"comment deleted", ev("comment", "deleted", ""), nil, DropEditedOrDeleted},
		{"review_comment created", ev("review_comment", "created", ""), []Kind{KindReviewComment}, ""},
		{"review_comment edited", ev("review_comment", "edited", ""), nil, DropEditedOrDeleted},
		{"review changes_requested", ev("review", "submitted", `{"state":"changes_requested"}`), []Kind{KindReview}, ""},
		{"review CHANGES_REQUESTED upper", ev("review", "submitted", `{"state":"CHANGES_REQUESTED"}`), []Kind{KindReview}, ""},
		{"review commented", ev("review", "submitted", `{"state":"commented"}`), []Kind{KindReview}, ""},
		{"review approved", ev("review", "submitted", `{"state":"approved"}`), nil, DropApprovedReview},
		{"review APPROVED upper", ev("review", "submitted", `{"state":"APPROVED"}`), nil, DropApprovedReview},
		{"review no state steers", ev("review", "submitted", ``), []Kind{KindReview}, ""},
		{"review edited", ev("review", "edited", `{"state":"commented"}`), nil, DropEditedOrDeleted},
		{"pr closed", ev("pull_request", "closed", ""), nil, DropPullRequestEvent},
		{"pr synchronize", ev("pull_request", "synchronize", ""), nil, DropPullRequestEvent},
		{"pr labeled", ev("pull_request", "labeled", ""), nil, DropPullRequestEvent},
		{"ci suite failure", ev("ci", "completed", `{"ci_kind":"check_suite","conclusion":"failure"}`), []Kind{KindCI}, ""},
		{"ci workflow timed_out", ev("ci", "completed", `{"ci_kind":"workflow_run","conclusion":"timed_out"}`), []Kind{KindCI}, ""},
		{"ci workflow startup_failure", ev("ci", "completed", `{"ci_kind":"workflow_run","conclusion":"startup_failure"}`), []Kind{KindCI}, ""},
		{"ci suite action_required", ev("ci", "completed", `{"ci_kind":"check_suite","conclusion":"action_required"}`), []Kind{KindCI}, ""},
		{"unknown kind", ev("star", "created", ""), nil, DropUnknownKind},
		{"empty kind", ev("", "", ""), nil, DropUnknownKind},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kinds, reason, ok := classify(tc.ev)
			assert.Equal(t, tc.kinds, kinds)
			assert.Equal(t, tc.reason, reason)
			assert.Equal(t, tc.kinds != nil, ok)
		})
	}
}

func TestClassify_should_DropAndAck_When_KindIsUnknown(t *testing.T) {
	e := ev("mystery", "created", "")
	_, reason, ok := classify(e)
	assert.False(t, ok)
	assert.Equal(t, DropUnknownKind, reason)

	e = ev("comment", "created", "")
	e.Version = 2
	_, reason, ok = classify(e)
	assert.False(t, ok)
	assert.Equal(t, DropUnknownKind, reason)

	e.Version = 0
	_, _, ok = classify(e)
	assert.False(t, ok)
}

func TestClassify_should_SteerOnlyFailureShapedCompletedSuiteOrWorkflow_When_KindIsCI(t *testing.T) {
	for _, ciKind := range []string{"check_suite", "workflow_run"} {
		for _, c := range []string{"failure", "timed_out", "startup_failure", "action_required"} {
			_, _, ok := classify(ev("ci", "completed", `{"ci_kind":"`+ciKind+`","conclusion":"`+c+`"}`))
			assert.True(t, ok, "%s/%s", ciKind, c)
		}
		for _, c := range []string{"cancelled", "success", "neutral", "skipped", "stale"} {
			_, reason, ok := classify(ev("ci", "completed", `{"ci_kind":"`+ciKind+`","conclusion":"`+c+`"}`))
			assert.False(t, ok, "%s/%s", ciKind, c)
			assert.Equal(t, DropCINotFailure, reason)
		}
		for _, d := range []string{`{"ci_kind":"` + ciKind + `","conclusion":null}`, `{"ci_kind":"` + ciKind + `"}`, `{"ci_kind":"` + ciKind + `","conclusion":5}`, ``, `null`, `not json`} {
			_, reason, ok := classify(ev("ci", "completed", d))
			assert.False(t, ok, d)
			assert.Equal(t, DropCINotFailure, reason, d)
		}
	}
	for _, c := range []string{"failure", "success", "timed_out", ""} {
		_, reason, ok := classify(ev("ci", "completed", `{"ci_kind":"check_run","conclusion":"`+c+`"}`))
		assert.False(t, ok)
		assert.Equal(t, DropCICheckRun, reason)
	}
}

func TestClassify_should_NotSteer_When_ApprovedReviewPullRequestEventEditedCancelledOrPerJobCheckRun(t *testing.T) {
	cases := []inbox.Event{
		ev("review", "submitted", `{"state":"approved"}`),
		ev("pull_request", "opened", ""),
		ev("pull_request", "closed", ""),
		ev("comment", "edited", ""),
		ev("comment", "deleted", ""),
		ev("ci", "completed", `{"ci_kind":"check_suite","conclusion":"cancelled"}`),
		ev("ci", "completed", `{"ci_kind":"check_run","conclusion":"failure"}`),
	}
	for _, c := range cases {
		kinds, reason, ok := classify(c)
		assert.False(t, ok, "%s/%s", c.Kind, c.Action)
		assert.Empty(t, kinds)
		assert.NotEmpty(t, reason)
	}
}

func TestClassify_should_YieldExactlyOneSteer_When_ThreeJobFailedRunFixture(t *testing.T) {
	fixture := []inbox.Event{
		ev("ci", "completed", `{"ci_kind":"check_run","conclusion":"failure"}`),
		ev("ci", "completed", `{"ci_kind":"check_run","conclusion":"failure"}`),
		ev("ci", "completed", `{"ci_kind":"check_run","conclusion":"failure"}`),
		ev("ci", "completed", `{"ci_kind":"check_suite","conclusion":"failure"}`),
		ev("ci", "completed", `{"ci_kind":"workflow_run","conclusion":"failure"}`),
	}
	steers := 0
	for _, e := range fixture {
		if _, _, ok := classify(e); ok {
			steers++
		}
	}
	// classify steers the suite and the workflow; the D12 head-SHA fold
	// (task 4.4) reduces those two to one. Per-job runs never steer.
	require.Equal(t, 2, steers)
}

func FuzzClassify(f *testing.F) {
	f.Add("ci", "completed", `{"ci_kind":"check_suite","conclusion":"failure"}`)
	f.Add("review", "submitted", `{"state":"APPROVED"}`)
	f.Fuzz(func(t *testing.T, kind, action, details string) {
		e := ev(kind, action, "")
		e.Details = json.RawMessage(details)
		kinds, reason, ok := classify(e)
		if ok {
			require.NotEmpty(t, kinds)
			require.Empty(t, reason)
		} else {
			require.Empty(t, kinds)
			require.NotEmpty(t, reason)
		}
	})
}

func TestIsPullRequestClosed_should_MatchOnlyClosedPullRequestEvents(t *testing.T) {
	assert.True(t, isPullRequestClosed(ev("pull_request", "closed", "")))
	assert.False(t, isPullRequestClosed(ev("pull_request", "synchronize", "")))
	assert.False(t, isPullRequestClosed(ev("comment", "closed", "")))
}
