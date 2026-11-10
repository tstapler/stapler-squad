package services

import (
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// withSessionPRHolds fills Session.pr_holds on detail responses only (never lists,
// K11), from the same hold derivation as the backlog item path and behind the same
// reader:backlog scope, so with the scope off the field stays empty.
func withSessionPRHolds(p *sessionv1.Session, inst *session.Instance) *sessionv1.Session {
	return withSessionPRHoldsScoped(p, inst, config.SessionMultiPR(config.SessionMultiPRScopeBacklog))
}

func withSessionPRHoldsScoped(p *sessionv1.Session, inst *session.Instance, scopeOn bool) *sessionv1.Session {
	if !scopeOn || p == nil || inst == nil {
		return p
	}
	snap := inst.Snapshot()
	p.PrHolds = NonTerminalPRHolds(snap.GitHub.PRs, snap.UUID)
	return p
}
