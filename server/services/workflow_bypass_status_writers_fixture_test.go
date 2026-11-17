package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeStatusFixture(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "server"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server", "x.go"), []byte("package server\n"+src+"\n"), 0o600))
	return dir
}

func TestBacklogItemStatusWriters_EveryVariantIsFlaggedOutsideTheAllowList(t *testing.T) {
	t.Parallel()
	variants := map[string]string{
		"SetNillableStatus":   `func V(c *entClient, id int, s *string) { c.BacklogItem.UpdateOneID(id).SetNillableStatus(s).Save(nil) }`,
		"SetNillableOnCreate": `func V(c *entClient, s *string) { c.BacklogItem.Create().SetNillableStatus(s).Save(nil) }`,
		"BulkUpdateSetStatus": `func V(c *entClient) { c.BacklogItem.Update().SetStatus("done").Save(nil) }`,
		"CreateSetStatus":     `func V(c *entClient) { c.BacklogItem.Create().SetStatus("done").Save(nil) }`,
		"UpsertUpdateStatus":  `func V(c *entClient) { c.BacklogItem.Create().OnConflict().UpdateStatus().Exec(nil) }`,
		"UpsertSetStatus":     `func V(u *ent.BacklogItemUpsertOne) { u.SetStatus("done") }`,
		"MutationSetStatus":   `func V(m *ent.BacklogItemMutation) { m.SetStatus("done") }`,
		"MutationSetField":    `func V(m *ent.BacklogItemMutation) { _ = m.SetField(backlogitem.FieldStatus, "done") }`,
		"MutationSetFieldStr": `func V(m *ent.BacklogItemMutation) { _ = m.SetField("status", "done") }`,
		"RawUpdateSQL":        "func V(db *sql.DB) { db.Exec(`UPDATE backlog_items SET status = 'done' WHERE id = 1`) }",
		"RawInsertSQL":        `func V(db *sql.DB) { db.Exec("INSERT INTO backlog_items (title, status) VALUES ('a', 'idea')") }`,
	}
	for name, body := range variants {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			problems := clsSetStatusViolations(t, writeStatusFixture(t, body), nil)
			require.Len(t, problems, 1, strings.Join(problems, "\n"))
			assert.Contains(t, problems[0], "server/x.go V")
		})
	}
}

func TestBacklogItemStatusWriters_UnrelatedWritesAreNotFlagged(t *testing.T) {
	t.Parallel()
	src := `func A(c *entClient, id int, s *string) { c.Session.UpdateOneID(id).SetNillableStatus(s).Save(nil) }
func B(c *entClient) { c.BacklogItem.Create().OnConflict().UpdateTitle().Exec(nil) }
func C(m *ent.BacklogItemMutation) { _ = m.SetField(backlogitem.FieldTitle, "x") }
func D(m *ent.SessionMutation) { _ = m.SetField("status", 1) }
func E(db *sql.DB) { db.Exec("UPDATE backlog_items SET title = 'x'") }
func F(db *sql.DB) { db.Exec("UPDATE sessions SET status = 1") }`
	problems := clsSetStatusViolations(t, writeStatusFixture(t, src), nil)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}
