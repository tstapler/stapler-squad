package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services"
)

// newChainTestServer builds a Server with only a mux, so the per-listener
// chains can be exercised without binding a port.
func newChainTestServer(t *testing.T, addr string) (*Server, *int) {
	t.Helper()
	srv, _ := newServerBase(addr)
	t.Cleanup(srv.connCtxCancel)
	reached := new(int)
	srv.mux.HandleFunc(probeProcedurePath, func(w http.ResponseWriter, _ *http.Request) {
		*reached++
		w.WriteHeader(http.StatusOK)
	})
	return srv, reached
}

func postProbe(h http.Handler, host, origin string) int {
	r := httptest.NewRequest(http.MethodPost, probeProcedurePath, strings.NewReader("{}"))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestStartChain_should_403WrongHostAnd_Pass_LocalhostHost_When_AuthMiddlewareNil(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	chain := srv.localChain()

	assert.Equal(t, http.StatusForbidden, postProbe(chain, "evil.example:8543", ""))
	assert.Zero(t, *reached)
	assert.Equal(t, http.StatusOK, postProbe(chain, "localhost:8543", ""))
	assert.Equal(t, 1, *reached)
}

func TestStartChain_should_403_When_ListenerBoundToAllInterfaces(t *testing.T) {
	srv, reached := newChainTestServer(t, "0.0.0.0:8543")
	assert.Equal(t, http.StatusForbidden, postProbe(srv.localChain(), "localhost:8543", ""))
	assert.Zero(t, *reached)
}

func TestStartChain_should_HonorOriginsSetAfterConstruction_When_SetOriginsCalledLater(t *testing.T) {
	srv, _ := newChainTestServer(t, "localhost:8543")
	chain := srv.localChain()
	assert.Equal(t, http.StatusForbidden, postProbe(chain, "localhost:8543", "https://ui.example"))
	srv.SetOrigins([]string{"https://ui.example"})
	assert.Equal(t, http.StatusOK, postProbe(chain, "localhost:8543", "https://ui.example"))
}

// The local listener is loopback-only, so a detected LAN hostname is never a
// legitimate Host on it: it could only arrive via a same-box proxy or rebinding.
func TestStartChain_should_Reject_PublishedLANHostname_When_SetHostnamesCalled(t *testing.T) {
	srv, _ := newChainTestServer(t, "localhost:8543")
	srv.SetHostnames([]string{"onyx.lan"})
	assert.Equal(t, http.StatusForbidden, postProbe(srv.localChain(), "onyx.lan:8543", ""))
}

// ProbeGuard's POST/loopback-bound checks are replaced by auth, but the Host
// guard stays: auth alone doesn't stop a same-box proxy or rebinding Host.
func TestStartChain_should_KeepHostGuard_When_AuthMiddlewareSet(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	srv.authMiddleware = func(next http.Handler) http.Handler { return next }
	assert.Equal(t, http.StatusForbidden, postProbe(srv.localChain(), "evil.example:8543", ""))
	assert.Zero(t, *reached)
	assert.Equal(t, http.StatusOK, postProbe(srv.localChain(), "localhost:8543", ""))
	assert.Equal(t, 1, *reached)
}

func TestRemoteChain_should_NotContainGuard_When_HostIsOnyxAt8444WithAuth(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	admit := func(next http.Handler) http.Handler { return next }
	assert.Equal(t, http.StatusOK, postProbe(srv.remoteChain(admit, false), "onyx.staplerhome.internal:8444", ""))
	assert.Equal(t, 1, *reached)
}

func postNudge(h http.Handler, method, host, origin string) int {
	r := httptest.NewRequest(method, nudgeProcedurePath, strings.NewReader("{}"))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestLocalChainAndRemoteChain_should_AlwaysWrapNudgePathWithAuthOrGuard_When_ChainsBuilt(t *testing.T) {
	newServer := func() (*Server, *int) {
		srv, _ := newChainTestServer(t, "localhost:8543")
		reached := new(int)
		srv.mux.HandleFunc(nudgeProcedurePath, func(w http.ResponseWriter, _ *http.Request) {
			*reached++
			w.WriteHeader(http.StatusOK)
		})
		return srv, reached
	}
	denyAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	}

	t.Run("local without auth uses the guard", func(t *testing.T) {
		srv, reached := newServer()
		chain := srv.localChain()
		assert.Equal(t, http.StatusForbidden, postNudge(chain, http.MethodPost, "rebind.example:8543", ""))
		assert.Equal(t, http.StatusForbidden, postNudge(chain, http.MethodPost, "localhost:8543", "https://evil.example"))
		assert.Equal(t, http.StatusMethodNotAllowed, postNudge(chain, http.MethodGet, "localhost:8543", ""))
		assert.Zero(t, *reached)
		assert.Equal(t, http.StatusOK, postNudge(chain, http.MethodPost, "localhost:8543", ""))
	})
	t.Run("local with auth uses auth", func(t *testing.T) {
		srv, reached := newServer()
		srv.authMiddleware = denyAll
		assert.Equal(t, http.StatusUnauthorized, postNudge(srv.localChain(), http.MethodPost, "localhost:8543", ""))
		assert.Zero(t, *reached)
	})
	t.Run("remote wraps with auth", func(t *testing.T) {
		srv, reached := newServer()
		assert.Equal(t, http.StatusUnauthorized, postNudge(srv.remoteChain(denyAll, true), http.MethodPost, "onyx.lan:8444", ""))
		assert.Zero(t, *reached)
	})
}

// Pins the existing posture: the remote chain's only boundary is the auth
// middleware passed to StartRemote. main.go always supplies one; with nil the
// nudge path is reachable, so callers must never pass nil outside tests.
func TestRemoteChain_should_LeaveNudgeReachable_When_AuthMiddlewareNil(t *testing.T) {
	srv, _ := newChainTestServer(t, "localhost:8543")
	reached := 0
	srv.mux.HandleFunc(nudgeProcedurePath, func(w http.ResponseWriter, _ *http.Request) {
		reached++
		w.WriteHeader(http.StatusOK)
	})
	assert.Equal(t, http.StatusOK, postNudge(srv.remoteChain(nil, false), http.MethodPost, "onyx.lan:8444", ""))
	assert.Equal(t, 1, reached)
}

type typedNilValidator struct{}

func (*typedNilValidator) ValidateAuthSession(string) bool { return false }

type okValidator struct{}

func (okValidator) ValidateAuthSession(string) bool { return true }

// recordProbe serves one request through h and returns the RequestRecord the
// handler behind the chain observed.
func recordProbe(t *testing.T, build func(*Server) http.Handler, host string, hdr map[string]string) services.RequestRecord {
	t.Helper()
	srv, _ := newChainTestServer(t, "localhost:8543")
	var got services.RequestRecord
	var found bool
	srv.mux.HandleFunc("/record-probe", func(_ http.ResponseWriter, r *http.Request) {
		got, found = services.RequestRecordFrom(r.Context())
	})
	r := httptest.NewRequest(http.MethodPost, "/record-probe", strings.NewReader("{}"))
	r.Host = host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	build(srv).ServeHTTP(httptest.NewRecorder(), r)
	require.True(t, found, "no request record reached the handler")
	return got
}

func TestAuthMode_ShouldBeRequiredOnlyWhenARealValidatorIsWired_AndAnIdentityOrNilWrapperOrTypedNilKeepsTheHostTest(t *testing.T) {
	identity := func(next http.Handler) http.Handler { return next }
	var typedNil *typedNilValidator
	cases := []struct {
		name string
		mw   func(http.Handler) http.Handler
		req  bool
		want string
	}{
		{"nil wrapper", nil, false, services.AuthModeNone},
		{"identity wrapper", identity, false, services.AuthModeNone},
		{"Auth(nil)", middleware.Auth(nil), middleware.AuthRequires(nil), services.AuthModeNone},
		{"Auth(typed nil)", middleware.Auth(typedNil), middleware.AuthRequires(typedNil), services.AuthModeNone},
		{"Auth(validator)", middleware.Auth(okValidator{}), middleware.AuthRequires(okValidator{}), services.AuthModeRequired},
	}
	for _, c := range cases {
		t.Run(c.name+" local", func(t *testing.T) {
			if c.name == "Auth(typed nil)" {
				t.Skip("Auth wraps a typed nil and 401s before any handler; auth_mode is covered by the remote subtest")
			}
			rec := recordProbe(t, func(srv *Server) http.Handler {
				srv.SetupAuth(c.mw, c.req)
				return srv.localChain()
			}, "localhost:8543", map[string]string{"Authorization": "Bearer x"})
			assert.Equal(t, c.want, rec.AuthMode)
		})
		t.Run(c.name+" remote", func(t *testing.T) {
			rec := recordProbe(t, func(srv *Server) http.Handler { return srv.remoteChain(identity, c.req) }, "onyx.lan:8444", nil)
			assert.Equal(t, c.want, rec.AuthMode)
		})
	}

	// An identity wrapper must keep the Host test on the local chain.
	srv, reached := newChainTestServer(t, "localhost:8543")
	srv.SetupAuth(identity, false)
	assert.Equal(t, http.StatusForbidden, postProbe(srv.localChain(), "evil.example:8543", ""))
	assert.Zero(t, *reached)
}

func TestRequestRecord_ShouldCarryRawListenerAuthModeHostOriginAndProxiedOnBothChainsWithoutAVerdict_WhenOnlyPr1fAIsMerged(t *testing.T) {
	local := recordProbe(t, (*Server).localChain, "localhost:8543", nil)
	assert.Equal(t, services.RequestRecord{Listener: services.ListenerLocal, AuthMode: services.AuthModeNone, Host: "localhost:8543"}, local)

	remote := recordProbe(t, func(srv *Server) http.Handler { return srv.remoteChain(nil, true) }, "onyx.lan:8444", map[string]string{"X-Forwarded-For": "10.0.0.9", "Origin": "https://onyx.lan:8444"})
	assert.Equal(t, services.RequestRecord{Listener: services.ListenerRemote, AuthMode: services.AuthModeRequired, Host: "onyx.lan:8444", Origin: "https://onyx.lan:8444", Proxied: true}, remote)

	// Raw values only: the record adds no refusal, even for a foreign Host.
	rebinding := recordProbe(t, func(srv *Server) http.Handler { return srv.remoteChain(nil, false) }, "evil.example", nil)
	assert.Equal(t, "evil.example", rebinding.Host)
}

// rebindingSet is the production set plus a stand-in for Reply, which
// registers in a later PR; Prune is already a member of guardedProcedures.
func rebindingSet() map[string]middleware.GuardProfile {
	set := map[string]middleware.GuardProfile{}
	for p, prof := range guardedProcedures {
		set[p] = prof
	}
	return set
}

func newRebindingTestServer(t *testing.T, addr string) (*Server, *int) {
	t.Helper()
	srv, _ := newChainTestServer(t, addr)
	reached := new(int)
	srv.mux.HandleFunc(pruneProcedurePath, func(w http.ResponseWriter, _ *http.Request) {
		*reached++
		w.WriteHeader(http.StatusOK)
	})
	return srv, reached
}

func postPath(h http.Handler, path, host, origin string) int {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

// T-RP-39: the rebinding profile on every supported bind, and ProbeProgram
// behavior unchanged beside it. The local chain's HostGuard (no AllowedHosts)
// refuses every non-loopback Host before the procedure guard, so a verified LAN
// hostname never reaches it on :8543; the profile's LAN-hostname allowance is
// covered at the middleware level (callerguard_test.go).
func TestLocalWriteGuard_ShouldRejectRebindingHostForeignOriginAndNonPostForReply_AndKeepProbeProgramBehaviorUnchanged(t *testing.T) {
	for _, addr := range []string{"localhost:8543", "127.0.0.1:8543", "0.0.0.0:8543", ":8543"} {
		t.Run(addr, func(t *testing.T) {
			srv, reached := newRebindingTestServer(t, addr)
			srv.ReplaceVerifiedHostnames([]string{"onyx.lan"})
			chain := srv.localChainWith(rebindingSet())

			assert.Equal(t, http.StatusForbidden, postPath(chain, pruneProcedurePath, "evil.example:8543", ""))
			assert.Equal(t, http.StatusForbidden, postPath(chain, pruneProcedurePath, "localhost:8543", "https://evil.example"))
			assert.Equal(t, http.StatusForbidden, postPath(chain, pruneProcedurePath, "onyx.lan:8543", ""), "HostGuard refuses LAN names on :8543")
			assert.Zero(t, *reached)

			r := httptest.NewRequest(http.MethodGet, pruneProcedurePath, nil)
			r.Host = "localhost:8543"
			w := httptest.NewRecorder()
			chain.ServeHTTP(w, r)
			assert.Equal(t, http.StatusMethodNotAllowed, w.Code, "non-POST to a guarded procedure")

			// Loopback passes the rebinding profile on every bind, wildcard included.
			assert.Equal(t, http.StatusOK, postPath(chain, pruneProcedurePath, "localhost:8543", ""))
			assert.Equal(t, 1, *reached)

			// ProbeProgram keeps its loopback-bound condition: allowed only on a loopback bind.
			wantProbe := http.StatusForbidden
			if middleware.ListenAddrIsLoopback(addr) {
				wantProbe = http.StatusOK
			}
			assert.Equal(t, wantProbe, postProbe(chain, "localhost:8543", ""))
		})
	}
}

// A name that is only in GetHostnames() (detected, unverified) is refused.
func TestLocalWriteGuard_ShouldRefuseAHostnameOnlyInDetectedSet_WhenVerifiedSetLacksIt(t *testing.T) {
	srv, reached := newRebindingTestServer(t, "0.0.0.0:8543")
	srv.SetHostnames([]string{"attacker.example"})
	assert.Equal(t, http.StatusForbidden, postPath(srv.localChainWith(rebindingSet()), pruneProcedurePath, "attacker.example:8543", ""))
	assert.Zero(t, *reached)
}

// T-RP-57: the set holds only the registered procedures and never
// ClearNotificationHistory.
func TestLocalWriteGuard_ShouldKeepProbeProgramByteForByteAndListOnlyRegisteredProcedures_AndNeverGuardClearNotificationHistory(t *testing.T) {
	assert.Equal(t, map[string]middleware.GuardProfile{
		probeProcedurePath: middleware.ProfileProbe,
		nudgeProcedurePath: middleware.ProfileProbe,
		pruneProcedurePath: middleware.ProfileRebinding,
	}, guardedProcedures)
	_, hasClear := guardedProcedures["/api"+sessionv1connect.SessionServiceClearNotificationHistoryProcedure]
	assert.False(t, hasClear)
}

// T-RP-67: path variants of a guarded procedure are not served without the guard.
func TestLocalWriteGuard_ShouldNotServeTrailingSlashDoubleSlashDotSegmentOrPercentEncodedProcedurePathsWithoutTheGuard(t *testing.T) {
	srv, reached := newRebindingTestServer(t, "localhost:8543")
	chain := srv.localChainWith(rebindingSet())
	bare := strings.TrimPrefix(pruneProcedurePath, "/api")
	variants := map[string]string{
		"unprefixed":      bare,
		"trailing slash":  pruneProcedurePath + "/",
		"double slash":    "/api/" + bare,
		"dot segment":     "/api/./" + strings.TrimPrefix(bare, "/"),
		"percent encoded": "/api%2F" + strings.TrimPrefix(bare, "/"),
		"encoded slash":   strings.Replace(pruneProcedurePath, "/PruneHidden", "%2FPruneHidden", 1),
	}
	for name, path := range variants {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
			r.URL.Path = path
			r.URL.RawPath = ""
			r.Host = "evil.example:8543"
			w := httptest.NewRecorder()
			chain.ServeHTTP(w, r)
			assert.NotEqual(t, http.StatusOK, w.Code)
		})
	}
	assert.Zero(t, *reached, "no variant reaches the handler from a rebinding Host")
}

// T-RP-68: with an authMiddleware the procedure guard is not installed and
// auth is the boundary; the Host guard stays.
func TestLocalWriteGuard_ShouldNotBeInstalledWhenAuthMiddlewareIsSetAndTheVerdictShouldFollowAuthMode_WhenLocalChainHasAuth(t *testing.T) {
	srv, reached := newRebindingTestServer(t, "0.0.0.0:8543")
	srv.SetupAuth(func(next http.Handler) http.Handler { return next }, true)
	chain := srv.localChainWith(rebindingSet())

	// A GET would be 405 from the procedure guard; it reaches the handler, so no guard is installed.
	r := httptest.NewRequest(http.MethodGet, pruneProcedurePath, nil)
	r.Host = "localhost:8543"
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, *reached)
	assert.Equal(t, http.StatusForbidden, postPath(chain, pruneProcedurePath, "evil.example:8543", ""), "Host guard stays")

	rec := recordProbe(t, func(s *Server) http.Handler {
		s.SetupAuth(func(next http.Handler) http.Handler { return next }, true)
		return s.localChain()
	}, "localhost:8543", nil)
	assert.Equal(t, services.AuthModeRequired, rec.AuthMode)
}

func TestReplaceVerifiedHostnames_ShouldNormalizeDropLiteralsAndLocalhostAndReplaceWholesale(t *testing.T) {
	srv, _ := newChainTestServer(t, "localhost:8543")
	assert.Nil(t, srv.GetVerifiedHostnames())

	srv.ReplaceVerifiedHostnames([]string{"B.lan.", "a.lan", "A.LAN", "localhost", "10.0.0.5", "::1", ""})
	assert.Equal(t, []string{"a.lan", "b.lan"}, srv.GetVerifiedHostnames())

	srv.ReplaceVerifiedHostnames([]string{"c.lan"})
	assert.Equal(t, []string{"c.lan"}, srv.GetVerifiedHostnames(), "not add-only")
	assert.Empty(t, srv.GetHostnames(), "the detected set is a separate store")
}

// T-PR-16 (guard half): the production set guards Prune whole-procedure, so a
// rebinding Host or foreign Origin is refused before the handler, dry run or
// apply alike; a loopback caller on a wildcard bind passes (no LoopbackBound).
func TestLocalChain_ShouldRefuseRebindingHostAndForeignOriginOnPrune_WithTheProductionGuardSet(t *testing.T) {
	for _, addr := range []string{"localhost:8543", "0.0.0.0:8543"} {
		srv, reached := newRebindingTestServer(t, addr)
		chain := srv.localChain()
		assert.Equal(t, http.StatusForbidden, postPath(chain, pruneProcedurePath, "evil.example:8543", ""), addr)
		assert.Equal(t, http.StatusForbidden, postPath(chain, pruneProcedurePath, "localhost:8543", "https://evil.example"), addr)
		assert.Zero(t, *reached, addr)
		assert.Equal(t, http.StatusOK, postPath(chain, pruneProcedurePath, "localhost:8543", ""), addr)
		assert.Equal(t, 1, *reached, addr)
	}
}
