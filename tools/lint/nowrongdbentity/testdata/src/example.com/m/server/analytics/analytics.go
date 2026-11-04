package analytics

import "example.com/m/session/ent"

func analyticsSide(c *ent.Client) {
	_ = c.AnalyticsEvent
	_ = c.EscapeEvent // want `entity EscapeEvent lives in db/escape_events.db but this package holds the analytics.db client`
	_ = c.Session     // want `entity Session lives in sessions.db`
}
