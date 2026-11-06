package repostate

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProduct struct {
	bytes int64
	log   *[]string
	name  string
}

func (f *fakeProduct) Bytes() int64 { return f.bytes }
func (f *fakeProduct) Drop() {
	*f.log = append(*f.log, f.name)
	f.bytes = 0
}

func TestRepoState_ShouldDropProductsThenGraphThenInterner_WhenOverBudget(t *testing.T) {
	build := func(budget int64, order *[]string) *RepoState {
		st := NewRepoState(Options{}, budget)
		// Registered out of tier order on purpose.
		require.True(t, st.Register(TierInterner, &fakeProduct{100, order, "interner"}))
		require.True(t, st.Register(TierProducts, &fakeProduct{100, order, "reach"}))
		require.True(t, st.Register(TierGraph, &fakeProduct{100, order, "graph"}))
		return st
	}
	var order []string
	st := build(250, &order)
	assert.EqualValues(t, 300, st.Bytes())
	st.Prune(false)
	assert.Equal(t, []string{"reach"}, order, "within budget after the first tier: stop")

	order = nil
	build(50, &order).Prune(false)
	assert.Equal(t, []string{"reach", "graph", "interner"}, order)
}

func TestRepoState_ShouldDropEverything_WhenSeverePrune(t *testing.T) {
	var order []string
	st := NewRepoState(Options{}, 1<<30)
	st.Register(TierGraph, &fakeProduct{1, &order, "graph"})
	st.Register(TierProducts, &fakeProduct{1, &order, "reach"})
	st.Prune(true)
	assert.Equal(t, []string{"reach", "graph"}, order)
	assert.EqualValues(t, 0, st.Bytes())
}

func TestRepoState_ShouldReportRepoCacheBytes_WhenRepoCacheRegistered(t *testing.T) {
	c := NewRepoCache[string, int](1000, 0, func(int) int { return 10 }, Options{})
	st := NewRepoState(Options{}, 5)
	require.True(t, st.Register(TierProducts, c))
	c.Put("a", 1)
	c.Put("b", 2)
	assert.EqualValues(t, 20, st.Bytes())
	st.Prune(false)
	assert.EqualValues(t, 0, st.Bytes())
	assert.Equal(t, 0, c.Stats().Entries)
}

func TestRepoState_ShouldRefuseRegisterAndReportZero_WhenClosed(t *testing.T) {
	var order []string
	st := NewRepoState(Options{}, 10)
	st.Register(TierProducts, &fakeProduct{50, &order, "a"})
	st.Close()
	st.Close() // idempotent
	assert.EqualValues(t, 0, st.Bytes())
	assert.False(t, st.Register(TierProducts, &fakeProduct{1, &order, "b"}))
	assert.Empty(t, order, "Close detaches without dropping")
}

func TestRepoState_ShouldReflectPressureFunc_WhenSet(t *testing.T) {
	assert.False(t, NewRepoState(Options{}, 1).UnderPressure())
	assert.True(t, NewRepoState(Options{PressureFunc: func() bool { return true }}, 1).UnderPressure())
}

func TestRepoState_ShouldKeepEveryRegistration_WhenConcurrent(t *testing.T) {
	st := NewRepoState(Options{}, 1<<30)
	const n = 64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var order []string
			st.Register(Tier(i%int(numTiers)), &fakeProduct{1, &order, "p"})
		}()
	}
	wg.Wait()
	assert.EqualValues(t, n, st.Bytes())
}
