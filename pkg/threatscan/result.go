package threatscan

// ThreatMatch reports one pattern that matched the scanned content.
type ThreatMatch struct {
	PatternID string
	Scope     Scope
	// Excerpt is always empty: no code path populates it from a match, so the matched
	// value can never leak through a ThreatMatch. Keep it that way until a consumer
	// needs redacted surrounding context.
	Excerpt string
}
