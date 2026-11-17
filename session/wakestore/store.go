// Package wakestore persists agent wake triggers (ADR-006). session.EntRepository
// embeds *Triggers so existing callers keep their method set.
package wakestore

import (
	"sync/atomic"

	"github.com/tstapler/stapler-squad/session/ent"
)

// Triggers owns the wake_triggers table.
type Triggers struct {
	client *ent.Client
	// wakeChanged runs after any row change so the wake timer can re-arm (SetWakeChangeHook).
	wakeChanged atomic.Pointer[func()]
}

// New returns Triggers over client, which must already hold the migrated schema.
func New(client *ent.Client) *Triggers {
	return &Triggers{client: client}
}
