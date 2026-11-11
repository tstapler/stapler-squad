package scenario

import "time"

// filesBundle seeds one session over the fixture repo (see fixtures.InitRepo);
// the repo itself is created at RPC-seeding time under Deps.WorkDir.
func filesBundle(ref time.Time) Bundle {
	return Bundle{Sessions: []SessionRecord{{
		ID: "files-docs", Title: "docs-site-cleanup", Path: "/demo/shop/docs-site", Branch: "chore/docs-cleanup",
		Status: "paused", Program: "claude", Category: "Frontend", Tags: []string{"Docs"},
		CreatedAt: ago(ref, 70*time.Minute), UpdatedAt: ago(ref, 9*time.Minute), Added: 54, Removed: 12,
	}}}
}
