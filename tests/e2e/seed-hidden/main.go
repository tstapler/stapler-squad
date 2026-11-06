// seed-hidden writes one hidden review session into a test data directory before the
// server boots, so the delivery gate's visibility index is seeded with a hidden kind.
// The e2e session client cannot create hidden sessions (only backlog dispatch does).
//
// Usage: go run ./tests/e2e/seed-hidden <dir>
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// hiddenReviewTitle is the title the e2e specs address the session by.
const hiddenReviewTitle = "e2e-hidden-review"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: seed-hidden <dir>")
		os.Exit(1)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create dir: %v\n", err)
		os.Exit(1)
	}
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(dir, "sessions.db")))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open repository: %v\n", err)
		os.Exit(1)
	}
	defer repo.Close()

	now := time.Now()
	data := session.InstanceData{
		Title:     hiddenReviewTitle,
		Path:      filepath.Join(dir, "hidden-review"),
		Branch:    "main",
		Status:    session.Paused,
		Program:   "claude",
		Category:  "Backlog",
		Tags:      []string{"backlog:review"},
		Hidden:    true,
		CreatedAt: now.Add(-10 * time.Minute),
		UpdatedAt: now,
	}
	if err := repo.Create(context.Background(), data); err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("seeded hidden session %q into %s\n", hiddenReviewTitle, dir)
}
