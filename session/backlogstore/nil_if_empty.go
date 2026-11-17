package backlogstore

import "github.com/tstapler/stapler-squad/session/domain"

// nilIfEmpty returns nil for an empty string so Ent's SetNillable* builders leave the column NULL.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nilIfEmptyJSON returns nil for an empty criteria JSON, else a pointer to its string form.
func nilIfEmptyJSON(j domain.AcCriteriaJSON) *string {
	if j == "" {
		return nil
	}
	s := string(j)
	return &s
}
