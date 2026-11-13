package session

import "github.com/tstapler/stapler-squad/session/domain"

// LegacyOnlyPRSafe is the one rule for a session whose only PR evidence is the
// legacy scalar number: the persisted terminal flag, nothing else. The retention
// sweeper and the item cleanup gate share it so they cannot disagree.
func LegacyOnlyPRSafe(d domain.InstanceData) bool { return d.GitHubPRStatusTerminal }

// LegacyNumberRepresented reports whether one of rows carries the repo and
// number of d's legacy PR. An unbuildable legacy ref is never represented.
func LegacyNumberRepresented(d domain.InstanceData, rows []domain.PRView) bool {
	ref, err := sessionLegacyRef(d.GitHubOwner, d.GitHubRepo, d.GitHubHost, d.Path, d.GitHubPRNumber)
	if err != nil {
		return false
	}
	for _, v := range rows {
		if v.Ref == ref {
			return true
		}
	}
	return false
}
