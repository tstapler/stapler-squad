// Package backlogstore is the persistence leaf for the backlog tables.
// session.EntRepository embeds *Backlog so existing callers keep their method
// set; the rest of the backlog store follows in later E10 moves.
package backlogstore

import (
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

// Backlog owns the item-source, sync-event and (later) backlog item tables and
// the late-bound hooks its mutations call after commit.
type Backlog struct {
	client *ent.Client
	hooks  BacklogHooks
}

// New returns a Backlog over client, which must already hold the migrated schema.
func New(client *ent.Client) *Backlog {
	return &Backlog{client: client}
}

// Hooks returns the late-bound publisher/callback/chain ports.
func (b *Backlog) Hooks() *BacklogHooks { return &b.hooks }

// SetItemChangePublisher wires the item-change publisher (see BacklogHooks).
func (b *Backlog) SetItemChangePublisher(p backlogdto.ItemChangePublisher) {
	b.hooks.SetItemChangePublisher(p)
}

// SetCallbackDispatcher wires the outbound-callback dispatcher (see BacklogHooks).
func (b *Backlog) SetCallbackDispatcher(d backlogdto.CallbackDispatcher) {
	b.hooks.SetCallbackDispatcher(d)
}

// SetChainFirer wires the pipeline-chain firer (see BacklogHooks).
func (b *Backlog) SetChainFirer(f backlogdto.ChainDispatcher) {
	b.hooks.SetChainFirer(f)
}

// ChainDispatcher returns the wired chain firer, or nil.
func (b *Backlog) ChainDispatcher() backlogdto.ChainDispatcher {
	return b.hooks.ChainDispatcher()
}

// DispatchCallback fires an outbound callback; used by TransitionBacklogItemStatus
// and BacklogLifecycleListener.reconcileStaleWorkSessions.
func (b *Backlog) DispatchCallback(eventType string, payload any) {
	b.hooks.DispatchCallback(eventType, payload)
}
