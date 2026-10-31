package analytics

import "example.com/m/session/ent"

func escapeSide(c *ent.Client) {
	_ = c.EscapeEvent
	_ = c.Session // want `entity Session lives in sessions.db`
}
