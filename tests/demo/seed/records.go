package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/tests/demo/scenario"
)

// writeRecords persists scenario session records into dir/sessions.db, the
// same file tests/demo's SeedMockSessions writes.
func writeRecords(dir string, recs []scenario.SessionRecord) error {
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(dir, "sessions.db")))
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}
	defer repo.Close()
	ctx := context.Background()
	for _, r := range recs {
		if err := repo.Create(ctx, toInstanceData(r)); err != nil {
			return fmt.Errorf("create session %q: %w", r.Title, err)
		}
	}
	return nil
}

func toInstanceData(r scenario.SessionRecord) session.InstanceData {
	status := session.Paused
	if r.Status == "active" {
		status = session.Active
	}
	return session.InstanceData{
		Title: r.Title, Path: r.Path, Branch: r.Branch, Status: status, Program: r.Program,
		Category: r.Category, Tags: r.Tags, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		LastTerminalUpdate: r.UpdatedAt, LastMeaningfulOutput: r.UpdatedAt,
		DiffStats: session.DiffStatsData{Added: r.Added, Removed: r.Removed},
	}
}
