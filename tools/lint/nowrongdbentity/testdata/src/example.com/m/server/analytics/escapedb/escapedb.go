package escapedb

import "example.com/m/session/ent"

func escapeEventsSide(c *ent.Client, tx *ent.Tx) {
	_ = c.EscapeEvent
	_ = tx.EscapeEvent
	_ = c.AnalyticsEvent // want `entity AnalyticsEvent lives in analytics.db but this package holds the db/escape_events.db client`
}
