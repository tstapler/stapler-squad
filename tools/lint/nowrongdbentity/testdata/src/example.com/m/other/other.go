package other

import "example.com/m/session/ent"

func neutral(c *ent.Client) {
	_ = c.EscapeEvent // packages outside the two owners are not judged
	_ = c.Session
}
