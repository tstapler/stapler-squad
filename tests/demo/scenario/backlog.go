package scenario

import "time"

func backlogBundle(ref time.Time) Bundle {
	item := func(id, title, desc, status string, prio int, age time.Duration) BacklogItem {
		return BacklogItem{ID: id, Title: title, Description: desc, Status: status, Priority: prio, CreatedAt: ago(ref, age)}
	}
	return Bundle{
		Flags: []FlagSetting{{Name: "backlog", Enabled: true}},
		Backlog: []BacklogItem{
			item("bl-1", "Add retry budget to the checkout client", "Cap retries so a slow payment provider cannot exhaust the pool.", "idea", 2, 6*time.Hour),
			item("bl-2", "Paginate the order history endpoint", "Return 50 orders per page with a stable cursor.", "ready", 3, 5*time.Hour),
			item("bl-3", "Cache product search facets", "Facet counts are recomputed on every query.", "in_progress", 2, 4*time.Hour),
			item("bl-4", "Dark mode contrast fixes", "Several badges fail contrast in the dark palette.", "review", 1, 3*time.Hour),
			item("bl-5", "Document the webhook signing scheme", "Add a how-to for verifying signatures.", "done", 1, 2*time.Hour),
		},
	}
}
