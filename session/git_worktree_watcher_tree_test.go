package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/internal/fsevents"
)

func TestTreeEventsRelevant(t *testing.T) {
	t.Parallel()
	git := "/w/.git"
	tests := []struct {
		name string
		evs  []fsevents.Event
		want bool
	}{
		{"working tree file", []fsevents.Event{{Path: "/w/main.go", Flags: fsevents.ItemModified}}, true},
		{"git dir file ignored", []fsevents.Event{{Path: "/w/.git/index", Flags: fsevents.ItemModified}}, false},
		{"sibling named .gitignore counts", []fsevents.Event{{Path: "/w/.gitignore", Flags: fsevents.ItemModified}}, true},
		{"rescan flag inside .git still counts", []fsevents.Event{{Path: "/w/.git", Flags: fsevents.KernelDropped}}, true},
		{"history done alone ignored", []fsevents.Event{{Path: "/w", Flags: fsevents.HistoryDone}}, false},
		{"root changed counts", []fsevents.Event{{Path: "/w", Flags: fsevents.RootChanged}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, treeEventsRelevant(git, tc.evs))
		})
	}
}

func TestTrailingDebouncer_CoalescesBurstAndStopsAfterCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	var n atomic.Int32
	d := &trailingDebouncer{ctx: ctx, wait: 10 * time.Millisecond, fn: func() { n.Add(1) }}

	for i := 0; i < 50; i++ {
		d.poke()
	}
	require.Eventually(t, func() bool { return n.Load() >= 1 }, time.Second, time.Millisecond)

	// All 50 pokes landed before the first fire, so a coalescing bug would
	// already show as n > 1; a second burst must then fire exactly once more.
	require.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return !d.armed
	}, time.Second, time.Millisecond)
	assert.EqualValues(t, 1, n.Load(), "first burst must fire exactly once")
	d.poke()
	require.Eventually(t, func() bool { return n.Load() == 2 }, time.Second, time.Millisecond)

	cancel()
	d.poke()
	d.mu.Lock()
	armed := d.armed
	d.mu.Unlock()
	assert.False(t, armed, "poke after ctx cancelled must not arm a timer")
}
