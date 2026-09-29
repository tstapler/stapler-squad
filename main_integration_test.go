package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

func TestMainWiring_should_StartClaimIndexAndClaimGossiperAlongsideAdvertiser_When_ServerStartsWithValidHostIdentity(t *testing.T) {
	configDir := t.TempDir()
	mux := http.NewServeMux()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	if err != nil {
		t.Fatalf("NewStorageWithRepository: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	startHostGossip(ctx, mux, configDir, []string{"hosta.example"}, nil, 8444, storage)

	for _, path := range []string{session.AdvertisementEndpointPath, session.ClaimAdvertisementEndpointPath} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{not json")))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s status = %d, want 400 from the registered handler (404 means unwired)", path, rec.Code)
		}
	}

	const issue = "https://github.com/o/r/issues/1"
	if _, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:       "wiring test",
		Status:      string(session.BacklogStatusInProgress),
		ExternalURL: issue,
	}); err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	identity, err := session.LoadOrCreateHostIdentity(configDir)
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity: %v", err)
	}
	registry, err := session.NewHostRegistry(configDir, session.DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry: %v", err)
	}
	index, err := session.NewClaimIndex(configDir, registry)
	if err != nil {
		t.Fatalf("NewClaimIndex: %v", err)
	}
	claim, ok := index.CheckClaim(issue)
	if !ok {
		t.Fatalf("CheckClaim() = not found; SetClaimRecorder was not wired to storage")
	}
	if claim.ClaimingHostID.String() != identity.ID.String() {
		t.Errorf("ClaimingHostID = %s, want local %s", claim.ClaimingHostID, identity.ID)
	}
	if !strings.HasPrefix(claim.ItemDeepLink, "ssq://hosta.example/backlog/v1/") {
		t.Errorf("ItemDeepLink = %q, want ssq://hosta.example/backlog/v1/...", claim.ItemDeepLink)
	}
}
