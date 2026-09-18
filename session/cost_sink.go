package session

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/headless"
)

// CostSinkForSessionUUID builds a headless.CostSink that attributes a headless
// call's cost to the ItemSession (if any) for sessionUUID. This is the shared
// primitive for pipeline call sites that have a session UUID in scope but no
// ItemSession ID directly at hand (autonomous fix-loop turns, tool-approval
// checks, ad hoc one-shot calls) — one lookup+update path instead of each caller
// reimplementing it, so cost persistence can't quietly diverge per call site.
func CostSinkForSessionUUID(storage *Storage, sessionUUID string) headless.CostSink {
	return func(usd float64) {
		if usd <= 0 || storage == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := storage.AddHeadlessCostBySessionUUID(ctx, sessionUUID, usd); err != nil {
			log.WarningLog().Printf("[CostSink] failed to persist headless call cost for session %s: %v", sessionUUID, err)
		}
	}
}
