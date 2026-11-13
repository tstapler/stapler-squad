package services

import (
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// activePRStatus is the CI-relevant state of a session's active PR.
type activePRStatus struct {
	Number     int
	Conclusion string
	CheckedAt  time.Time
}

// activePRStatusOf reads the active PR's own check conclusion. The legacy
// GitHubCheckConclusion scalar is not a per-PR mirror (only number, URL and the
// terminal flag are projected), so with several PRs it can describe a merged one;
// the scalars are used only when the session has no links.
func activePRStatusOf(snap *domain.InstanceSnapshot) activePRStatus {
	g := snap.GitHub
	if active := domain.ActivePR(g.PRs, snap.Branch); active != nil {
		return activePRStatus{Number: active.Ref.Number(), Conclusion: active.CheckConclusion, CheckedAt: lastChecked(active, g.LastPRStatusCheck)}
	}
	return activePRStatus{Number: g.GitHubPRNumber, Conclusion: g.GitHubCheckConclusion, CheckedAt: g.LastPRStatusCheck}
}

func lastChecked(v *domain.PRView, scalar time.Time) time.Time {
	if v.LastCheckedAt.IsZero() {
		return scalar
	}
	return v.LastCheckedAt
}

// activePRCIStatus returns the active PR's number and check conclusion.
func activePRCIStatus(snap *domain.InstanceSnapshot) (number int, conclusion string) {
	s := activePRStatusOf(snap)
	return s.Number, s.Conclusion
}
