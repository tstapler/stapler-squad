package session

import (
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// BacklogHooks holds the three consumer-side ports the backlog store calls after
// its mutations commit: item-change publisher, outbound callback dispatcher and
// pipeline-chain firer. Each is optional (nil = no-op), best-effort and
// recover()-guarded so a panicking adapter never reaches a mutation's return path.
// Ports are bound late (composition-root wiring runs after the repository is
// built), so each slot is an atomic pointer: a bind is race-free against readers.
// The zero value is ready to use.
type BacklogHooks struct {
	publisher  atomic.Pointer[backlogdto.ItemChangePublisher]
	callbacks  atomic.Pointer[backlogdto.CallbackDispatcher]
	chainFirer atomic.Pointer[backlogdto.ChainDispatcher]
}

// SetItemChangePublisher binds p; nil unbinds.
func (h *BacklogHooks) SetItemChangePublisher(p backlogdto.ItemChangePublisher) {
	if p == nil {
		h.publisher.Store(nil)
		return
	}
	h.publisher.Store(&p)
}

// SetCallbackDispatcher binds d; nil unbinds.
func (h *BacklogHooks) SetCallbackDispatcher(d backlogdto.CallbackDispatcher) {
	if d == nil {
		h.callbacks.Store(nil)
		return
	}
	h.callbacks.Store(&d)
}

// SetChainFirer binds f; nil unbinds.
func (h *BacklogHooks) SetChainFirer(f backlogdto.ChainDispatcher) {
	if f == nil {
		h.chainFirer.Store(nil)
		return
	}
	h.chainFirer.Store(&f)
}

// ChainDispatcher returns the bound chain firer, or nil.
func (h *BacklogHooks) ChainDispatcher() backlogdto.ChainDispatcher {
	if p := h.chainFirer.Load(); p != nil {
		return *p
	}
	return nil
}

// PublishItemChanged forwards to the bound publisher; no-op when none is bound.
func (h *BacklogHooks) PublishItemChanged(item *backlogdto.BacklogItemData, change backlogdto.BacklogItemChange) {
	p := h.publisher.Load()
	if p == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.WarningLog().Printf("[EntRepository] itemChangePublisher.PublishItemChanged panicked (recovered): %v", rec)
		}
	}()
	(*p).PublishItemChanged(item, change)
}

// DispatchCallback forwards to the bound dispatcher; no-op when none is bound.
func (h *BacklogHooks) DispatchCallback(eventType string, payload any) {
	d := h.callbacks.Load()
	if d == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.WarningLog().Printf("[EntRepository] callbackDispatcher.Dispatch panicked (recovered): %v", rec)
		}
	}()
	(*d).Dispatch(eventType, payload)
}

// DispatchChainFire forwards to the bound chain firer; no-op when none is bound.
// The firer is itself non-blocking and derives its own context, so the fire
// outlives the transition call's context.
func (h *BacklogHooks) DispatchChainFire(item *backlogdto.BacklogItemData) {
	f := h.chainFirer.Load()
	if f == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.WarningLog().Printf("[EntRepository] chainFirer.Dispatch panicked (recovered): %v", rec)
		}
	}()
	(*f).Dispatch(item)
}

// SetItemChangePublisher wires the item-change publisher (see BacklogHooks).
func (r *EntRepository) SetItemChangePublisher(p backlogdto.ItemChangePublisher) {
	r.backlogHooks.SetItemChangePublisher(p)
}

// SetCallbackDispatcher wires the outbound-callback dispatcher (see BacklogHooks).
func (r *EntRepository) SetCallbackDispatcher(d backlogdto.CallbackDispatcher) {
	r.backlogHooks.SetCallbackDispatcher(d)
}

// SetChainFirer wires the pipeline-chain firer (see BacklogHooks).
func (r *EntRepository) SetChainFirer(f backlogdto.ChainDispatcher) {
	r.backlogHooks.SetChainFirer(f)
}

// ChainDispatcher returns the wired chain firer, or nil.
func (r *EntRepository) ChainDispatcher() backlogdto.ChainDispatcher {
	return r.backlogHooks.ChainDispatcher()
}

// DispatchCallback fires an outbound callback; used by TransitionBacklogItemStatus
// and BacklogLifecycleListener.reconcileStaleWorkSessions.
func (r *EntRepository) DispatchCallback(eventType string, payload any) {
	r.backlogHooks.DispatchCallback(eventType, payload)
}

// publishItemChangedSnapshot publishes item as-is, without eager-loading its
// sessions: DeleteBacklogItem must snapshot before deleting, and AppendActivityNote's
// event never carries a full item (see publishItemChanged for the eager-loading path).
func (r *EntRepository) publishItemChangedSnapshot(item *backlogdto.BacklogItemData, change backlogdto.BacklogItemChange) {
	r.backlogHooks.PublishItemChanged(item, change)
}

func (r *EntRepository) dispatchChainFire(item *backlogdto.BacklogItemData) {
	r.backlogHooks.DispatchChainFire(item)
}
