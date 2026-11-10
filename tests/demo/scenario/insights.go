package scenario

import "time"

// Insights are computed server-side from session history, which cannot be
// seeded over RPC (Q5: not tested), so they ship as a route fixture.
func insightsBundle(ref time.Time) Bundle {
	days := []float64{4.2, 6.8, 5.1, 7.9, 9.3, 3.4, 8.6}
	rows := make([]Row, 0, len(days)+2)
	for i, v := range days {
		rows = append(rows, Row{ID: "spend-" + string(rune('a'+i)), Kind: "spend", Title: "Daily spend (USD)", Value: v, At: ago(ref, time.Duration(len(days)-i)*24*time.Hour)})
	}
	rows = append(rows,
		Row{ID: "finding-1", Kind: "finding", Title: "Approvals auto-resolved", Detail: "71% of prompts matched an allow rule.", Value: 71, At: ago(ref, 2*time.Hour)},
		Row{ID: "finding-2", Kind: "finding", Title: "Longest idle stretch", Detail: "storefront-dark-mode waited 47 minutes for review.", Value: 47, At: ago(ref, 3*time.Hour)},
	)
	return Bundle{Routes: []RouteFixture{{ID: "insights-summary", Route: "/api/session.v1.InsightsService/GetInsightsSummary", Seeded: SeededRoute, Rows: rows}}}
}
