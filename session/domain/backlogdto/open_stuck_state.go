package backlogdto

import (
	"github.com/tstapler/stapler-squad/session/domain"
)

// findOpenStuckStateFor returns the row for (itemID, reason) from rows, if present.
func FindOpenStuckStateFor(rows []OpenStuckStateData, itemID string, reason domain.StuckReason) (OpenStuckStateData, bool) {
	for _, row := range rows {
		if row.ItemID == itemID && row.Reason == reason {
			return row, true
		}
	}
	return OpenStuckStateData{}, false
}
