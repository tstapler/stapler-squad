package server

import (
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// attentionOptionsSource adapts the live-settable attention flags to the session
// package, which cannot import server/services.
func attentionOptionsSource() (session.DeterminerOptions, session.RuleOptions) {
	f := services.LoadAttentionFlags()
	return session.DeterminerOptions{
			IdleClockOutput: f.IdleClockOutput,
			ProseQuestion:   f.ProseQuestionDetection,
		},
		session.RuleOptions{AckBypassFix: f.AckBypassFix}
}
