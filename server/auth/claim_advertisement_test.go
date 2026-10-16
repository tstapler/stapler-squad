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

const claimTestURL = "https://github.com/o/r/issues/1"

func newClaimTestMux(t *testing.T) (*http.ServeMux, *session.ClaimIndex, *session.HostRegistry) {
	t.Helper()
	registry, err := session.NewHostRegistry(t.TempDir(), session.DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}
	index, err := session.NewClaimIndex(t.TempDir(), registry)
	if err != nil {
		t.Fatalf("NewClaimIndex() error = %v, want nil", err)
	}
	mux := http.NewServeMux()
	RegisterClaimAdvertisementRoute(mux, index, nil)
	return mux, index, registry
}

func enroll(t *testing.T, registry *session.HostRegistry, identity session.HostIdentity) {
	t.Helper()
	ad := session.BuildAdvertisement(identity, []string{"peer.example:8444"}, time.Now())
	if _, accepted, err := registry.Advertise(ad); err != nil || !accepted {
		t.Fatalf("Advertise(enroll) accepted=%v err=%v", accepted, err)
	}
}

func postClaim(t *testing.T, mux *http.ServeMux, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, session.ClaimAdvertisementEndpointPath, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestClaimAdvertisementEndpoint_should_AcceptValidAndReject400OnBadSignature_When_PostedRecordProcessed(t *testing.T) {
	mux, index, registry := newClaimTestMux(t)
	claimant, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v", err)
	}
	enroll(t, registry, claimant)
	valid := session.NewSignedClaimRecord(claimant, claimTestURL, "ssq://claimant/backlog/v1/bl_01J", time.Now())

	body, _ := json.Marshal(valid)
	if rec := postClaim(t, mux, body); rec.Code != http.StatusOK {
		t.Fatalf("valid POST status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if got, ok := index.CheckClaim(claimTestURL); !ok || got.ClaimingHostID.String() != claimant.ID.String() {
		t.Fatalf("CheckClaim() = (%+v, %v), want the posted record stored", got, ok)
	}

	const otherURL = "https://github.com/o/r/issues/2"
	bad := session.NewSignedClaimRecord(claimant, otherURL, "ssq://claimant/x", time.Now())
	bad.Signature[0] ^= 0xFF
	body, _ = json.Marshal(bad)
	if rec := postClaim(t, mux, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("badly-signed POST status = %d, want 400", rec.Code)
	}
	if _, ok := index.CheckClaim(otherURL); ok {
		t.Fatalf("badly-signed record was stored, want rejected")
	}
}

func TestClaimAdvertisementEndpoint_should_Reject400_When_BodyMalformedOrKeyContradictsPin(t *testing.T) {
	mux, index, registry := newClaimTestMux(t)
	if rec := postClaim(t, mux, []byte("{not json")); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed POST status = %d, want 400", rec.Code)
	}

	hostA, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v", err)
	}
	ad := session.BuildAdvertisement(hostA, []string{"peer.example:8444"}, time.Now())
	if _, accepted, err := registry.Advertise(ad); err != nil || !accepted {
		t.Fatalf("Advertise(pin) accepted=%v err=%v", accepted, err)
	}
	forger, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v", err)
	}
	forged := session.ClaimRecord{ExternalURL: claimTestURL, ClaimingHostID: hostA.ID, ItemDeepLink: "ssq://evil/x", ClaimedAt: time.Now()}
	forged.Sign(forger)
	body, _ := json.Marshal(forged)
	if rec := postClaim(t, mux, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("forged POST status = %d, want 400", rec.Code)
	}
	if _, ok := index.CheckClaim(claimTestURL); ok {
		t.Fatalf("forged record stored, want rejected")
	}
}

func TestClaimLookupEndpoint_should_Return200RecordOr404OrBadRequest(t *testing.T) {
	mux, index, registry := newClaimTestMux(t)
	claimant, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v", err)
	}
	enroll(t, registry, claimant)
	record := session.NewSignedClaimRecord(claimant, claimTestURL, "ssq://claimant/backlog/v1/x", time.Now())
	if _, err := index.RecordClaim(record); err != nil {
		t.Fatalf("RecordClaim() error = %v", err)
	}

	get := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		session.SignClaimLookup(req.Header, claimant, req.URL.Query().Get("url"), time.Now())
		mux.ServeHTTP(rec, req)
		return rec
	}

	unsigned := httptest.NewRecorder()
	mux.ServeHTTP(unsigned, httptest.NewRequest(http.MethodGet, session.ClaimLookupEndpointPath+"?url="+claimTestURL, nil))
	if unsigned.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned lookup status = %d, want 401", unsigned.Code)
	}

	rec := get(session.ClaimLookupEndpointPath + "?url=" + claimTestURL)
	if rec.Code != http.StatusOK {
		t.Fatalf("known URL status = %d, want 200", rec.Code)
	}
	var got session.ClaimRecord
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ClaimingHostID.String() != claimant.ID.String() || !got.Verify() {
		t.Fatalf("lookup returned %+v, want the verifiable stored record", got)
	}

	if rec := get(session.ClaimLookupEndpointPath + "?url=https://github.com/o/r/issues/99"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown URL status = %d, want 404", rec.Code)
	}
	if rec := get(session.ClaimLookupEndpointPath); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing url status = %d, want 400", rec.Code)
	}
}
