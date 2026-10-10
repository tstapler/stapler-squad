package session

// backlog_item_legacy_import_external_backfill.go — idempotent startup
// migration that fills ExternalID/ExternalURL on backlog items created by an
// older hand-built GitHub import path. That path wrote only
// `Notes: "Imported from <issue-or-pr-url>"`, so the UI showed no provenance
// chip or SourceSection for them.
//
// Deliberately a plain ent field update, not Storage.CreateBacklogItem: claims
// (ClaimRecorder / claim gossip) are recorded only on that creation path, so
// this migration neither creates claim records nor advertises these legacy
// items to peers.

import (
	"context"
	"regexp"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

const legacyImportNotesPrefix = "Imported from "

// legacyImportNotesPattern matches only the exact legacy form; free-text notes
// that merely mention an issue never match.
var legacyImportNotesPattern = regexp.MustCompile( //nolint:gochecknoglobals
	`^Imported from (https://[^/\s]+/[\w.-]+/[\w.-]+/(?:issues|pull)/(\d+))\s*$`)

// parseLegacyImportNotes returns the issue/PR URL and number from notes in the
// exact legacy "Imported from <url>" form.
func parseLegacyImportNotes(notes string) (url, number string, ok bool) {
	m := legacyImportNotesPattern.FindStringSubmatch(notes)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// runBacklogItemLegacyImportExternalBackfill sets external_url/external_id on
// items that have both empty and legacy-import notes. Idempotent: filled rows
// are excluded by the query, and a non-empty field is never overwritten.
// updated_at is preserved so recency ordering and CAS preconditions are
// unaffected. Best-effort per row.
func runBacklogItemLegacyImportExternalBackfill(ctx context.Context, er *EntRepository) error {
	rows, err := er.client.BacklogItem.Query().
		Where(
			backlogitem.NotesHasPrefix(legacyImportNotesPrefix),
			backlogitem.Or(backlogitem.ExternalURLIsNil(), backlogitem.ExternalURL("")),
			backlogitem.Or(backlogitem.ExternalIDIsNil(), backlogitem.ExternalID("")),
		).
		All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create).
		return nil //nolint:nilerr
	}

	var migrated int
	for _, row := range rows {
		url, number, ok := parseLegacyImportNotes(row.Notes)
		if !ok {
			continue
		}
		if _, saveErr := er.client.BacklogItem.UpdateOneID(row.ID).
			SetExternalURL(url).
			SetExternalID(number).
			SetUpdatedAt(row.UpdatedAt).
			Save(ctx); saveErr != nil {
			log.WarningLog().Printf("[Migration] backlog item legacy import external backfill: item=%s: %v", row.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog().Printf("[Migration] backlog item legacy import external backfill: populated %d row(s)", migrated)
	}
	return nil
}
