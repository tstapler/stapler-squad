package interceptors

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const testRemoteHost = "host_01J00000000000000000000000"

func guardCode(t *testing.T, msg any) (connect.Code, bool) {
	t.Helper()
	var req connect.AnyRequest
	switch m := msg.(type) {
	case *sessionv1.GetSessionRequest:
		req = connect.NewRequest(m)
	case *sessionv1.DeleteSessionRequest:
		req = connect.NewRequest(m)
	}
	_, err := NewFederatedSessionGuard()(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})(context.Background(), req)
	if err == nil {
		return 0, false
	}
	return connect.CodeOf(err), true
}

func TestFederatedSessionGuard_RejectsRemoteIDs(t *testing.T) {
	if code, rejected := guardCode(t, &sessionv1.DeleteSessionRequest{Id: testRemoteHost + "/abc"}); !rejected || code != connect.CodeUnimplemented {
		t.Fatalf("remote delete: rejected=%v code=%v", rejected, code)
	}
	if _, rejected := guardCode(t, &sessionv1.GetSessionRequest{Id: testRemoteHost + "/abc"}); !rejected {
		t.Fatal("remote get should be rejected")
	}
}

func TestFederatedSessionGuard_PassesLocalIDs(t *testing.T) {
	for _, id := range []string{"abc", "team/feature", "host_nope/abc"} {
		if _, rejected := guardCode(t, &sessionv1.DeleteSessionRequest{Id: id}); rejected {
			t.Errorf("local id %q rejected", id)
		}
	}
}
