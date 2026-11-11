package scenario

import (
	"fmt"
	"time"
)

// FanoutGroupID is the group the fanout scenario seeds; tour scenes open /fanout?groupId=<it>.
const FanoutGroupID = "demo-pricing"

func fanoutBundle(ref time.Time) Bundle {
	// Group membership is carried as tags (web-app useFanoutService.ts), so /fanout?groupId=FanoutGroupID lists these three.
	rec := func(id, title, branch, program string, variant int, updated time.Duration, add int) SessionRecord {
		return SessionRecord{
			ID: id, Title: title, Path: "/demo/shop/pricing-service", Branch: branch, Status: "paused",
			Program: program, Category: "Backend", Tags: []string{"Fan-out", "Pricing", "fanout:" + FanoutGroupID, fmt.Sprintf("fanout-variant:%d", variant)},
			CreatedAt: ago(ref, 30*time.Minute), UpdatedAt: ago(ref, updated), Added: add, Removed: add / 5,
		}
	}
	return Bundle{Sessions: []SessionRecord{
		rec("fan-a", "pricing-cache-variant-a", "try/lru-cache", "claude", 1, 4*time.Minute, 120),
		rec("fan-b", "pricing-cache-variant-b", "try/ttl-cache", "claude", 2, 5*time.Minute, 88),
		rec("fan-c", "pricing-cache-variant-c", "try/singleflight", "aider", 3, 6*time.Minute, 142),
	}}
}
