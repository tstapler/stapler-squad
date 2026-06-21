package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/server/events"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestCreateSession_PathGuard_AliasNameBypassesPathRequired verifies that when
// alias_name is set and path is empty, the "path is required" guard does NOT
// fire (i.e. the response error is not CodeInvalidArgument).
//
// The service will ultimately return CodeNotFound (alias not in config) or
// CodeInternal, but never CodeInvalidArgument for the path guard, because the
// guard condition explicitly exempts requests that carry an AliasName.
func TestCreateSession_PathGuard_AliasNameBypassesPathRequired(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

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
