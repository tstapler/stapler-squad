package searchrpc

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/vc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VCSStatusToProto converts a VCS status for GetVCSStatus and history detail; it lives here so the search leaf never imports server/services.
func VCSStatusToProto(status *vc.VCSStatus) *sessionv1.VCSStatus {
	if status == nil {
		return nil
	}

	protoStatus := &sessionv1.VCSStatus{
		Type:        vcsTypeToProto(status.Type),
		Branch:      status.Branch,
		HeadCommit:  status.HeadCommit,
		Description: status.Description,
		// #nosec G115 -- AheadBy/BehindBy are git/jj ahead-behind commit
		// counts for one local branch, far below int32 range.
		AheadBy: int32(status.AheadBy),
		// #nosec G115 -- see AheadBy above.
		BehindBy:     int32(status.BehindBy),
		Upstream:     status.Upstream,
		HasStaged:    status.HasStaged,
		HasUnstaged:  status.HasUnstaged,
		HasUntracked: status.HasUntracked,
		HasConflicts: status.HasConflicts,
		IsClean:      status.IsClean,
	}

	for _, f := range status.StagedFiles {
		protoStatus.StagedFiles = append(protoStatus.StagedFiles, fileChangeToProto(f))
	}
	for _, f := range status.UnstagedFiles {
		protoStatus.UnstagedFiles = append(protoStatus.UnstagedFiles, fileChangeToProto(f))
	}
	for _, f := range status.UntrackedFiles {
		protoStatus.UntrackedFiles = append(protoStatus.UntrackedFiles, fileChangeToProto(f))
	}
	for _, f := range status.ConflictFiles {
		protoStatus.ConflictFiles = append(protoStatus.ConflictFiles, fileChangeToProto(f))
	}

	for _, c := range status.Commits {
		commit := &sessionv1.ShippedCommit{Sha: c.SHA, Summary: c.Summary, AuthorName: c.AuthorName}
		if !c.AuthorAt.IsZero() {
			commit.AuthoredAt = timestamppb.New(c.AuthorAt)
		}
		protoStatus.Commits = append(protoStatus.Commits, commit)
	}
	protoStatus.CommitsTruncated = status.CommitsTruncated
	protoStatus.CommitsUnavailable = status.CommitsUnavailable
	if status.AggregateDiffStat != nil {
		protoStatus.AggregateDiffStat = &sessionv1.AggregateDiffStat{
			// #nosec G115 -- diff stats for one local commit range, far below int32 range.
			FilesChanged: int32(status.AggregateDiffStat.FilesChanged),
			// #nosec G115 -- see FilesChanged above.
			Additions: int32(status.AggregateDiffStat.Additions),
			// #nosec G115 -- see FilesChanged above.
			Deletions: int32(status.AggregateDiffStat.Deletions),
		}
	}
	if !status.StatusAsOf.IsZero() {
		protoStatus.StatusAsOf = timestamppb.New(status.StatusAsOf)
	}

	return protoStatus
}

func vcsTypeToProto(t vc.VCSType) sessionv1.VCSType {
	switch t {
	case vc.VCSGit:
		return sessionv1.VCSType_VCS_TYPE_GIT
	case vc.VCSJujutsu:
		return sessionv1.VCSType_VCS_TYPE_JUJUTSU
	default:
		return sessionv1.VCSType_VCS_TYPE_UNSPECIFIED
	}
}

func fileStatusToProto(s vc.FileStatus) sessionv1.FileStatus {
	switch s {
	case vc.FileModified:
		return sessionv1.FileStatus_FILE_STATUS_MODIFIED
	case vc.FileAdded:
		return sessionv1.FileStatus_FILE_STATUS_ADDED
	case vc.FileDeleted:
		return sessionv1.FileStatus_FILE_STATUS_DELETED
	case vc.FileRenamed:
		return sessionv1.FileStatus_FILE_STATUS_RENAMED
	case vc.FileCopied:
		return sessionv1.FileStatus_FILE_STATUS_COPIED
	case vc.FileUntracked:
		return sessionv1.FileStatus_FILE_STATUS_UNTRACKED
	case vc.FileIgnored:
		return sessionv1.FileStatus_FILE_STATUS_IGNORED
	case vc.FileConflict:
		return sessionv1.FileStatus_FILE_STATUS_CONFLICT
	default:
		return sessionv1.FileStatus_FILE_STATUS_UNSPECIFIED
	}
}

func fileChangeToProto(f vc.FileChange) *sessionv1.FileChange {
	return &sessionv1.FileChange{
		Path:     f.Path,
		Status:   fileStatusToProto(f.Status),
		IsStaged: f.IsStaged,
		OldPath:  f.OldPath,
		// #nosec G115 -- per-file diff line counts, far below int32 range.
		Additions: int32(f.Additions),
		// #nosec G115 -- see Additions above.
		Deletions: int32(f.Deletions),
	}
}
