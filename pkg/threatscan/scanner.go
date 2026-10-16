package threatscan

import "slices"

// Scan returns one ThreatMatch per pattern that matches content under scope, in the
// declared order of DefaultPatterns. ScopeAll runs every pattern.
func Scan(content string, scope Scope) []ThreatMatch {
	var matches []ThreatMatch
	for _, p := range DefaultPatterns() {
		if scope != ScopeAll && !slices.Contains(p.scopes, scope) {
			continue
		}
		if p.re.MatchString(content) {
			matches = append(matches, ThreatMatch{PatternID: p.id, Scope: scope})
		}
	}
	return matches
}
