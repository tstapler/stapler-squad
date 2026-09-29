package auth

import (
	"encoding/json"
	"net/http"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// RegisterClaimAdvertisementRoute registers the claim-gossip endpoint
// (ADR-001 of project_plans/cross-host-claim-dedup) on mux, sibling to
// RegisterHostAdvertisementRoute. A record ClaimIndex.RecordClaim rejects (bad
// signature or failed TOFU pin check) gets 400 and is never stored or
// re-gossiped. gossiper may be nil (tests); when set, a record that changed
// the index triggers one bounded re-gossip hop.
func RegisterClaimAdvertisementRoute(mux *http.ServeMux, index *session.ClaimIndex, gossiper *session.ClaimGossiper) {
	mux.HandleFunc("POST "+session.ClaimAdvertisementEndpointPath, func(w http.ResponseWriter, r *http.Request) {
		var record session.ClaimRecord
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
		if outcome.IsNew && gossiper != nil {
			// Synchronous for the same reason as the host advertisement route:
			// the fan-out is one bounded hop, and finishing it before replying
			// keeps convergence deterministic.
			if err := gossiper.ReGossip(r.Context(), record); err != nil {
				log.Debug("claim_advertisement.regossip_partial", "err", err)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}
