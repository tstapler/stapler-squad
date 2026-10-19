package github

import (
	"reflect"
	"testing"
)

func TestFetchUserPRsForToken_should_MapExtendedFields_When_NodeCarriesThem(t *testing.T) {
	extra := `,"id":"PR_kwDOabc","body":"hello","additions":12,"deletions":3,"changedFiles":4,` +
		`"labels":{"nodes":[{"name":"bug"},{"name":"ui"}]},` +
		`"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"bob"}},{"requestedReviewer":{}}]}`
	startGitHubCom(t, fixedResponse(prResponse(prNode(1, extra))))

	pr := fetchOne(t, newTestCache(), "github.com")[0]

	want := UserPR{NodeID: "PR_kwDOabc", Body: "hello", Additions: 12, Deletions: 3, ChangedFiles: 4,
		Labels: []string{"bug", "ui"}, RequestedReviewerLogins: []string{"bob"}}
	got := UserPR{NodeID: pr.NodeID, Body: pr.Body, Additions: pr.Additions, Deletions: pr.Deletions,
		ChangedFiles: pr.ChangedFiles, Labels: pr.Labels, RequestedReviewerLogins: pr.RequestedReviewerLogins}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
