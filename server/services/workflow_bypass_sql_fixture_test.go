package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBacklogItemStatusWriters_SQLHeuristicFlagsEveryWriteForm(t *testing.T) {
	t.Parallel()
	writes := []string{
		"UPDATE OR REPLACE backlog_items SET status = 'x'",
		"UPDATE backlog_items SET note = 'where status', status = 'x'",
		"UPDATE backlog_items SET title = 'a' /* where */, status = 'x'",
		"UPDATE backlog_items SET title = 'a' -- from here\n, status = 'x'",
		"UPDATE backlog_items SET title = (SELECT t FROM y), status = 'x'",
		"UPDATE /* c */ backlog_items SET status = 'x'",
		"UPDATE main.backlog_items SET status = 'x'",
		`UPDATE "main"."backlog_items" SET "status" = 'x'`,
		"UPDATE backlog_items AS b SET status = 'x'",
		"UPDATE backlog_items b SET status = 'x'",
		"UPDATE backlog_items SET (title, status) = ('a', 'x')",
		"WITH x AS (SELECT 1) UPDATE backlog_items SET status = 'x'",
		"UPDATE backlog_items SET title = 'a;b', status = 'x'",
		"UPDATE backlog_items\nSET title = 'a',\n status = 'x'\nWHERE id = 1",
		"INSERT INTO backlog_items VALUES (1, 'a', 'idea')",
		"REPLACE INTO backlog_items SELECT * FROM backlog_items_archive",
		"INSERT INTO main.backlog_items (status) VALUES ('x')",
		"INSERT INTO backlog_items (id) VALUES (1) ON CONFLICT (id) DO UPDATE SET status = excluded.status",
		"UPDATE backlog_items SET title = 'it''s', status = 'x'",
	}
	for _, q := range writes {
		assert.True(t, clsSQLWritesBacklogStatus(q), q)
	}
}

func TestBacklogItemStatusWriters_SQLHeuristicIgnoresNonWrites(t *testing.T) {
	t.Parallel()
	nonWrites := []string{
		"UPDATE backlog_items SET title = 'status = x'",
		"UPDATE backlog_items SET title = 'a' WHERE note = 'status = x'",
		"UPDATE backlog_items SET title = 'a' WHERE status = 'idea'",
		"UPDATE backlog_items SET title = (SELECT x FROM y WHERE status = 'a')",
		"UPDATE backlog_items SET status_reason = 'x', user_modified_status_at = 1, github_last_progress_comment_status = 'y'",
		"UPDATE backlog_items_archive SET status = 'x'",
		"INSERT INTO backlog_items_archive (status) VALUES ('x')",
		"INSERT INTO backlog_items (id, title) VALUES (1, 'a')",
		"INSERT INTO backlog_items (id) VALUES (1) ON CONFLICT (id) DO UPDATE SET title = excluded.title",
		"SELECT status FROM backlog_items WHERE status = 'idea'",
		"DELETE FROM backlog_items WHERE status = 'done'",
		"ALTER TABLE backlog_items ADD COLUMN status_extra TEXT",
		"UPDATE sessions SET status = 1",
	}
	for _, q := range nonWrites {
		assert.False(t, clsSQLWritesBacklogStatus(q), q)
	}
}
