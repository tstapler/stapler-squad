package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tstapler/stapler-squad/log"
	serverauth "github.com/tstapler/stapler-squad/server/auth"
)

// defaultRedetectHostnamesManualTimeout bounds both the send and the receive
// halves of a manual-trigger round trip (plan.md Task 4.2.2a / pre-mortem.md
// #3): guarding only the send and not the subsequent receive would still
// leave the handler able to hang indefinitely if Run were mid-cycle or stuck.
const defaultRedetectHostnamesManualTimeout = 10 * time.Second

// registerRedetectHostnamesEndpoint registers a loopback-only
// POST /api/debug/redetect-hostnames handler that forces one HostnameDetector
// cycle and returns its redetectCycle summary as JSON (plan.md Story 4.2.2).
// It never calls detector.redetect directly from the handler goroutine --
// that would race Run's own goroutine over the unsynchronized
// HostnameDetector.networks map -- and instead round-trips through
// detector.manual, the same channel Run's select loop already drains.
// disabled mirrors the STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE check that
// decided whether Run's goroutine was started at all: when true, nothing is
// listening on detector.manual, so the handler must fail fast (503) instead
// of blocking on a send nobody will ever receive. timeout is a parameter
// (not a package var mutated by tests) so tests can pass a short value
// directly instead of sharing mutable package state.
func registerRedetectHostnamesEndpoint(mux *http.ServeMux, detector *HostnameDetector, disabled bool, timeout time.Duration) {
	mux.HandleFunc("POST /api/debug/redetect-hostnames", func(w http.ResponseWriter, r *http.Request) {
		if !serverauth.IsLocalhostRequest(r) {
			http.Error(w, "forbidden: loopback only", http.StatusForbidden)
			return
		}
		if disabled {
			http.Error(w, "hostname redetection is disabled via STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE", http.StatusServiceUnavailable)
			return
		}

		respCh := make(chan redetectCycle, 1)
		select {
		case detector.manual <- respCh:
		case <-r.Context().Done():
			http.Error(w, "request cancelled", http.StatusGatewayTimeout)
			return
		case <-time.After(timeout):
			http.Error(w, "timed out sending manual redetect request", http.StatusGatewayTimeout)
			return
		}

		select {
		case cycle := <-respCh:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(cycle); err != nil {
				log.Warn("hostname-detect: failed to encode manual redetect response", "err", err)
			}
		case <-r.Context().Done():
			http.Error(w, "request cancelled", http.StatusGatewayTimeout)
		case <-time.After(timeout):
			http.Error(w, "timed out waiting for redetect cycle result", http.StatusGatewayTimeout)
		}
	})
}
