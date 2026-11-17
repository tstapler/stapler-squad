package adapters

import (
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"

	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// artifactRecordsToProto maps the blob's typed records to the wire, keeping order.
func artifactRecordsToProto(recs domain.RecordList) []*sessionv1.ArtifactRecordProto {
	if len(recs) == 0 {
		return nil
	}
	out := make([]*sessionv1.ArtifactRecordProto, 0, len(recs))
	for _, r := range recs {
		if p := artifactRecordToProto(r); p != nil {
			out = append(out, p)
		}
	}
	return out
}

func artifactRecordToProto(r domain.ArtifactRecord) *sessionv1.ArtifactRecordProto {
	p := &sessionv1.ArtifactRecordProto{Kind: u(string(r.Kind()))}
	switch v := r.(type) {
	case domain.PRRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Pr{Pr: &sessionv1.ArtifactPRRecord{
			Host: u(v.Host), Owner: u(v.Owner), Repo: u(v.Repo), Number: int32(v.Number), //#nosec G115 -- PR numbers are far below int32 max
			Source: u(string(v.Source)), Branch: u(v.Branch), Base: u(v.Base),
		}}
	case domain.BranchRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Branch{Branch: &sessionv1.ArtifactBranchRecord{Name: u(v.Name), Action: u(v.Action)}}
	case domain.CommitRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Commit{Commit: &sessionv1.ArtifactCommitRecord{Sha: u(v.SHA), Subject: u(v.Subject), Branch: u(v.Branch)}}
	case domain.IssueRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Issue{Issue: &sessionv1.ArtifactIssueRecord{
			Host: u(v.Host), Owner: u(v.Owner), Repo: u(v.Repo), Number: int32(v.Number), Relation: u(v.Relation), //#nosec G115 -- issue numbers are far below int32 max
		}}
	case domain.PlanDocRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_PlanDoc{PlanDoc: &sessionv1.ArtifactPlanDocRecord{Path: u(v.Path)}}
	case domain.CILinkRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_CiLink{CiLink: &sessionv1.ArtifactCILinkRecord{Url: u(v.URL)}}
	case domain.CommandRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Command{Command: &sessionv1.ArtifactCommandRecord{
			Type: u(v.Type), Command: u(v.Command), Detail: u(v.Detail), Result: u(v.Result),
		}}
	default:
		return nil
	}
	return p
}

// seenAt leaves the timestamp unset for a record whose transcript line had none.
func seenAt(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// ArtifactRecordToProto maps one typed record to the wire (nil for an unknown variant).
func ArtifactRecordToProto(r domain.ArtifactRecord) *sessionv1.ArtifactRecordProto {
	return artifactRecordToProto(r)
}

// u replaces invalid UTF-8 with U+FFFD: proto3 string fields reject it, and one bad
// transcript-derived value would otherwise fail the whole ListSessions response.
func u(s string) string { return strings.ToValidUTF8(s, "\uFFFD") }

func validUTF8All(ss []string) []string {
	if ss == nil {
		return nil
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = u(s)
	}
	return out
}
