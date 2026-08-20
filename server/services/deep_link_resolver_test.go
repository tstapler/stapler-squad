package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		Kind string                    `json:"kind"`
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

	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer live.Close()
	// Advertise as "localhost:<port>" rather than the raw 127.0.0.1 address
	// so the hostname component matches the ssq:// URL's "localhost" host
	// (session.HostRegistry.LookupByHostname matches on hostname, not IP),
	// while the address remains real and connectable for the liveness check.
	addr := "localhost:" + strings.TrimPrefix(live.URL, "http://127.0.0.1:")

	seedRegistryEntry(t, stateDir, addr)

	resolver := NewDeepLinkResolver(storage, NewRegistryHostResolver(stateDir, session.DefaultHostRegistryTTL))
	resolver.ownHostname = fakeOwnHostname("myhost")

	req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://localhost/backlog/v1/bl_01J0000000000000000000", nil)
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

	// A handler that hangs until the test unblocks it, so the request
	// stalls until the resolver's own bounded liveness timeout fires --
	// not until any real network/DNS failure. block is released before
	// hang.Close() (deferred in reverse order) so Close() doesn't wait
	// forever on a handler goroutine that never returns.
	block := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer hang.Close()
	defer close(block)
	// See the handoff-address test above: advertise "localhost:<port>" so the
	// hostname matches the ssq:// URL's "localhost" host while the address
	// itself stays real (and hangs, for this test).
	addr := "localhost:" + strings.TrimPrefix(hang.URL, "http://127.0.0.1:")

	seedRegistryEntry(t, stateDir, addr)

	resolver := NewDeepLinkResolver(storage, NewRegistryHostResolver(stateDir, session.DefaultHostRegistryTTL))
	resolver.ownHostname = fakeOwnHostname("myhost")

	req := httptest.NewRequest(http.MethodGet, "/api/deep-link/resolve?url=ssq://localhost/backlog/v1/bl_01J0000000000000000000", nil)
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
