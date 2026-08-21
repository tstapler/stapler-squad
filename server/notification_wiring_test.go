package server

// Enforcement test for Bug 2 — SetNotificationStore called after StartSubscriber.
//
// Pre-fix: wireDepsIntoServer called notifications.StartSubscriber before
// deps.SessionService.SetNotificationStore, so a goroutine started by the
// subscriber could race with GetNotificationStore() returning nil.
//
// Fix: SetNotificationStore is now called first (line ~190 of server.go).
//
// This test reads server.go and asserts the correct source ordering. It will
// fail if the two lines are swapped back, catching the regression before it ships.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWireDeps_SetNotificationStoreBeforeStartSubscriber(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	serverGoPath := filepath.Join(filepath.Dir(thisFile), "server.go")
	src, err := os.ReadFile(serverGoPath)
	if err != nil {
		t.Fatalf("could not read server.go: %v", err)
	}

	setIdx := strings.Index(string(src), "SetNotificationStore(notifStore)")
	subIdx := strings.Index(string(src), "notifications.StartSubscriber(")

	if setIdx == -1 {
		t.Fatal("SetNotificationStore(notifStore) not found in server.go — was it renamed?")
	}
	if subIdx == -1 {
		t.Fatal("notifications.StartSubscriber( not found in server.go — was it renamed?")
	}

	// Pre-fix: subIdx < setIdx (subscriber started before store wired).
	// Post-fix: setIdx < subIdx (store wired before subscriber starts).
	if setIdx > subIdx {
		t.Errorf(
			"Bug 2 regression: SetNotificationStore (byte %d) appears AFTER StartSubscriber (byte %d) in server.go.\n"+
				"SetNotificationStore must be called first so GetNotificationStore() is non-nil\n"+
				"when the subscriber goroutine begins processing events.",
			setIdx, subIdx,
		)
	}
}
