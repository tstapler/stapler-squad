package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// RegisterHostAdvertisementRoute registers the gossip-style host
// advertisement endpoint (ADR-002, plan.md Story 3.2) on mux.
//
// Task 1 confirmation: the --remote-port HTTPS server (main.go's
// startRemoteAccess) registering routes via this package's RegisterRoutes
// on srv.Mux() is the only genuinely cross-host-reachable HTTP surface in
// this codebase -- session/workspace_peers.go's WorkspacePeer is local-only
// (DB + `tmux list-sessions`, no network transport). So the advertisement
// endpoint is served here, as a sibling registration alongside RegisterRoutes
// (not folded into it, to avoid growing that function's already-long
// parameter list -- see the `primitive-obsession-checklist` skill),
// rather than a new listener or a piggyback on WorkspacePeer.
//
// advertiser may be nil (e.g. in tests exercising only accept/reject
// behavior); when set, a freshly-learned advertisement (Advertise returning
// isNew) triggers one bounded re-gossip hop to this instance's own known
// peers, excluding the sender's own AdvertisedAddress[].
func RegisterHostAdvertisementRoute(mux *http.ServeMux, identity session.HostIdentity, registry *session.HostRegistry, advertiser *session.HostAdvertiser, addresses []string) {
	mux.HandleFunc("POST "+session.AdvertisementEndpointPath, func(w http.ResponseWriter, r *http.Request) {
		var record session.AdvertisementRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			log.Debug("host_advertisement.received", "err", err)
			http.Error(w, "malformed advertisement payload", http.StatusBadRequest)
			return
		}

		isNew, accepted, err := registry.Advertise(record)
		if err != nil {
			log.Debug("host_advertisement.received", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Debug("host_advertisement.received", "host_id", record.HostIdentity.String(), "is_new", isNew, "accepted", accepted)

		if accepted && isNew && advertiser != nil {
			exclude := make(map[string]bool, len(record.AdvertisedAddress))
			for _, addr := range record.AdvertisedAddress {
				exclude[addr] = true
			}
			// Synchronous, not backgrounded: the re-gossip fan-out is bounded
			// (one hop, only for newly-learned identities -- see
			// HostAdvertiser.ReGossip's doc comment) so it is cheap enough to
			// finish before replying, and doing so keeps convergence
			// deterministic for callers/tests instead of racing the response.
			advertiser.ReGossip(r.Context(), record, exclude)
		}

		reply := session.BuildAdvertisement(identity, addresses, time.Now())
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(reply); err != nil {
			log.Debug("host_advertisement.sent", "err", err)
			return
		}
		log.Debug("host_advertisement.sent", "host_id", identity.ID.String())
	})
}
