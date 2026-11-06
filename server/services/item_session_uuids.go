package services

import "github.com/tstapler/stapler-squad/session"

// nonEmptySessionUUIDs lists the session UUIDs an item owns, skipping rows
// that never got one.
func nonEmptySessionUUIDs(sessions []session.ItemSessionSummary) []string {
	uuids := make([]string, 0, len(sessions))
	for _, is := range sessions {
		if is.SessionUUID != "" {
			uuids = append(uuids, is.SessionUUID)
		}
	}
	return uuids
}
