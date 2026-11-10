package scenario

import "time"

// Triggers are workflows with a schedule, so they ride the workflows fixture.
// Tagging-rule and workflow RPC request shapes are not verified in the spike,
// so these ship as route fixtures.
func automationBundle(ref time.Time) Bundle {
	return Bundle{Routes: []RouteFixture{
		{ID: "tagging-rules", Route: "/api/session.v1.SessionService/ListTaggingRules", Seeded: SeededRoute, Rows: []Row{
			{ID: "tr-1", Kind: "tagging-rule", Title: "Tag payments work", Detail: "path contains payments", Labels: []string{"Payments"}, At: ago(ref, 5*time.Hour)},
			{ID: "tr-2", Kind: "tagging-rule", Title: "Tag infra work", Detail: "path contains infra", Labels: []string{"DevOps"}, At: ago(ref, 4*time.Hour)},
		}},
		{ID: "workflows", Route: "/api/session.v1.SessionService/ListWorkflows", Seeded: SeededRoute, Rows: []Row{
			{ID: "wf-1", Kind: "workflow", Title: "Review then merge", Detail: "idea, plan, implement, review", At: ago(ref, 6*time.Hour)},
			{ID: "wf-2", Kind: "trigger", Title: "Nightly dependency check", Detail: "cron 0 3 * * *", At: ago(ref, 8*time.Hour)},
		}},
	}}
}
