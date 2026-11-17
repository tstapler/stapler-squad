package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
)

func TestHistoryHooks_RegisteredWhenSessionLinked(t *testing.T) {
	t.Parallel()
	if instance.HistoryRegistryHook == nil {
		t.Error("instance.HistoryRegistryHook is nil: session/history_wiring.go must blank-import session/historyadapters")
	}
	if instance.HistoryPorterHook == nil {
		t.Error("instance.HistoryPorterHook is nil: session/history_wiring.go must blank-import session/historyadapters")
	}
}
