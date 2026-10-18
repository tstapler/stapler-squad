package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	assert.Eventually(t, func() bool { return n.Load() == 1 }, time.Second, time.Millisecond)
	time.Sleep(30 * time.Millisecond) // lets a wrongly-armed second fire show up
	assert.EqualValues(t, 1, n.Load(), "burst must fire exactly once")

	cancel()
	d.poke()
	time.Sleep(30 * time.Millisecond)
	assert.EqualValues(t, 1, n.Load(), "no fire after ctx cancelled")
}
