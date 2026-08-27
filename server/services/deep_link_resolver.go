package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/deeplink"
)

// backlogItemGetter is the narrow slice of *session.Storage the
// DeepLinkResolver needs — defined here (the consuming package) rather than
// in session, per this repo's interface-pollution convention, so a fake with
// no real *session.Storage (and no ent-backed repository) can stand in for
// tests.
type backlogItemGetter interface {
	GetBacklogItem(ctx context.Context, id string) (*session.BacklogItemData, error)
}

// HostResolver resolves a deep link's hostname to either a live remote
// host's advertised address or a reason it can't be reached, via the
// Workspace Host Registry (Epic 3). Defined narrowly here (consumer-side,
// per .claude/rules/interface-pollution-checklist.md) so Epic 3's Story 3.3
// can implement it against session.HostRegistry independently of this file.
type HostResolver interface {
	// ResolveHost looks up hostname in the registry. ok is false when the
	// hostname has no registry entry at all ("not-registered"); when ok is
	// true but reachable is false, the entry exists but the liveness check
	// failed ("unreachable"). advertisedAddress and lastSeenAt are
	// best-effort and may be empty even when ok is true.
	ResolveHost(ctx context.Context, hostname string) (advertisedAddress string, lastSeenAt string, reachable bool, ok bool)
}

// unimplementedHostResolver is the Story 2.2 placeholder for the
// cross-host branch: every hostname reports as not registered. Superseded
// by registryHostResolver (Story 3.3) but kept as the nil-hostResolver
// fallback in NewDeepLinkResolver.
type unimplementedHostResolver struct{}

func (unimplementedHostResolver) ResolveHost(_ context.Context, hostname string) (string, string, bool, bool) {
	return "", "", false, false
}

// defaultLivenessTimeout bounds the liveness HTTP check registryHostResolver
// performs against a registry entry's advertised address. Per ADR-002 and
// the implementation pre-mortem's requirement that "known but unreachable"
// be distinguished from "never registered" without hanging the resolve
// request indefinitely.
const defaultLivenessTimeout = 2 * time.Second

// registryHostResolver is the Story 3.3 HostResolver implementation: it
// looks up hostname in a session.HostRegistry (session/host_registry.go)
// and, when an entry exists, performs a bounded-timeout HTTP liveness check
// against its advertised address before reporting "handoff." It never
// synthesizes a redirect address from anything other than an already-present,
// current registry entry (per pitfalls.md, referenced by ADR-002) — a
// hostname with no entry is always "not-registered," never a guess.
//
// A fresh *session.HostRegistry is opened (re-reading host_registry.json)
// on every ResolveHost call rather than holding one long-lived instance:
// the registry is mutated by a separate HostRegistry instance owned by the
// host-advertisement gossip loop (see main.go's remote-server setup), so a
// cached instance here would never observe advertisements received after
// startup. Re-opening is a flock-protected file read, which is cheap enough
// for this feature's stated low request volume (requirements.md's "No new
// metrics/alerts — this is a low-volume, non-critical-path feature").
type registryHostResolver struct {
	stateDir        string
	registryTTL     time.Duration
	httpClient      *http.Client
	livenessTimeout time.Duration
}

// NewRegistryHostResolver creates a HostResolver backed by the
// session.HostRegistry persisted under stateDir, using ttl for entry
// expiry (see session.DefaultHostRegistryTTL).
func NewRegistryHostResolver(stateDir string, ttl time.Duration) *registryHostResolver {
	return &registryHostResolver{
		stateDir:        stateDir,
		registryTTL:     ttl,
		httpClient:      &http.Client{Transport: peerTLSTransport()},
		livenessTimeout: defaultLivenessTimeout,
	}
}

// peerTLSTransport returns an http.Transport for dialing another
// stapler-squad instance's --remote-port server (server/server.go's
// StartRemote always calls ServeTLS -- there is no plaintext-HTTP remote
// listener). Certificate verification is deliberately skipped: each
// instance mints its own local, self-signed TLS CA (server/tls.go) with no
// mechanism for two independently-provisioned hosts to share or exchange
// CAs, so per-connection TLS verification cannot succeed between peers by
// construction. Authentication instead happens at the application layer,
// exactly as ADR-002 designs it: an advertisement is only trusted if its
// Ed25519 signature verifies against a TOFU-pinned public key
// (session.HostRegistry.Advertise) -- TLS here provides transport
// encryption only, not peer identity.
func peerTLSTransport() *http.Transport {
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // see doc comment: identity is verified at the application layer (Ed25519/TOFU), not via the TLS chain
}

// ResolveHost implements HostResolver.
func (r *registryHostResolver) ResolveHost(ctx context.Context, hostname string) (advertisedAddress string, lastSeenAt string, reachable bool, ok bool) {
	registry, err := session.NewHostRegistry(r.stateDir, r.registryTTL)
	if err != nil {
		log.Warn("deep_link.host_registry_open_failed", "err", err)
		return "", "", false, false
	}

	_, entry, found := registry.LookupByHostname(hostname)
	if !found {
		return "", "", false, false
	}

	if len(entry.AdvertisedAddress) > 0 {
		advertisedAddress = entry.AdvertisedAddress[0]
	}
	lastSeenAt = entry.LastSeenAt.Format(time.RFC3339)
	reachable = r.checkLiveness(ctx, advertisedAddress)
	return advertisedAddress, lastSeenAt, reachable, true
}

// checkLiveness performs a bounded-timeout HTTP GET against address's
// /health endpoint (registered by every stapler-squad instance, see
// server/server.go). Any error (timeout, connection refused, non-2xx) is
// treated as unreachable — the distinction this resolver reports is only
// ever "reachable" vs. "unreachable," never a more granular network error.
// ctx is the resolving HTTP request's own context (HandleResolve's
// r.Context()), not context.Background() — a client that navigates away
// mid-check cancels this probe immediately instead of it running to its own
// fixed ceiling regardless.
func (r *registryHostResolver) checkLiveness(ctx context.Context, address string) bool {
	if address == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, r.livenessTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+address+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// DeepLinkResolver implements GET /api/deep-link/resolve — parses an ssq://
// deep link and returns either the local backlog item payload, the address
// of the host that owns it, or a distinct failure reason. See
// project_plans/backlog-deep-linking/design/ux.md Surface 11 for the
// response contract.
type DeepLinkResolver struct {
	items        backlogItemGetter
	hostResolver HostResolver
	ownHostname  func() (string, error)
}

// NewDeepLinkResolver creates a resolver backed by items. hostResolver may
// be nil, in which case every cross-host lookup reports "not-registered" —
// pass a *registryHostResolver (NewRegistryHostResolver) for a real
// registry-backed implementation.
func NewDeepLinkResolver(items backlogItemGetter, hostResolver HostResolver) *DeepLinkResolver {
	if hostResolver == nil {
		hostResolver = unimplementedHostResolver{}
	}
	return &DeepLinkResolver{
		items:        items,
		hostResolver: hostResolver,
		ownHostname:  os.Hostname,
	}
}

type deepLinkInvalidResponse struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type deepLinkNotFoundResponse struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type deepLinkUnreachableResponse struct {
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	LastSeenAt string `json:"lastSeenAt,omitempty"`
}

type deepLinkLocalResponse struct {
	Kind string                   `json:"kind"`
	Item *session.BacklogItemData `json:"item"`
}

type deepLinkHandoffResponse struct {
	Kind              string `json:"kind"`
	AdvertisedAddress string `json:"advertisedAddress"`
}

// RegisterRoutes wires the deep-link resolver into mux.
func (d *DeepLinkResolver) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/deep-link/resolve", d.HandleResolve)
}

// +http: GET /api/deep-link/resolve deep-link:resolve
// HandleResolve resolves the ssq:// URL in the "url" query parameter.
func (d *DeepLinkResolver) HandleResolve(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")

	link, err := deeplink.ParseDeepLink(raw)
	if err != nil {
		reason := "malformed"
		if errors.Is(err, deeplink.ErrUnsupportedVersion) {
			reason = "version-mismatch"
		}
		log.Warn("deep_link.resolve_failed", "reason", reason, "err", err)
		writeJSON(w, http.StatusBadRequest, deepLinkInvalidResponse{Kind: "invalid", Reason: reason})
		return
	}

	if d.isOwnHostname(link.Hostname) {
		d.resolveLocal(r.Context(), w, link)
		return
	}

	advertisedAddress, lastSeenAt, reachable, ok := d.hostResolver.ResolveHost(r.Context(), link.Hostname)
	if !ok {
		log.Warn("deep_link.resolve_failed", "host", link.Hostname, "reason", "not-registered")
		writeJSON(w, http.StatusConflict, deepLinkUnreachableResponse{Kind: "unreachable", Reason: "not-registered"})
		return
	}
	if !reachable {
		log.Warn("deep_link.resolve_failed", "host", link.Hostname, "reason", "unreachable", "last_seen", lastSeenAt)
		writeJSON(w, http.StatusConflict, deepLinkUnreachableResponse{Kind: "unreachable", Reason: "unreachable", LastSeenAt: lastSeenAt})
		return
	}

	log.Info("deep_link.resolved", "host", link.Hostname, "kind", "handoff")
	writeJSON(w, http.StatusOK, deepLinkHandoffResponse{Kind: "handoff", AdvertisedAddress: advertisedAddress})
}

// isOwnHostname compares link.Hostname (the authority component of the
// ssq:// URL) against this process's own network hostname. HostIdentity
// (session/host_identity.go) deliberately carries no hostname field — only
// an opaque HostID — so same-host detection here uses os.Hostname()
// directly rather than the identity/registry types, matching the hostname
// component AdvertisedAddress entries are keyed on
// (session.HostRegistry.LookupByHostname's net.SplitHostPort(addr) host
// part).
func (d *DeepLinkResolver) isOwnHostname(hostname string) bool {
	own, err := d.ownHostname()
	if err != nil {
		log.Warn("deep_link.own_hostname_lookup_failed", "err", err)
		return false
	}
	return strings.EqualFold(own, hostname)
}

func (d *DeepLinkResolver) resolveLocal(ctx context.Context, w http.ResponseWriter, link deeplink.DeepLink) {
	item, err := d.items.GetBacklogItem(ctx, link.ID)
	if err != nil {
		log.Warn("deep_link.resolve_failed", "host", link.Hostname, "item_id", link.ID, "reason", "deleted", "err", err)
		writeJSON(w, http.StatusNotFound, deepLinkNotFoundResponse{Kind: "not-found", Reason: "deleted"})
		return
	}

	if item.Status == string(session.BacklogStatusArchived) {
		log.Warn("deep_link.resolve_failed", "host", link.Hostname, "item_id", link.ID, "reason", "archived")
		writeJSON(w, http.StatusNotFound, deepLinkNotFoundResponse{Kind: "not-found", Reason: "archived"})
		return
	}

	log.Info("deep_link.resolved", "host", link.Hostname, "item_id", link.ID, "kind", "local")
	writeJSON(w, http.StatusOK, deepLinkLocalResponse{Kind: "local", Item: item})
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error("deep_link.write_response_failed", "err", err)
	}
}
