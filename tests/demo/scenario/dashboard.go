package scenario

import "time"

const mn = time.Minute

func dashboardBundle(ref time.Time) Bundle {
	rec := func(id, title, repo, branch, status, program, cat string, tags []string, created, updated time.Duration, add, rm int) SessionRecord {
		return SessionRecord{
			ID: id, Title: title, Path: "/demo/" + repo, Branch: branch, Status: status,
			Program: program, Category: cat, Tags: tags,
			CreatedAt: ago(ref, created), UpdatedAt: ago(ref, updated), Added: add, Removed: rm,
		}
	}
	return Bundle{Sessions: []SessionRecord{
		rec("dash-checkout", "checkout-retry-logic", "shop/checkout-service", "feature/retry-backoff", "active", "claude", "Backend", []string{"Payments", "Priority"}, 52*mn, 3*mn, 214, 31),
		rec("dash-search", "search-index-rebuild", "shop/search-service", "fix/stale-index", "active", "claude", "Backend", []string{"Bug", "Search"}, 38*mn, 1*mn, 96, 40),
		rec("dash-storefront", "storefront-dark-mode", "shop/storefront", "feature/dark-mode", "paused", "claude", "Frontend", []string{"React", "UX"}, 3*time.Hour, 47*mn, 388, 102),
		rec("dash-infra", "autoscaling-tuning", "shop/infra-modules", "feature/hpa-tuning", "paused", "aider", "Infrastructure", []string{"DevOps"}, 2*time.Hour, 19*mn, 61, 9),
	}}
}
