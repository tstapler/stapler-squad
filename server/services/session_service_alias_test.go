package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// TestCreateSession_PathGuard_AliasNameBypassesPathRequired verifies that when
// alias_name is set and path is empty, the "path is required" guard does NOT
// fire (i.e. the response error is not CodeInvalidArgument).
//
// The service will ultimately return CodeNotFound (alias not in config) or
// CodeInternal, but never CodeInvalidArgument for the path guard, because the
// guard condition explicitly exempts requests that carry an AliasName.
func TestCreateSession_PathGuard_AliasNameBypassesPathRequired(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	req := connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:     "test",
		AliasName: "myalias",
		Path:      "",
	})

	_, err := svc.CreateSession(context.Background(), req)

	// The path guard must NOT have fired.
	if err != nil {
		if connectErr, ok := err.(*connect.Error); ok {
			if connectErr.Code() == connect.CodeInvalidArgument {
				t.Fatalf("path guard fired for alias request: %v", err)
			}
			// CodeNotFound (alias not in config) or CodeInternal are both acceptable.
		}
	}
}

// TestCreateSession_should_ReturnSynchronousNotFound_When_AliasNameDoesNotExist
// is Task 2.2.2b-2's regression test: the alias-existence check
// (config.FindAlias, Task 2.1.1a-2) must stay in CreateSession's synchronous
// prefix, ahead of instance construction and the Background Resolution
// Pipeline -- never move into the pipeline itself, where an invalid alias
// would surface as an async Failed card instead of the synchronous NotFound
// RPC error requirements.md's Constraints and design/ux.md's Surface 1
// mandate. Asserts all three: the synchronous CodeNotFound response, that no
// SessionCreatedEvent ever fires, and that no instance is ever persisted
// (including via a short poll for a subsequent Failed card, which must never
// appear).
func TestCreateSession_should_ReturnSynchronousNotFound_When_AliasNameDoesNotExist(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	eventCh, _ := eventBus.Subscribe(ctx)

	const title = "alias-notfound-regression-session"
	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:     title,
		AliasName: "definitely-does-not-exist-alias",
	}))

	require.Nil(t, resp, "an invalid alias must never produce a CreateSessionResponse/Instance")
	require.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok, "expected *connect.Error, got %T", err)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())

	// No SessionCreatedEvent (or any event at all) may fire for a request
	// that never got past the synchronous prefix.
	select {
	case ev := <-eventCh:
		t.Fatalf("no event should be published for a synchronously-rejected alias request, got %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}

	// No row was ever persisted, so nothing can subsequently be observed as
	// a Failed card either.
	existing, err := storage.ListInstanceData()
	require.NoError(t, err)
	for _, data := range existing {
		assert.NotEqual(t, title, data.Title, "an invalid alias must never leave behind a persisted instance, Failed or otherwise")
	}
}
