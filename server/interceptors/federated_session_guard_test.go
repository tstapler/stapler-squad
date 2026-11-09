package interceptors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
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
		return connect.NewResponse(&sessionv1.GetSessionResponse{}), nil
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
}

// A real handler chain: GetSession for a remote ID reaches the handler, while
// DeleteSession for the same ID is rejected before it.
func TestFederatedSessionGuard_ReadsPassWhenHandlerServesThem(t *testing.T) {
	t.Parallel()
	guard := connect.WithInterceptors(NewFederatedSessionGuard())
	mux := http.NewServeMux()
	reached := false
	mux.Handle(sessionv1connect.SessionServiceGetSessionProcedure, connect.NewUnaryHandler(
		sessionv1connect.SessionServiceGetSessionProcedure,
		func(context.Context, *connect.Request[sessionv1.GetSessionRequest]) (*connect.Response[sessionv1.GetSessionResponse], error) {
			reached = true
			return connect.NewResponse(&sessionv1.GetSessionResponse{}), nil
		}, guard))
	mux.Handle(sessionv1connect.SessionServiceDeleteSessionProcedure, connect.NewUnaryHandler(
		sessionv1connect.SessionServiceDeleteSessionProcedure,
		func(context.Context, *connect.Request[sessionv1.DeleteSessionRequest]) (*connect.Response[sessionv1.DeleteSessionResponse], error) {
			t.Error("delete of a remote id must not reach the handler")
			return connect.NewResponse(&sessionv1.DeleteSessionResponse{}), nil
		}, guard))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	if _, err := client.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: testRemoteHost + "/abc"})); err != nil || !reached {
		t.Fatalf("GetSession for remote id: err=%v reached=%v", err, reached)
	}
	_, err := client.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: testRemoteHost + "/abc"}))
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("DeleteSession for remote id: err=%v", err)
	}
}

func TestFederatedSessionGuard_PassesLocalIDs(t *testing.T) {
	for _, id := range []string{"abc", "team/feature", "host_nope/abc"} {
		if _, rejected := guardCode(t, &sessionv1.DeleteSessionRequest{Id: id}); rejected {
			t.Errorf("local id %q rejected", id)
		}
	}
}
