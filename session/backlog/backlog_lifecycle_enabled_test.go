package backlog

import "testing"

func TestBacklogLifecycleListener_IsEnabled_ReflectsSetEnabled(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	l := NewBacklogLifecycleListener(storage)
	if l.IsEnabled() {
		t.Fatal("a new listener should start disabled")
	}
	l.SetEnabled(true)
	if !l.IsEnabled() {
		t.Fatal("IsEnabled should be true after SetEnabled(true)")
	}
	l.SetEnabled(false)
	if l.IsEnabled() {
		t.Fatal("IsEnabled should be false after SetEnabled(false)")
	}
}
