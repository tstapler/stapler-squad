package searchrpc

import (
	"testing"

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
