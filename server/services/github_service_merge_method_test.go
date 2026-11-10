package services

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

func TestResolveMergeMethod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  *sessionv1.MergePRRequest
		want string
	}{
		{"new only", &sessionv1.MergePRRequest{MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_SQUASH}, "squash"},
		{"new rebase", &sessionv1.MergePRRequest{MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_REBASE}, "rebase"},
		{"old only", &sessionv1.MergePRRequest{Method: svctest.StrPtr("rebase")}, "rebase"},
		{"both set, new wins", &sessionv1.MergePRRequest{Method: svctest.StrPtr("rebase"), MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_SQUASH}, "squash"},
		{"enum merge", &sessionv1.MergePRRequest{MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_MERGE}, "merge"},
		{"enum unspecified", &sessionv1.MergePRRequest{MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_UNSPECIFIED}, "merge"},
		{"neither defaults to merge", &sessionv1.MergePRRequest{}, "merge"},
		{"empty legacy defaults to merge", &sessionv1.MergePRRequest{Method: svctest.StrPtr("")}, "merge"},
		{"explicit legacy squash honored", &sessionv1.MergePRRequest{Method: svctest.StrPtr("squash")}, "squash"},
		{"invalid legacy passes through", &sessionv1.MergePRRequest{Method: svctest.StrPtr("bogus")}, "bogus"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveMergeMethod(c.req); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}
