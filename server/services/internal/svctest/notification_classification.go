package svctest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
)

// requireOneNotification waits briefly for exactly one EventNotification on ch and
// returns it, failing the test otherwise.
func RequireOneNotification(t *testing.T, ch <-chan *events.Event) *events.Event {
	t.Helper()
	select {
	case ev := <-ch:
		require.Equal(t, events.EventNotification, ev.Type)
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event, got none")
		return nil
	}
}
