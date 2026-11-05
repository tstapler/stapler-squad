package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

func TestHostAdvertisementRoute_should_RejectMalformedPayload_When_BodyIsNotValidJSON(t *testing.T) {
	identity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	registry, err := session.NewHostRegistry(t.TempDir(), session.DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}

	mux := http.NewServeMux()
	RegisterHostAdvertisementRoute(mux, identity, registry, nil, []string{"self.example:8444"})

	req := httptest.NewRequest(http.MethodPost, session.AdvertisementEndpointPath, bytes.NewReader([]byte("{not valid json")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for malformed payload", rec.Code, http.StatusBadRequest)
	}
}

func TestHostAdvertisementRoute_should_UpsertRegistryAndReturnOwnSignedRecord_When_AdvertisementValid(t *testing.T) {
	identity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	registry, err := session.NewHostRegistry(t.TempDir(), session.DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}

	mux := http.NewServeMux()
	RegisterHostAdvertisementRoute(mux, identity, registry, nil, []string{"self.example:8444"})

	peerIdentity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity(peer) error = %v, want nil", err)
	}
	record := session.BuildAdvertisement(peerIdentity, []string{"peer.example:8444"}, time.Now())
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal advertisement: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, session.AdvertisementEndpointPath, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for a valid advertisement; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if _, ok := registry.Lookup(peerIdentity.ID); !ok {
		t.Fatalf("registry.Lookup(peer) = not found, want the advertisement to have been upserted")
	}

	var reply session.AdvertisementRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if reply.HostIdentity.String() != identity.ID.String() {
		t.Fatalf("reply.HostIdentity = %s, want this instance's own identity %s", reply.HostIdentity, identity.ID)
	}
	if !reply.Verify() {
		t.Fatalf("reply.Verify() = false, want the endpoint's own reply to be a validly signed record")
	}
}
