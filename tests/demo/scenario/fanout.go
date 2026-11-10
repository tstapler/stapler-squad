package scenario

import "time"

func fanoutBundle(ref time.Time) Bundle {
	rec := func(id, title, branch, program string, updated time.Duration, add int) SessionRecord {
		return SessionRecord{
			ID: id, Title: title, Path: "/demo/shop/pricing-service", Branch: branch, Status: "paused",
			Program: program, Category: "Backend", Tags: []string{"Fan-out", "Pricing"},
			CreatedAt: ago(ref, 30*time.Minute), UpdatedAt: ago(ref, updated), Added: add, Removed: add / 5,
		}
	}
	return Bundle{Sessions: []SessionRecord{
		rec("fan-a", "pricing-cache-variant-a", "try/lru-cache", "claude", 4*time.Minute, 120),
		rec("fan-b", "pricing-cache-variant-b", "try/ttl-cache", "claude", 5*time.Minute, 88),
		rec("fan-c", "pricing-cache-variant-c", "try/singleflight", "aider", 6*time.Minute, 142),
	}}
}
