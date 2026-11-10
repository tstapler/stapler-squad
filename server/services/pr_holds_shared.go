package services

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// NonTerminalPRHolds is one hold per PR that keeps its owner's worktree. ownerOverride,
// when set, names the owning session for every hold; otherwise each view's own
// SessionUUID is used.
func NonTerminalPRHolds(prs []session.PRView, ownerOverride string) []*sessionv1.PrHold {
	var holds []*sessionv1.PrHold
	for _, v := range prs {
		kind, held := holdKindOf(v)
		if !held {
			continue
		}
		owner := v.SessionUUID
		if ownerOverride != "" {
			owner = ownerOverride
		}
		holds = append(holds, &sessionv1.PrHold{Kind: kind, Pr: PRRefToProto(v.Ref), OwnerSessionId: owner})
	}
	return holds
}

func holdKindOf(v session.PRView) (sessionv1.PrHold_Kind, bool) {
	switch {
	case v.Terminal():
		return 0, false
	case v.State == session.PRStateUnknown && v.StoredTerminal:
		return sessionv1.PrHold_KIND_UNVERIFIED, true
	case v.State == session.PRStateUnknown:
		return sessionv1.PrHold_KIND_UNKNOWN, true
	default:
		return sessionv1.PrHold_KIND_OPEN_PR, true
	}
}

func PRRefToProto(r session.PRRef) *sessionv1.PullRequestRef {
	return &sessionv1.PullRequestRef{Host: r.Host(), Owner: r.Owner(), Repo: r.Repo(), Number: int32(r.Number())} //nolint:gosec // PR numbers are far below int32 max
}
