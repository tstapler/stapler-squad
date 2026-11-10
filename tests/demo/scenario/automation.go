package scenario

import "time"

// Tagging rules and workflows (a cron workflow is a trigger) are seeded over RPC.
func automationBundle(_ time.Time) Bundle {
	return Bundle{
		TaggingRules: []TaggingRule{
			{ID: "tr-1", Name: "Tag payments work", PathPattern: "payments", OutputTag: "Payments", Priority: 10},
			{ID: "tr-2", Name: "Tag infra work", PathPattern: "infra", OutputTag: "DevOps", Priority: 20},
		},
		Workflows: []Workflow{
			{Slug: "review-then-merge", Name: "Review then merge", Description: "idea, plan, implement, review", Command: "/review"},
			{Slug: "nightly-dependency-check", Name: "Nightly dependency check", Description: "cron 0 3 * * *", Command: "/deps", Cron: "0 3 * * *"},
		},
	}
}
