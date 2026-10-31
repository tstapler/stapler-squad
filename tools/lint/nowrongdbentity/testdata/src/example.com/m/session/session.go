package session

import "example.com/m/session/ent"

func coreSide(c *ent.Client) {
	_ = c.Session     // sessions.db entity on the sessions.db side
	_ = c.EscapeEvent // want `entity EscapeEvent lives in analytics.db`

	_ = c.EscapeEvent //nolint:nowrongdbentity known latent bug, tracked in ADR-036

	//nolint:nowrongdbentity
	_ = c.EscapeEvent // want `needs a one-line reason`
}

func coreSideTx(tx *ent.Tx) {
	_ = tx.Session
	_ = tx.EscapeEvent // want `entity EscapeEvent lives in analytics.db`
}
