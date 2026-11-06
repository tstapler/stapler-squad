package unfinished

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/fsevents"
)

func TestBatchScope(t *testing.T) {
	const git = "/r/.git/"
	tests := []struct {
		name         string
		evs          []fsevents.Event
		scope        gitEventScope
		wantRelevant bool
	}{
		{"working file", []fsevents.Event{{Path: "/r/a.go", Flags: fsevents.ItemModified}}, gitEventRepoRoot, true},
		{"only .git", []fsevents.Event{{Path: "/r/.git/index"}}, gitEventRepoRoot, false},
		{"history done ignored", []fsevents.Event{{Path: "/r", Flags: fsevents.HistoryDone}}, gitEventRepoRoot, false},
		{"rescan wins even inside .git", []fsevents.Event{{Path: "/r/.git/x", Flags: fsevents.KernelDropped}}, gitEventAllWorktrees, true},
		{"empty", nil, gitEventRepoRoot, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scope, relevant := batchScope(git, tc.evs)
			require.Equal(t, tc.wantRelevant, relevant)
			require.Equal(t, tc.scope, scope)
		})
	}
}
