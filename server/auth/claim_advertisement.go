package auth

import (
	"encoding/json"
	"net/http"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// maxClaimAdvertisementBody caps a claim POST body; a real record is well under 1 KiB.
const maxClaimAdvertisementBody = 64 << 10

// RegisterClaimAdvertisementRoute registers the claim-gossip endpoint
// (ADR-001 of project_plans/cross-host-claim-dedup) on mux, sibling to
// RegisterHostAdvertisementRoute. A record ClaimIndex.RecordClaim rejects (bad
// signature, host not enrolled or failed TOFU pin check, out-of-range time,
// oversize fields) gets 400 and is never stored or re-gossiped. gossiper may be
// nil (tests); when set, a record that changed the index triggers one bounded
// re-gossip hop, run after the 200 reply is written.
func RegisterClaimAdvertisementRoute(mux *http.ServeMux, index *session.ClaimIndex, gossiper *session.ClaimGossiper) {
	registerClaimLookupRoute(mux, index)
	mux.HandleFunc("POST "+session.ClaimAdvertisementEndpointPath, func(w http.ResponseWriter, r *http.Request) {
		var record session.ClaimRecord
		r.Body = http.MaxBytesReader(w, r.Body, maxClaimAdvertisementBody)
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			log.Debug("claim_advertisement.received", "err", err)
			http.Error(w, "malformed claim payload", http.StatusBadRequest)
			return
		}
		outcome, err := index.RecordClaim(record)
		if err != nil {
			log.Warn("claim_advertisement.record_failed", "err", err)
			http.Error(w, "failed to record claim", http.StatusInternalServerError)
			return
		}
		log.Debug("claim_advertisement.received", "host_id", record.ClaimingHostID.String(),
			"external_url", record.ExternalURL, "accepted", outcome.Accepted, "is_new", outcome.IsNew)
		if !outcome.Accepted {
			http.Error(w, "claim rejected", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		if outcome.IsNew && gossiper != nil {
			// After the reply and off the request context: a slow or dead peer
			// must not hold this connection open, and the sender does not need
			// the fan-out to finish.
			gossiper.ReGossipAsync(record)
		}
	})
}

// registerClaimLookupRoute serves GET /internal/claim-lookup?url=<external URL>:
// 200 with the local index's signed ClaimRecord, or 404 when none is held. The
// caller must present a fresh signed-request header set from an enrolled host
// (session.SignClaimLookup), else 401, so the claim map and deep links are not
// readable by anyone who can reach the port. The asking peer verifies the
// returned record's signature itself, so the response is not trusted blindly.
func registerClaimLookupRoute(mux *http.ServeMux, index *session.ClaimIndex) {
	mux.HandleFunc("GET "+session.ClaimLookupEndpointPath, func(w http.ResponseWriter, r *http.Request) {
		externalURL := r.URL.Query().Get("url")
		if externalURL == "" {
			http.Error(w, "url query parameter is required", http.StatusBadRequest)
			return
		}
		if !index.AuthorizeClaimLookup(r.Header, externalURL) {
			http.Error(w, "claim lookup requires a signed request from an enrolled host", http.StatusUnauthorized)
			return
		}
		record, ok := index.CheckClaim(externalURL)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(record); err != nil {
			log.Debug("claim_lookup.write_failed", "err", err)
		}
	})
}
