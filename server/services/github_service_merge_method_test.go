package services

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestResolveMergeMethod(t *testing.T) {
	cases := []struct {
		name string
		req  *sessionv1.MergePRRequest
		want string
	}{
		{"new only", &sessionv1.MergePRRequest{MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_SQUASH}, "squash"},
		{"new rebase", &sessionv1.MergePRRequest{MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_REBASE}, "rebase"},
		{"old only", &sessionv1.MergePRRequest{Method: strPtr("rebase")}, "rebase"},
		{"both set, new wins", &sessionv1.MergePRRequest{Method: strPtr("rebase"), MergeMethod: sessionv1.MergeMethod_MERGE_METHOD_SQUASH}, "squash"},
		{"neither", &sessionv1.MergePRRequest{}, "merge"},
		{"empty legacy", &sessionv1.MergePRRequest{Method: strPtr("")}, "merge"},
		{"invalid legacy passes through", &sessionv1.MergePRRequest{Method: strPtr("bogus")}, "bogus"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveMergeMethod(c.req); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}
