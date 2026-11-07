package adapters

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/session/artifacts"
)

func TestArtifactsToProto_ShouldMarshal_WhenStringsHaveInvalidUTF8(t *testing.T) {
	t.Parallel()
	bad := "ok\xe6\x97 tail" // truncated 3-byte rune
	recs := artifacts.RecordList{
		artifacts.CommandRecord{Type: bad, Command: bad, Detail: bad, Result: bad},
		artifacts.CommitRecord{SHA: bad, Subject: bad, Branch: bad},
		artifacts.PRRecord{Host: bad, Owner: bad, Repo: bad, Branch: bad, Base: bad},
		artifacts.BranchRecord{Name: bad, Action: bad},
		artifacts.IssueRecord{Host: bad, Owner: bad, Repo: bad, Relation: bad},
		artifacts.PlanDocRecord{Path: bad},
		artifacts.CILinkRecord{URL: bad},
	}
	got := artifactsToProto(&artifacts.SessionArtifactsBlob{
		PRURLs: []string{bad}, CommitSHAs: []string{bad}, ExternalURLs: []string{bad}, Records: recs,
	}, true, nil)
	if _, err := proto.Marshal(got); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if r := got.Records[0].GetCommand().GetResult(); r != "ok� tail" {
		t.Fatalf("result = %q", r)
	}
}
