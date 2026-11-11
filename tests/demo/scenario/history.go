package scenario

import "time"

// History entries mirror the synthetic ~/.claude-style history; the seam found
// in the spike is not reachable over RPC, so they ship as a route fixture.
func historyBundle(ref time.Time) Bundle {
	h := func(id, title, detail string, age time.Duration) Row {
		return Row{ID: id, Kind: "history", Title: title, Detail: detail, At: ago(ref, age)}
	}
	return Bundle{Routes: []RouteFixture{{
		ID: "claude-history", Route: "/api/session.v1.SessionService/SearchClaudeHistory", Seeded: SeededRoute,
		Rows: []Row{
			h("h-1", "Fix flaky checkout test", "Pinned the clock and removed a real sleep.", 26*time.Hour),
			h("h-2", "Add pagination to order history", "Cursor-based, 50 per page.", 50*time.Hour),
			h("h-3", "Tune autoscaling thresholds", "Raised target CPU to 65 percent.", 74*time.Hour),
		},
	}}}
}
