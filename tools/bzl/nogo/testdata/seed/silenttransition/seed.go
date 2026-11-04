// Package silenttransition is a deliberate silenttransition violation; never import it.
package silenttransition

import (
	"context"

	"github.com/tstapler/stapler-squad/session"
)

func logIt(format string, args ...interface{}) {}

// Violation only logs the error of a watched Storage write.
func Violation(ctx context.Context, s *session.Storage, itemID string) {
	if endErr := s.UpdateItemSessionEnded(ctx, itemID, 0); endErr != nil {
		logIt("UpdateItemSessionEnded failed: %v", endErr)
	}
}
