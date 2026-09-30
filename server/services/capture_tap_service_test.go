package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

type tapFixture struct {
	client sessionv1connect.SessionServiceClient
	reg    *streamhub.TapRegistry
	dir    string
}

// newTapFixture serves a SessionService over HTTP. remoteAddr, when set,
// replaces the request's RemoteAddr to simulate a peer other than loopback.
func newTapFixture(t *testing.T, remoteAddr string) tapFixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tap")
	reg := streamhub.NewTapRegistry(streamhub.TapRegistryOptions{
		DirFn:     func() (string, error) { return dir, nil },
		AfterFunc: func(time.Duration, func()) {},
	})
	svc := &SessionService{tapRegistry: reg}
	path, h := sessionv1connect.NewSessionServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if remoteAddr != "" {
			r.RemoteAddr = remoteAddr
		}
		h.ServeHTTP(w, r)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return tapFixture{client: sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL), reg: reg, dir: dir}
}

func TestSetCaptureTap_should_EnableAndReportState_When_CalledFromLoopback(t *testing.T) {
	f := newTapFixture(t, "")
	resp, err := f.client.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{
		Enabled: true, SessionIds: []string{"s1"}, TtlSeconds: 60,
	}))
	if err != nil {
		t.Fatal(err)
	}
	st := resp.Msg.GetState()
	if !st.GetEnabled() || st.GetScope() != "sessions" || len(st.GetSessionIds()) != 1 || st.GetDir() != f.dir || st.GetTtlExpiresAt() == nil {
		t.Fatalf("state = %v", st)
	}

	f.reg.Handle("s1").Record(streamhub.TapOutput, "", []byte("x"))
	got, err := f.client.GetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.GetCaptureTapRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	sess := got.Msg.GetState().GetSessions()
	if len(sess) != 1 || sess[0].GetName() != "s1" || sess[0].GetBytesWritten() == 0 || sess[0].GetPath() != filepath.Join(f.dir, "s1.jsonl") {
		t.Fatalf("sessions = %v", sess)
	}

	off, err := f.client.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{}))
	if err != nil || off.Msg.GetState().GetEnabled() || off.Msg.GetState().GetScope() != "off" {
		t.Fatalf("disable: %v, %v", off, err)
	}
}

func TestSetCaptureTap_should_ClampTTL_When_AboveMax(t *testing.T) {
	f := newTapFixture(t, "")
	before := time.Now()
	resp, err := f.client.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{
		Enabled: true, TtlSeconds: 1_000_000,
	}))
	if err != nil {
		t.Fatal(err)
	}
	exp := resp.Msg.GetState().GetTtlExpiresAt().AsTime()
	if exp.After(before.Add(streamhub.TapMaxTTL+time.Minute)) || exp.Before(before.Add(streamhub.TapMaxTTL-time.Minute)) {
		t.Fatalf("expires %v, want about now+%v", exp, streamhub.TapMaxTTL)
	}
}

func TestSetCaptureTap_should_RejectBadInput(t *testing.T) {
	f := newTapFixture(t, "")
	tests := []struct {
		name string
		req  *sessionv1.SetCaptureTapRequest
		want connect.Code
	}{
		{"negative ttl", &sessionv1.SetCaptureTapRequest{Enabled: true, TtlSeconds: -1}, connect.CodeInvalidArgument},
		{"empty session name", &sessionv1.SetCaptureTapRequest{Enabled: true, SessionIds: []string{""}}, connect.CodeInvalidArgument},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.client.SetCaptureTap(context.Background(), connect.NewRequest(tc.req))
			if connect.CodeOf(err) != tc.want {
				t.Fatalf("code = %v (%v), want %v", connect.CodeOf(err), err, tc.want)
			}
		})
	}
	if f.reg.Status().Enabled {
		t.Fatal("a rejected request must not change state")
	}
}

func TestCaptureTap_should_RejectWithPermissionDenied_When_PeerIsNotLoopback(t *testing.T) {
	f := newTapFixture(t, "203.0.113.5:4242")
	_, err := f.client.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{Enabled: true}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("Set code = %v (%v), want PermissionDenied", connect.CodeOf(err), err)
	}
	_, err = f.client.GetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.GetCaptureTapRequest{}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("Get code = %v (%v), want PermissionDenied", connect.CodeOf(err), err)
	}
	if f.reg.Status().Enabled {
		t.Fatal("a rejected request must not enable the tap")
	}
}

func TestCaptureTap_should_RejectWithPermissionDenied_When_RequestIsProxied(t *testing.T) {
	f := newTapFixture(t, "127.0.0.1:4242") // a local proxy connects from loopback
	for _, header := range []string{"X-Forwarded-For", "Forwarded", "X-Real-Ip", "Via", "X-Forwarded-Host"} {
		t.Run(header, func(t *testing.T) {
			req := connect.NewRequest(&sessionv1.SetCaptureTapRequest{Enabled: true})
			req.Header().Set(header, "203.0.113.5")
			_, err := f.client.SetCaptureTap(context.Background(), req)
			if connect.CodeOf(err) != connect.CodePermissionDenied {
				t.Fatalf("code = %v (%v), want PermissionDenied", connect.CodeOf(err), err)
			}
		})
	}
	if f.reg.Status().Enabled {
		t.Fatal("a proxied request must not enable the tap")
	}
}

func TestRequireLoopbackCaller(t *testing.T) {
	tests := []struct {
		name    string
		peer    string
		origin  string
		wantErr bool
	}{
		{"ipv4 loopback", "127.0.0.1:1", "", false},
		{"ipv6 loopback", "[::1]:1", "", false},
		{"ipv4-mapped loopback", "[::ffff:127.0.0.1]:1", "", false},
		{"LAN address", "192.168.1.5:1", "", true},
		{"unspecified", "0.0.0.0:1", "", true},
		{"unix socket", "@", "", true},
		{"empty", "", "", true},
		{"loopback origin", "127.0.0.1:1", "http://localhost:8543", false},
		{"foreign origin", "127.0.0.1:1", "https://evil.example", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.origin != "" {
				h.Set("Origin", tc.origin)
			}
			err := requireLoopbackCaller(tc.peer, h)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && connect.CodeOf(err) != connect.CodePermissionDenied {
				t.Fatalf("code = %v", connect.CodeOf(err))
			}
		})
	}
}

func TestSetCaptureTap_should_ReturnFailedPrecondition_When_DisablingNamedSessionUnderAll(t *testing.T) {
	f := newTapFixture(t, "")
	if _, err := f.client.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{Enabled: true})); err != nil {
		t.Fatal(err)
	}
	_, err := f.client.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{SessionIds: []string{"a"}}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("code = %v (%v), want FailedPrecondition", connect.CodeOf(err), err)
	}
}
