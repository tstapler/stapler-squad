// Package nowrongdbentity is a deliberate nowrongdbentity violation (a /session/ package using an analytics entity); never import it.
package nowrongdbentity

import "github.com/tstapler/stapler-squad/session/ent"

// Violation selects an analytics.db entity from the sessions.db client.
func Violation(c *ent.Client) { _ = c.AnalyticsEvent }
