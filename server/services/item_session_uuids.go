package services

import "github.com/tstapler/stapler-squad/session/domain/backlogdto"

// nonEmptySessionUUIDs lists the session UUIDs an item owns, skipping rows
// that never got one.
func NonEmptySessionUUIDs(sessions []backlogdto.ItemSessionSummary) []string {
	uuids := make([]string, 0, len(sessions))
	for _, is := range sessions {
		if is.SessionUUID != "" {
			uuids = append(uuids, is.SessionUUID)
		}
	}
	return uuids
}
