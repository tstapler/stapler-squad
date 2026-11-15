package dbpool

import "sync"

// EntSchemaCreateMu serializes calls to (*ent.Client).Schema.Create across the
// whole process, including from other packages that migrate their own
// *ent.Client against the same generated ent package (e.g.
// server/analytics.OpenAnalyticsDB). entgo.io/ent/dialect/sql/schema.(*Atlas)
// has internal package-level state that data-races when multiple goroutines
// run schema migration concurrently (e.g. `go test -parallel` spinning up
// many independent clients at once) — each SQLite connection is isolated, but
// Atlas itself is not safe for concurrent use. Lives here, not in session, so
// leaf stores and their test harness can lock it without importing session.
var EntSchemaCreateMu sync.Mutex
