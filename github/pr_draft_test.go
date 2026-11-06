package github

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPRDraftMutations_should_SendNodeIDAndBearerToken_When_Called(t *testing.T) {
	cases := []struct {
		name string
		call func() error
		op   string
	}{
		{"to draft", func() error { return ConvertPRToDraft(t.Context(), "github.com", "PR_1", "tok") }, "convertPullRequestToDraft"},
		{"to ready", func() error { return MarkPRReadyForReview(t.Context(), "github.com", "PR_1", "tok") }, "markPullRequestReadyForReview"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotQuery, gotAuth, gotID string
			startGitHubCom(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var in struct {
					Query     string            `json:"query"`
					Variables map[string]string `json:"variables"`
				}
				_ = json.NewDecoder(r.Body).Decode(&in)
				gotQuery, gotAuth, gotID = in.Query, r.Header.Get("Authorization"), in.Variables["id"]
				_, _ = w.Write([]byte(`{"data":{}}`))
			}))
			if err := tc.call(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(gotQuery, tc.op) || gotAuth != "Bearer tok" || gotID != "PR_1" {
				t.Fatalf("query=%q auth=%q id=%q", gotQuery, gotAuth, gotID)
			}
		})
	}
}

func TestPRDraftMutations_should_ReturnError_When_GraphQLReportsErrors(t *testing.T) {
	startGitHubCom(t, fixedResponse(`{"errors":[{"type":"FORBIDDEN","message":"nope"}]}`))
	err := ConvertPRToDraft(t.Context(), "github.com", "PR_1", "tok")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("want error containing 'nope', got %v", err)
	}
}
