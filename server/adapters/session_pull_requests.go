package adapters

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
)

// SessionPullRequests maps a session's PR set to its wire form. The entry the
// session's ActivePR picks (given its current branch) carries is_active. The
// outcome_unverified flag is derived (unknown but terminal), never stored.
func SessionPullRequests(prs []domain.PRView, currentBranch string) []*sessionv1.SessionPullRequest {
	active := domain.ActivePR(prs, currentBranch)
	out := make([]*sessionv1.SessionPullRequest, 0, len(prs))
	for _, v := range prs {
		out = append(out, sessionPullRequest(v, active != nil && active.Ref == v.Ref))
	}
	return out
}

// SessionPullRequestsForList is the list-response form (K11): one slim entry per PR
// with what a row or chip renders (ref, state, role, flags, check summary). Urls,
// branches, timestamps, attach source and the rate-limit reset time are left out; the detail view
// reads them from the PR RPCs (RefreshSessionPullRequests and friends), which return
// the full SessionPullRequests.
func SessionPullRequestsForList(prs []domain.PRView, currentBranch string) []*sessionv1.SessionPullRequest {
	active := domain.ActivePR(prs, currentBranch)
	out := make([]*sessionv1.SessionPullRequest, 0, len(prs))
	for _, v := range prs {
		out = append(out, &sessionv1.SessionPullRequest{
			Host: v.Ref.Host(), Owner: v.Ref.Owner(), Repo: v.Ref.Repo(), Number: clampInt32(v.Ref.Number()),
			State: string(v.State), IsDraft: v.IsDraft, Role: string(v.Role),
			IsActive: active != nil && active.Ref == v.Ref, Pinned: v.Pinned,
			CheckConclusion:   v.CheckConclusion,
			PollError:         pollErrorToProto(v.PollError), // one varint, and zero when healthy
			OutcomeUnverified: v.State == domain.PRStateUnknown && v.StoredTerminal,
		})
	}
	return out
}

func sessionPullRequest(v domain.PRView, isActive bool) *sessionv1.SessionPullRequest {
	out := &sessionv1.SessionPullRequest{
		Host: v.Ref.Host(), Owner: v.Ref.Owner(), Repo: v.Ref.Repo(),
		Number: clampInt32(v.Ref.Number()), Url: v.URL, State: string(v.State), IsDraft: v.IsDraft,
		Role: string(v.Role), HeadBranch: v.HeadBranch, BaseBranch: v.BaseBranch,
		CreatedAt: optTimestamp(v.OpenedAt), MergedAt: optTimestamp(v.MergedAt), ClosedAt: optTimestamp(v.ClosedAt),
		IsActive: isActive, Pinned: v.Pinned, AttachSource: string(v.AttachSource),
		CheckConclusion: v.CheckConclusion, ApprovedCount: clampInt32(v.ApprovedCount),
		ChangesRequestedCount: clampInt32(v.ChangesRequestedCount),
		PollError:             pollErrorToProto(v.PollError),
		LastPolledAt:          optTimestamp(v.LastPolledAt), RateLimitResetAt: optTimestamp(v.RateLimitResetAt),
		OutcomeUnverified: v.State == domain.PRStateUnknown && v.StoredTerminal,
		Title:             v.Title,
		ItemDirect:        v.BacklogItemID != "",
	}
	if v.UnverifiedTerminal() {
		out.VerifyAttempts = clampInt32(v.PollFailCount)
		out.VerifyAttemptLimit = clampInt32(domain.VerifyAttemptLimit)
		out.VerifyExhausted = v.VerifyExhausted()
	}
	return out
}

func clampInt32(n int) int32 {
	const maxInt32 = 1<<31 - 1
	if n > maxInt32 {
		return maxInt32
	}
	if n < 0 {
		return 0
	}
	return int32(n) // #nosec G115 -- bounded above
}

func optTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func pollErrorToProto(s string) sessionv1.PollError {
	switch s {
	case "auth":
		return sessionv1.PollError_POLL_ERROR_AUTH
	case "rate_limited":
		return sessionv1.PollError_POLL_ERROR_RATE_LIMITED
	case "not_found":
		return sessionv1.PollError_POLL_ERROR_NOT_FOUND
	case "repo_inaccessible":
		return sessionv1.PollError_POLL_ERROR_REPO_INACCESSIBLE
	case "network":
		return sessionv1.PollError_POLL_ERROR_NETWORK
	}
	return sessionv1.PollError_POLL_ERROR_NONE
}
