package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// seedRegistryEntry writes a valid, signed advertisement for hostname/addr
// into a HostRegistry rooted at stateDir, so a registryHostResolver opened
// against the same stateDir observes it via LookupByHostname.
func seedRegistryEntry(t *testing.T, stateDir string, addr string) {
	t.Helper()
	identity, err := session.LoadOrCreateHostIdentity(stateDir)
	require.NoError(t, err)

	registry, err := session.NewHostRegistry(stateDir, session.DefaultHostRegistryTTL)
	require.NoError(t, err)

	record := session.BuildAdvertisement(identity, []string{addr}, time.Now())
	_, accepted, err := registry.Advertise(record)
	require.NoError(t, err)
	require.True(t, accepted)
}

// fakeOwnHostname lets tests control what isOwnHostname compares against
// without depending on the real os.Hostname() of the machine running the
// test.
func fakeOwnHostname(name string) func() (string, error) {
	return func() (string, error) { return name, nil }
}

func newDeepLinkResolverForTest(items backlogItemGetter, hostname string) *DeepLinkResolver {
	r := NewDeepLinkResolver(items, nil)
	r.ownHostname = fakeOwnHostname(hostname)
	return r
}

// stubHealthTransport lets liveness-check tests control checkLiveness's
// http.Client.Do outcome directly, without a real network dial -- needed
// because the advertised addresses these tests seed must be plausible
// (non-loopback) per session.HostRegistry's isPlausiblePeerAddress SSRF
// guard, so a real httptest.Server (always loopback-bound) can no longer
// stand in for "the address is live."
type stubHealthTransport struct {
	respond func(req *http.Request) (*http.Response, error)
}

func (s stubHealthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return s.respond(req)
}

// registryResolverWithStubTransport builds a registryHostResolver identical
// to NewRegistryHostResolver's, except its httpClient's Transport is the
// given stub rather than the real network -- same package as
// registryHostResolver, so its unexported fields are reachable directly.
func registryResolverWithStubTransport(stateDir string, transport http.RoundTripper) *registryHostResolver {
	return &registryHostResolver{
		stateDir:        stateDir,
		registryTTL:     session.DefaultHostRegistryTTL,
		httpClient:      &http.Client{Transport: transport},
		livenessTimeout: defaultLivenessTimeout,
	}
}

func TestDeepLinkResolver_should_ReturnLocalItemPayload_When_HostnameMatchesThisInstance(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "deep link happy path",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	resolver := newDeepLinkResolverForTest(storage, "myhost")

	req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://myhost/backlog/v1/"+item.ID, nil)
	rec := httptest.NewRecorder()

	resolver.HandleResolve(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Kind string                   `json:"kind"`
		Item *session.BacklogItemData `json:"item"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "local", body.Kind)
	require.NotNil(t, body.Item)
	require.Equal(t, item.ID, body.Item.ID)
}

func TestDeepLinkResolver_should_Return400WithDistinctReason_When_URLMalformedOrVersionMismatched(t *testing.T) {
	storage := createTestStorage(t)
	resolver := newDeepLinkResolverForTest(storage, "myhost")

	tests := []struct {
		name       string
		rawURL     string
		wantReason string
	}{
		{
			name:       "malformed - truncated",
			rawURL:     "ssq://",
			wantReason: "malformed",
		},
		{
			name:       "malformed - missing segments",
			rawURL:     "ssq://myhost/backlog/v1",
			wantReason: "malformed",
		},
		{
			name:       "version mismatch",
			rawURL:     "ssq://myhost/backlog/v2/bl_01J0000000000000000000",
			wantReason: "version-mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url="+tt.rawURL, nil)
			rec := httptest.NewRecorder()

			resolver.HandleResolve(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)

			var body struct {
				Kind   string `json:"kind"`
				Reason string `json:"reason"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "invalid", body.Kind)
			require.Equal(t, tt.wantReason, body.Reason)
		})
	}
}

func TestDeepLinkResolver_should_Return404WithDeletedOrArchivedReason_When_ItemNoLongerExists(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	resolver := newDeepLinkResolverForTest(storage, "myhost")

	t.Run("deleted", func(t *testing.T) {
		item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
			Title:  "item to delete",
			Status: string(session.BacklogStatusIdea),
		})
		require.NoError(t, err)
		require.NoError(t, storage.DeleteBacklogItem(ctx, item.ID))

		req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://myhost/backlog/v1/"+item.ID, nil)
		rec := httptest.NewRecorder()
		resolver.HandleResolve(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)

		var body struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, "not-found", body.Kind)
		require.Equal(t, "deleted", body.Reason)
	})

	t.Run("archived", func(t *testing.T) {
		item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
			Title:  "item to archive",
			Status: string(session.BacklogStatusIdea),
		})
		require.NoError(t, err)
		_, err = storage.ArchiveBacklogItem(ctx, item.ID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://myhost/backlog/v1/"+item.ID, nil)
		rec := httptest.NewRecorder()
		resolver.HandleResolve(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)

		var body struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, "not-found", body.Kind)
		require.Equal(t, "archived", body.Reason)
	})
}

func TestDeepLinkResolver_should_ReturnHandoffAddress_When_HostnameKnownAndLivenessCheckPasses(t *testing.T) {
	storage := createTestStorage(t)
	stateDir := t.TempDir()

	// A plausible (non-loopback) advertised address -- session.HostRegistry
	// rejects loopback/link-local addresses at Advertise-time as an SSRF
	// guard, so this can no longer be a real httptest.Server's 127.0.0.1
	// address. The liveness check itself is stubbed below instead of hitting
	// the network.
	addr := "remotehost.internal:9443"

	seedRegistryEntry(t, stateDir, addr)

	transport := stubHealthTransport{respond: func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	}}
	resolver := NewDeepLinkResolver(storage, registryResolverWithStubTransport(stateDir, transport))
	resolver.ownHostname = fakeOwnHostname("myhost")

	req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://remotehost.internal/backlog/v1/bl_01J0000000000000000000", nil)
	rec := httptest.NewRecorder()

	resolver.HandleResolve(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Kind              string `json:"kind"`
		AdvertisedAddress string `json:"advertisedAddress"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "handoff", body.Kind)
	require.Equal(t, addr, body.AdvertisedAddress)
}

func TestDeepLinkResolver_should_ReturnNotRegistered_When_HostnameHasNoRegistryEntry(t *testing.T) {
	storage := createTestStorage(t)
	stateDir := t.TempDir()

	resolver := NewDeepLinkResolver(storage, NewRegistryHostResolver(stateDir, session.DefaultHostRegistryTTL))
	resolver.ownHostname = fakeOwnHostname("myhost")

	req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://unknownhost/backlog/v1/bl_01J0000000000000000000", nil)
	rec := httptest.NewRecorder()

	resolver.HandleResolve(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)

	var body struct {
		Kind   string `json:"kind"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "unreachable", body.Kind)
	require.Equal(t, "not-registered", body.Reason)
}

func TestDeepLinkResolver_should_ReturnUnreachableWithinBoundedTimeout_When_LivenessCheckNeverResponds(t *testing.T) {
	storage := createTestStorage(t)
	stateDir := t.TempDir()

	// See the handoff-address test above: advertise a plausible (non-loopback)
	// address and stub the liveness transport rather than dialing a real
	// httptest.Server, which would always be loopback and rejected by
	// session.HostRegistry's SSRF guard. The stub blocks until the
	// resolver's own bounded liveness timeout cancels the request's
	// context, exactly like a real unresponsive peer would.
	addr := "remotehost.internal:9443"

	seedRegistryEntry(t, stateDir, addr)

	transport := stubHealthTransport{respond: func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}}
	resolver := NewDeepLinkResolver(storage, registryResolverWithStubTransport(stateDir, transport))
	resolver.ownHostname = fakeOwnHostname("myhost")

	req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://remotehost.internal/backlog/v1/bl_01J0000000000000000000", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	resolver.HandleResolve(rec, req)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 10*time.Second, "resolver must not block on an unresponsive peer")
	require.Equal(t, http.StatusConflict, rec.Code)

	var body struct {
		Kind   string `json:"kind"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "unreachable", body.Kind)
	require.Equal(t, "unreachable", body.Reason)
}

func TestDeepLinkResolver_should_LogResolvedOrFailedEvent_When_EveryOutcomeReasonOccurs(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	resolver := newDeepLinkResolverForTest(storage, "myhost")

	localItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "logging happy path",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	deletedItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "logging deleted",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)
	require.NoError(t, storage.DeleteBacklogItem(ctx, deletedItem.ID))

	archivedItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "logging archived",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)
	_, err = storage.ArchiveBacklogItem(ctx, archivedItem.ID)
	require.NoError(t, err)

	cases := []struct {
		name       string
		rawURL     string
		wantLevel  string
		wantEvent  string
		wantReason string
	}{
		{"local success", "ssq://myhost/backlog/v1/" + localItem.ID, "INFO", "deep_link.resolved", ""},
		{"malformed", "ssq://", "WARN", "deep_link.resolve_failed", "malformed"},
		{"version mismatch", "ssq://myhost/backlog/v2/bl_01J0000000000000000000", "WARN", "deep_link.resolve_failed", "version-mismatch"},
		{"deleted", "ssq://myhost/backlog/v1/" + deletedItem.ID, "WARN", "deep_link.resolve_failed", "deleted"},
		{"archived", "ssq://myhost/backlog/v1/" + archivedItem.ID, "WARN", "deep_link.resolve_failed", "archived"},
		{"not-registered", "ssq://otherhost/backlog/v1/bl_01J0000000000000000000", "WARN", "deep_link.resolve_failed", "not-registered"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)

			req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url="+tc.rawURL, nil)
			rec := httptest.NewRecorder()
			resolver.HandleResolve(rec, req)

			output := logs.String()
			require.Contains(t, output, tc.wantEvent)
			require.Contains(t, output, "level="+tc.wantLevel)
			if tc.wantReason != "" {
				require.Contains(t, output, "reason="+tc.wantReason)
			}
			require.NotContains(t, output, "host_advertisement.sent")
			require.NotContains(t, output, "host_advertisement.received")
		})
	}
}
