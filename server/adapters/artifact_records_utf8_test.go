package adapters

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"

	"google.golang.org/protobuf/proto"
)

func TestArtifactsToProto_ShouldMarshal_WhenStringsHaveInvalidUTF8(t *testing.T) {
	t.Parallel()
	bad := "ok\xe6\x97 tail" // truncated 3-byte rune
	recs := domain.RecordList{
		domain.CommandRecord{Type: bad, Command: bad, Detail: bad, Result: bad},
		domain.CommitRecord{SHA: bad, Subject: bad, Branch: bad},
		domain.PRRecord{Host: bad, Owner: bad, Repo: bad, Branch: bad, Base: bad},
		domain.BranchRecord{Name: bad, Action: bad},
		domain.IssueRecord{Host: bad, Owner: bad, Repo: bad, Relation: bad},
		domain.PlanDocRecord{Path: bad},
		domain.CILinkRecord{URL: bad},
	}
	got := artifactsToProto(&domain.SessionArtifactsBlob{
		PRURLs: []string{bad}, CommitSHAs: []string{bad}, ExternalURLs: []string{bad}, Records: recs,
	}, true, nil)
	if _, err := proto.Marshal(got); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if r := got.Records[0].GetCommand().GetResult(); r != "ok� tail" {
		t.Fatalf("result = %q", r)
	}
}
