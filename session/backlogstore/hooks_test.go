package backlogstore

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

type hookPublisher struct {
	fn func(*backlogdto.BacklogItemData, backlogdto.BacklogItemChange)
}

func (p hookPublisher) PublishItemChanged(i *backlogdto.BacklogItemData, c backlogdto.BacklogItemChange) {
	p.fn(i, c)
}

type hookDispatcher struct{ fn func(string, any) }

func (d hookDispatcher) Dispatch(e string, p any) { d.fn(e, p) }

type hookChain struct {
	fn func(*backlogdto.BacklogItemData)
}

func (c hookChain) Dispatch(i *backlogdto.BacklogItemData) { c.fn(i) }

func TestBacklogHooks_should_noOp_When_NothingBound(t *testing.T) {
	t.Parallel()
	var h BacklogHooks
	assert.NotPanics(t, func() {
		h.PublishItemChanged(&backlogdto.BacklogItemData{}, backlogdto.BacklogItemChange{})
		h.DispatchCallback("e", nil)
		h.DispatchChainFire(&backlogdto.BacklogItemData{})
	})
	assert.Nil(t, h.ChainDispatcher())
}

func TestBacklogHooks_should_swallowPanics_When_AnyBoundPortPanics(t *testing.T) {
	t.Parallel()
	var h BacklogHooks
	h.SetItemChangePublisher(hookPublisher{func(*backlogdto.BacklogItemData, backlogdto.BacklogItemChange) { panic("p") }})
	h.SetCallbackDispatcher(hookDispatcher{func(string, any) { panic("d") }})
	h.SetChainFirer(hookChain{func(*backlogdto.BacklogItemData) { panic("c") }})
	assert.NotPanics(t, func() {
		h.PublishItemChanged(&backlogdto.BacklogItemData{}, backlogdto.BacklogItemChange{})
		h.DispatchCallback("e", nil)
		h.DispatchChainFire(&backlogdto.BacklogItemData{})
	})
}

func TestBacklogHooks_should_invokeBoundPort_And_rebindAndUnbind_ForEachPort(t *testing.T) {
	t.Parallel()
	var h BacklogHooks
	item := &backlogdto.BacklogItemData{}
	var pubA, pubB, cbA, cbB, chA, chB int
	fire := func() {
		h.PublishItemChanged(item, backlogdto.BacklogItemChange{})
		h.DispatchCallback("e", nil)
		h.DispatchChainFire(item)
	}

	fire()
	h.SetItemChangePublisher(hookPublisher{func(*backlogdto.BacklogItemData, backlogdto.BacklogItemChange) { pubA++ }})
	h.SetCallbackDispatcher(hookDispatcher{func(string, any) { cbA++ }})
	h.SetChainFirer(hookChain{func(*backlogdto.BacklogItemData) { chA++ }})
	fire()
	assert.Equal(t, []int{1, 1, 1}, []int{pubA, cbA, chA}, "bound ports are invoked once each")
	assert.NotNil(t, h.ChainDispatcher())

	h.SetItemChangePublisher(hookPublisher{func(*backlogdto.BacklogItemData, backlogdto.BacklogItemChange) { pubB++ }})
	h.SetCallbackDispatcher(hookDispatcher{func(string, any) { cbB++ }})
	h.SetChainFirer(hookChain{func(*backlogdto.BacklogItemData) { chB++ }})
	fire()
	assert.Equal(t, []int{1, 1, 1}, []int{pubA, cbA, chA}, "rebinding stops the old ports")
	assert.Equal(t, []int{1, 1, 1}, []int{pubB, cbB, chB}, "rebinding replaces with the new ports")

	h.SetItemChangePublisher(nil)
	h.SetCallbackDispatcher(nil)
	h.SetChainFirer(nil)
	fire()
	assert.Equal(t, []int{1, 1, 1}, []int{pubB, cbB, chB}, "unbinding stops calls")
	assert.Nil(t, h.ChainDispatcher())
}

func TestBacklogHooks_should_neverLoseCalls_When_BoundWhileDispatching(t *testing.T) {
	t.Parallel()
	var h BacklogHooks
	var calls atomic.Int64
	pub := hookPublisher{func(*backlogdto.BacklogItemData, backlogdto.BacklogItemChange) { calls.Add(1) }}
	h.SetItemChangePublisher(pub)
	const workers, iters = 4, 200
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				h.SetItemChangePublisher(pub)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				h.PublishItemChanged(&backlogdto.BacklogItemData{}, backlogdto.BacklogItemChange{})
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(workers*iters), calls.Load(), "a publisher is always bound, so no publish may be dropped")
}
