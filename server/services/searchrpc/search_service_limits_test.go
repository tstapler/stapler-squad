package searchrpc

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestSearchClaudeHistory_should_ApplyDefaultAndMaximumLimit_When_ManySessionsMatch(t *testing.T) {
	seedManySessions(t, 105)
	svc := setupSyncedSearchService(t)

	for name, tc := range map[string]struct {
		limit int32
		want  int
	}{
		"unset uses the default of 20":       {limit: 0, want: 20},
		"negative uses the default of 20":    {limit: -5, want: 20},
		"explicit limit is honoured":         {limit: 7, want: 7},
		"the cap is 100 exactly":             {limit: 100, want: 100},
		"above the cap is clamped to 100":    {limit: 5000, want: 100},
		"one below the cap is not clamped":   {limit: 99, want: 99},
		"one above the default is not stuck": {limit: 21, want: 21},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
				Query: "streaming widget",
				Limit: tc.limit,
			}))
			require.NoError(t, err)
			require.Len(t, resp.Msg.Results, tc.want)
		})
	}
}

func TestSearchClaudeHistory_should_OversampleRawFetchFivefold_When_ProjectFilterDropsTopHits(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Equal-length messages whose "zebra" count differs give strictly ordered scores (no ties).
	msg := func(k int) string {
		return strings.Repeat("zebra ", k) + strings.Repeat("pad ", 12-k)
	}
	sessions := map[string]testConvSession{}
	for k := 3; k <= 10; k++ { // 8 higher-scoring sessions in another project
		sessions[fmt.Sprintf("other-%02d", k)] = testConvSession{project: "/repo/other", messages: []testConvMsg{{role: "user", content: msg(k), time: base}}}
	}
	sessions["keep-1"] = testConvSession{project: "/repo/keep", messages: []testConvMsg{{role: "user", content: msg(1), time: base}}}
	sessions["keep-2"] = testConvSession{project: "/repo/keep", messages: []testConvMsg{{role: "user", content: msg(2), time: base}}}
	seedClaudeHome(t, sessions)
	svc := setupSyncedSearchService(t)

	// Limit 2 fetches 2*5=10 raw hits; the two kept sessions rank 9th and 10th.
	resp, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query:   "zebra",
		Limit:   2,
		Project: ptr("/repo/keep"),
	}))
	require.NoError(t, err)
	ids := make([]string, 0, len(resp.Msg.Results))
	for _, r := range resp.Msg.Results {
		ids = append(ids, r.SessionId)
	}
	require.ElementsMatch(t, []string{"keep-2", "keep-1"}, ids)
}
