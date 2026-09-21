package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

const testProbePath = "/api/session.v1.SessionService/ProbeProgram"

type guardHarness struct {
	origins  []string
	hosts    []string
	loopback bool
	reached  int
}

func (h *guardHarness) handler() http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h.reached++
		w.WriteHeader(http.StatusOK)
	})
	return ProbeGuard(testProbePath, ProbeGuardConfig{
		LoopbackBound:  func() bool { return h.loopback },
		AllowedOrigins: func() []string { return h.origins },
		AllowedHosts:   func() []string { return h.hosts },
	})(next)
}

func (h *guardHarness) do(method, path, host, origin string) int {
	r := httptest.NewRequest(method, path, strings.NewReader("{}"))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.handler().ServeHTTP(w, r)
	return w.Code
}

func TestProbeGuard_should_Return405_When_MethodGet(t *testing.T) {
	h := &guardHarness{loopback: true}
	assert.Equal(t, http.StatusMethodNotAllowed, h.do(http.MethodGet, testProbePath, "localhost:8543", ""))
	assert.Zero(t, h.reached)
}

func TestProbeGuard_should_Return403_When_HostOrOriginNotLoopbackOrAllowed(t *testing.T) {
	tests := []struct{ name, host, origin string }{
		{"evil host", "evil.example:8543", ""},
		{"evil origin", "localhost:8543", "https://evil.example"},
		{"null origin", "localhost:8543", "null"},
		{"rebinding host with loopback origin", "evil.example", "http://localhost:8543"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &guardHarness{loopback: true}
			assert.Equal(t, http.StatusForbidden, h.do(http.MethodPost, testProbePath, tt.host, tt.origin))
			assert.Zero(t, h.reached)
		})
	}
}

func TestProbeGuard_should_Pass_When_LoopbackHostV4V6AndAllowedOrOmittedOrigin(t *testing.T) {
	for _, host := range []string{"localhost:8543", "127.0.0.1:8543", "[::1]:8543", "localhost"} {
		for _, origin := range []string{"", "http://localhost:8543", "http://[::1]:8543"} {
			h := &guardHarness{loopback: true}
			assert.Equal(t, http.StatusOK, h.do(http.MethodPost, testProbePath, host, origin), "host=%q origin=%q", host, origin)
			assert.Equal(t, 1, h.reached)
		}
	}
}

func TestProbeGuard_should_HonorLateSetOriginsAndHostnames_When_ReadLazily(t *testing.T) {
	h := &guardHarness{loopback: true}
	handler := h.handler() // built before the config changes
	do := func(host, origin string) int {
		r := httptest.NewRequest(http.MethodPost, testProbePath, nil)
		r.Host = host
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	assert.Equal(t, http.StatusForbidden, do("localhost:8543", "https://ui.example"))
	h.origins = []string{"https://ui.example"}
	assert.Equal(t, http.StatusOK, do("localhost:8543", "https://ui.example"))
	assert.Equal(t, http.StatusForbidden, do("onyx.lan:8543", "http://localhost:8543"))
	h.hosts = []string{"onyx.lan"}
	assert.Equal(t, http.StatusOK, do("onyx.lan:8543", "http://localhost:8543"))
}

func TestProbeGuard_should_Pass_When_HostPublishedViaSetHostnames(t *testing.T) {
	h := &guardHarness{loopback: true, hosts: []string{"onyx.staplerhome.internal"}}
	assert.Equal(t, http.StatusOK, h.do(http.MethodPost, testProbePath, "ONYX.staplerhome.internal:8543", ""))
}

func TestProbeGuard_should_Return403ForEveryRequest_When_NonLoopbackBind(t *testing.T) {
	h := &guardHarness{loopback: false}
	assert.Equal(t, http.StatusForbidden, h.do(http.MethodPost, testProbePath, "localhost:8543", ""))
	assert.Zero(t, h.reached)
}

func TestProbeGuard_should_LeaveOtherProceduresUntouched_When_DifferentPath(t *testing.T) {
	h := &guardHarness{loopback: false}
	other := "/api/session.v1.SessionService/ListSessions"
	assert.Equal(t, http.StatusOK, h.do(http.MethodGet, other, "evil.example", "https://evil.example"))
	assert.Equal(t, 1, h.reached)
}

func TestListenAddrIsLoopback_should_ClassifyBindAddresses_When_Given(t *testing.T) {
	tests := map[string]bool{
		"localhost:8543": true, "127.0.0.1:8543": true, "[::1]:8543": true, "127.0.0.1:0": true,
		"0.0.0.0:8543": false, ":8543": false, "192.168.1.5:8543": false, "[::]:8543": false, "": false,
	}
	for addr, want := range tests {
		assert.Equal(t, want, ListenAddrIsLoopback(addr), addr)
	}
}

type countingSessionHandler struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	calls int
}

func (c *countingSessionHandler) ProbeProgram(context.Context, *connect.Request[sessionv1.ProbeProgramRequest]) (*connect.Response[sessionv1.ProbeProgramResponse], error) {
	c.calls++
	return connect.NewResponse(&sessionv1.ProbeProgramResponse{Found: true}), nil
}

func TestProbeGuard_should_NotReachService_When_WrongHostPostAgainstRealConnectHandler(t *testing.T) {
	svc := &countingSessionHandler{}
	path, connectHandler := sessionv1connect.NewSessionServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle("/api"+path, http.StripPrefix("/api", connectHandler))
	guarded := ProbeGuard("/api"+sessionv1connect.SessionServiceProbeProgramProcedure, ProbeGuardConfig{
		LoopbackBound: func() bool { return true },
	})(mux)

	post := func(host string) int {
		r := httptest.NewRequest(http.MethodPost, "/api"+sessionv1connect.SessionServiceProbeProgramProcedure, strings.NewReader(`{"command":"claude"}`))
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		guarded.ServeHTTP(w, r)
		return w.Code
	}
	assert.Equal(t, http.StatusForbidden, post("evil.example:8543"))
	assert.Zero(t, svc.calls)
	assert.Equal(t, http.StatusOK, post("localhost:8543"))
	assert.Equal(t, 1, svc.calls)
}
