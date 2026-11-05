package adapters

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/artifacts"
)

// artifactRecordsToProto maps the blob's typed records to the wire, keeping order.
func artifactRecordsToProto(recs artifacts.RecordList) []*sessionv1.ArtifactRecordProto {
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

func artifactRecordToProto(r artifacts.ArtifactRecord) *sessionv1.ArtifactRecordProto {
	p := &sessionv1.ArtifactRecordProto{Kind: string(r.Kind())}
	switch v := r.(type) {
	case artifacts.PRRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Pr{Pr: &sessionv1.ArtifactPRRecord{
			Host: v.Host, Owner: v.Owner, Repo: v.Repo, Number: int32(v.Number), //#nosec G115 -- PR numbers are far below int32 max
			Source: string(v.Source), Branch: v.Branch, Base: v.Base,
		}}
	case artifacts.BranchRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Branch{Branch: &sessionv1.ArtifactBranchRecord{Name: v.Name, Action: v.Action}}
	case artifacts.CommitRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Commit{Commit: &sessionv1.ArtifactCommitRecord{Sha: v.SHA, Subject: v.Subject, Branch: v.Branch}}
	case artifacts.IssueRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Issue{Issue: &sessionv1.ArtifactIssueRecord{
			Host: v.Host, Owner: v.Owner, Repo: v.Repo, Number: int32(v.Number), Relation: v.Relation, //#nosec G115 -- issue numbers are far below int32 max
		}}
	case artifacts.PlanDocRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_PlanDoc{PlanDoc: &sessionv1.ArtifactPlanDocRecord{Path: v.Path}}
	case artifacts.CILinkRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_CiLink{CiLink: &sessionv1.ArtifactCILinkRecord{Url: v.URL}}
	case artifacts.CommandRecord:
		p.FirstSeenAt = seenAt(v.FirstSeenAt)
		p.Record = &sessionv1.ArtifactRecordProto_Command{Command: &sessionv1.ArtifactCommandRecord{
			Type: v.Type, Command: v.Command, Detail: v.Detail, Result: v.Result,
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
func ArtifactRecordToProto(r artifacts.ArtifactRecord) *sessionv1.ArtifactRecordProto {
	return artifactRecordToProto(r)
}
