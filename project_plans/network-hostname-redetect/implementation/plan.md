# Implementation Plan: network-hostname-redetect

**Feature**: Periodically (and on OS-level network-change events) re-run LAN hostname detection at runtime — thread-safely publishing results into `Server.hostnames`, WebAuthn RPIDs, and remote-access TLS cert SANs — so a network switch no longer requires a process restart.
**Date**: 2026-09-18
**Status**: Ready for implementation
**ADRs**: ADR-001 (adopt `tailscale.com/net/netmon` for OS-level network-change detection)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `HostnameDetector` | New type (`hostname_detector.go`, package `main`) owning the background `Run(ctx)` loop that periodically/eventually re-invokes `detectLANIPs`/`resolveLANHostnames` and publishes newly-discovered hostnames into `Server`, the WebAuthn `Handler`, and the TLS `NetworkCertStore`. | Mirrors `session/host_advertiser.go`'s `HostAdvertiser.Run(ctx)` shape. |
| `TriggerSource` | A defined string type (`type TriggerSource string`) with constants `TriggerStartup`, `TriggerTimer`, `TriggerNetworkChange`, `TriggerManual` identifying why a redetection cycle ran. | Sum type via defined constants, not raw string literals at call sites — used in every log line and test assertion. |
| `redetectCycle` (or `cycleResult`) | Internal struct capturing one run of the detection logic: `Trigger TriggerSource`, `Duration time.Duration`, `PrevCount int`, `NewCount int`, `Added []string`. | Exists only to make the Observability Plan's log line a single structured call site, not a domain entity persisted anywhere. |
| `NetworkChangeSource` | Small project-owned interface: `RegisterChangeCallback(func()) (unregister func())`, `Close() error`. Abstracts "something told us the network changed" so `HostnameDetector` never imports `tailscale.com/net/netmon` directly. | GoF Adapter around `*netmon.Monitor`; the interface is what tests fake. |
| `tailscaleNetmonSource` | The production `NetworkChangeSource` implementation, wrapping a real `*netmon.Monitor`. | New file, `netchange_source.go`, package `main`. |
| `NetworkCertStore` | New type in `server/tls.go`: `atomic.Pointer[map[string]*NetworkCert]` plus `Load()`/`Store(map[string]*NetworkCert)` methods, replacing the plain `certs map[string]*NetworkCert` closed over by `GetCertificateByLocalAddr`. | Same "publish immutable value, read lock-free" shape as `.claude/rules/instance-lock-free-reads.md`. |
| `RegisterHostname` | New exported method on `server/auth.Handler`: performs the same add-only RPID/origin registration `webauthnForHost` already does reactively (`webauthn.go:126-154`), callable proactively by `HostnameDetector` instead of waiting for a request. | Extracted from `webauthnForHost` via a shared private helper so there is exactly one place that constructs a new `webauthn.WebAuthn` instance for a hostname. |
| `verifyHostnameOwnership` | Package-level function in `main.go`, extracted from `startRemoteAccess`'s local `hostnameValidator` closure (`main.go:1312-1329`): forward-resolves a candidate hostname (via `net.LookupHost` + `forwardLookupViaKnownNameservers`) and returns true only if one of the resolved IPs matches one of `listNonLoopbackIPs()`. | **The single mandatory security gate.** Every hostname `HostnameDetector` discovers must pass this before it's eligible for `RegisterHostname` or a TLS SAN. Same function object is passed into `serverauth.NewHandler` as `hostnameValidator` and called directly by `HostnameDetector`, so there is exactly one implementation to audit. |
| `networks` | `map[string][]string` — LAN IP → SAN list, the same shape `startRemoetAccess` already builds at `main.go:1288-1293` for `EnsureNetworkTLSCerts`. `HostnameDetector` keeps its own copy across cycles, growing it add-only (never removing an IP or a hostname already present for it). | Not a new concept — reused shape, now mutated by a second call site. |
| `hostnameRedetectInterval` | `time.Duration`, default `5 * time.Minute`, overridable via `STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL` (parsed with `time.ParseDuration`). | Chosen to equal `negativeCacheTTL` (`server/auth/hostname_guard.go:19`) — see Pattern Decisions and the interval-choice rationale below. |

---

## Interval choice (must be concrete, per planning instructions)

**5 minutes**, matching `negativeCacheTTL`. Rationale: architecture.md and pitfalls.md both confirm a redetection interval `>=` the negative-cache TTL means a hostname that failed validation on the old network self-expires from `negativeHostnameCache` before or at the same time the next periodic cycle would re-check it — no cache-busting method needed. Going shorter (e.g. 1 minute) would need an explicit `negativeHostnameCache.Clear` call, adding a story for no user-visible benefit given the OS-event trigger already covers the "I want it fast" case. Configurable via `STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL` (duration string, e.g. `2m`) and disableable via `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true`, per the Risk Control section's "trivially disableable" requirement.

**Note on the negative-cache Open Question**: it resolves even more cleanly than "self-expires in time" — `HostnameDetector`'s successful `RegisterHostname` call adds the hostname directly to `Handler.rpIDs`/`Handler.webauthn`, so a subsequent real request's `webauthnForHost` matches it in its *first* loop (`webauthn.go:100-106`, checking `h.rpIDs`) and never reaches the `negativeCache.IsNegative` check at all. The negative cache is only consulted for hostnames *not yet* in `rpIDs` — once the detector adds one, that hostname is permanently past that gate for the rest of the process's life (consistent with add-only scope).

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `HostnameDetector.Run(ctx)` | Active-object background loop (ticker + event `select`), mirroring `HostAdvertiser.Run` | Repo convention (`session/host_advertiser.go:75-91`) | Push-based observer/pub-sub fan-out to `Server`/`Handler`/TLS as independent subscribers | architecture.md: no existing pub/sub primitive in this codebase, and all three consumers are already *pull*-based (`Load()`/per-request/per-handshake) — a push layer would add abstraction for zero benefit |
| `Server.hostnames` storage | `atomic.Pointer[[]string]` copy-on-write + `Snapshot()`-style `Load()` | `.claude/rules/instance-lock-free-reads.md`, `session/instance_snapshot.go` | `sync.RWMutex`-guarded plain field | Repo already standardizes on atomic-publish for this exact read-heavy/write-rare shape (`Server.addr` at `server/server.go:54` is the same field already doing this) — consistency over introducing a second idiom |
| TLS `certs` map storage | `NetworkCertStore` = `atomic.Pointer[map[string]*NetworkCert]` | Same rule as above | `sync.RWMutex`-guarded map | Same reasoning; also matches the exact hazard class `instance-lock-free-reads.md` was written for (a closure read on every TLS handshake racing a background writer) |
| OS network-change trigger | Adapter (GoF) — `NetworkChangeSource` interface wrapping `*tailscale.com/net/netmon.Monitor` | GoF; build-vs-buy.md's "Recommended: adopt tailscale.com/net/netmon" | Hand-rolled `AF_ROUTE`/netlink parsing per OS (`golang.org/x/net/route` + `golang.org/x/sys/unix`) | build-vs-buy.md: `x/net/route` has an open, unresolved live-event parsing bug (golang/go#44740) on Darwin; hand-rolling reproduces a known-hard-to-get-right protocol surface for a single-host diagnostic feature |
| `TriggerSource` | Defined-string sum type with exported constants | type-driven-design | Raw `string` literals (`"timer"`, `"event"`) at each call site | Compiler/linter-visible exhaustiveness for the 4 known triggers; prevents a typo'd trigger name silently breaking a log-based grep the Observability Plan depends on |
| Hostname ownership validation | Single extracted function (`verifyHostnameOwnership`), reused by both `startRemoteAccess`'s `hostnameValidator` and `HostnameDetector` | Fowler "Extract Function" (not a GoF pattern, but the load-bearing decision here) | Reimplementing an equivalent round-trip check inside `HostnameDetector` | pitfalls.md §3: this is the exact "materially larger attack surface" risk — a second, possibly-divergent implementation is precisely the bug class to avoid; one function, one thing to audit |
| WebAuthn proactive registration | Extract shared private helper (`registerRPIDLocked`) called by both `webauthnForHost`'s existing reactive path and the new exported `RegisterHostname` | PoEAA-adjacent "Extract Method" to avoid duplicated Service-Layer logic | Duplicate the `webauthn.New(...)` + `h.webauthn[...]=...` + `h.rpIDs = append(...)` block in a new method | Two independent implementations of "how a hostname becomes a trusted rpID" is the divergence risk features.md's "Unstated user needs" section calls out explicitly |
| Detection primitives (`detectLANIPs`, `resolveLANHostnames`) | Transaction-Script-style plain functions, left untouched | PoEAA (Fowler) — appropriately simple for stateless OS-command-then-parse logic | Wrapping them in a `Repository`/`Service Layer` abstraction | Requirements explicitly forbid rewriting this logic; it's already the right level of complexity for what it does |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `main.go` (~1400+ lines, `startRemoteAccess` alone spans hundreds of lines) | God-function mixing CLI parsing, boot-time detection, TLS setup, WebAuthn wiring | Extend as-is | architecture.md's explicit finding: not this project's problem to fix; `HostnameDetector` is introduced as an isolated new type in a new file, and `startRemoteAccess`'s only required change is returning a few already-constructed values (`waHandler`, `certStore`, `networks`) instead of restructuring its control flow |

---

## Observability Plan
- **Logs**: one structured `slog` line per redetection cycle (success path), matching `logs/staplersquad.log` conventions: `log.Info("hostname-detect: cycle complete", "trigger", trigger, "duration", dur, "prev_count", prevCount, "new_count", newCount, "added", added)` — emitted even when `added` is empty, per features.md's "confidence that the fix actually engaged" finding (a "nothing changed" cycle is as important to see as a hostname being added). A `log.Warn` line when `verifyHostnameOwnership` rejects a candidate hostname (`"hostname-detect: dropped unverified candidate", "hostname", h`), mirroring the existing boot-time warning at `main.go:1343`.
- **Metrics**: none — requirements.md explicitly says no new metric/alert is required (single-host diagnostic logging only).
- **Alerts**: no new alerts required.

## Risk Control
- **Feature flag**: `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true` (env var, default unset = enabled) short-circuits `HostnameDetector.Run` into a no-op before starting the ticker/event loop — satisfies "trivially disableable" without a full config-schema feature-flag plumbing effort.
- **Rollback procedure**: standard revert via PR close + revert commit — no data migration, no persisted state format change (per requirements.md's own Risk Control framing).
- **Staged rollout**: full rollout on merge (single-developer personal tool, no cohort/canary infra applicable).

## Unresolved Questions
- [ ] Confirm `tailscale.com/net/netmon`'s actual transitive dependency diff via `go mod tidy` + `git diff go.sum` is acceptable before merging — blocks Story 3.1.1 — owner: Tyler (per build-vs-buy.md, this was explicitly flagged as not independently verified during research).

## Dependency Visualization

```
Phase 1 (seams, independently testable, no new behavior)
  1.1.1 Server.hostnames -> atomic.Pointer      \
  1.2.1 NetworkCertStore                         >--- all three land before Phase 2 wires them
  1.3.1 Handler.RegisterHostname                /
        |
        v
Phase 2 (detector core, uses Phase 1 seams)
  2.1.1 HostnameDetector type + Run loop (fake deps only)
        |
        v
  2.1.2 Wire into main.go runtime phase + extract verifyHostnameOwnership
        |
        v
  2.2.1 Mandatory validation gate + adversarial test
        |
        v
Phase 3 (OS event source; independent of 2.2.1's logic, needs 2.1.1's event-channel field)
  3.1.1 go.mod: add tailscale.com/net/netmon, verify diff
        |
        v
  3.1.2 NetworkChangeSource + tailscaleNetmonSource, wired into HostnameDetector's events field
        |
        v
Phase 4 (operability, depends on 2.1.2 for the running detector to control/observe)
  4.1.1 Structured per-cycle logging (can start once 2.1.1 exists)
  4.2.1 Disable/interval env vars
  4.2.2 Loopback-only manual trigger endpoint
```

---

## Phase 1: Thread-Safe Publish Seams

### Epic 1.1: `Server.hostnames` atomic publish
**Goal**: Make `Server.hostnames` safe for a background writer, with add-only merge semantics, before anything writes to it concurrently.

#### Story 1.1.1: Replace the unsynchronized `hostnames []string` field with `atomic.Pointer[[]string]`
**As a** background `HostnameDetector` goroutine, **I want** to update `Server.hostnames` while HTTP handlers read it concurrently, **so that** a network-change redetection cycle never races `go test -race`-visible readers.
**Acceptance Criteria**:
- `SetHostnames` computes the add-only union of the previously-published slice and the new candidate slice, then atomically publishes the union — it never shrinks the set.
  - *Given* `Server.hostnames` currently holds `["netflix1.staplerhome.com"]`, *When* `SetHostnames(["netflix1.staplerhome.com", "netflix1.newwifi.local"])` is called, *Then* `GetHostnames()` returns both `netflix1.staplerhome.com` and `netflix1.newwifi.local`, in a deterministic (e.g. previous-order-preserved, new-entries-appended) order.
- `GetHostnames` never blocks and never data-races with a concurrent `SetHostnames`.
  - *Given* one goroutine calls `SetHostnames` in a loop and another calls `GetHostnames` in a loop for 100ms, *When* the test runs under `go test -race`, *Then* no race is reported.
**Files**: `server/server.go`

##### Task 1.1.1a: Change the field and accessors (~4 min)
- In `server/server.go`, change `hostnames []string` (line 60) to `hostnames atomic.Pointer[[]string]`.
- Rewrite `SetHostnames`/`GetHostnames` (lines 1510-1518): `SetHostnames` loads the current slice via `s.hostnames.Load()`, builds a `seen map[string]bool` from it, appends any new entries from the argument not already present, and `Store`s the resulting slice. `GetHostnames` returns `*p` if `s.hostnames.Load()` is non-nil, else `nil`.
- Files: `server/server.go`

##### Task 1.1.1b: Add a race/concurrency test (~5 min)
- New test in `server/server_test.go` (or a new `server/server_hostnames_test.go` if the existing file is already large): `TestServer_SetHostnames_AddOnlyMerge` (table-driven: seed a server, call `SetHostnames` twice with overlapping and new entries, assert `GetHostnames()` is the union) and `TestServer_SetHostnames_ConcurrentAccessIsRaceFree` (spawn a writer and reader goroutine for a bounded number of iterations, no `time.Sleep`, join both before returning — run under `go test -race`).
- Files: `server/server_test.go` (or new `server/server_hostnames_test.go`)

---

### Epic 1.2: TLS cert map atomic publish
**Goal**: Make the `certs map[string]*NetworkCert` that `GetCertificateByLocalAddr`'s closure reads swappable at runtime without racing live TLS handshakes.

#### Story 1.2.1: Introduce `NetworkCertStore`
**As a** background `HostnameDetector` goroutine, **I want** to publish a freshly-issued `certs` map after a new network's SANs are detected, **so that** a live TLS handshake never reads a half-updated map.
**Acceptance Criteria**:
- `GetCertificateByLocalAddr` selects a cert from whatever `NetworkCertStore.Load()` currently returns, not a value captured once at construction time.
  - *Given* a `NetworkCertStore` initially holding a cert only for `"192.168.1.5"`, *When* `Store` is called with a map that also contains `"192.168.1.9"`, *Then* a subsequent `GetCertificate` callback invocation for a connection whose local address is `"192.168.1.9"` returns the new cert without restarting the listener.
- No unsynchronized map read/write: `NetworkCertStore.Load`/`Store` are the only access points.
  - *Given* `NetworkCertStore.Store` is called concurrently with `GetCertificateByLocalAddr`'s returned function invoking `Load`, *When* `go test -race` runs both in a loop, *Then* no race is reported.
**Files**: `server/tls.go`

##### Task 1.2.1a: Add `NetworkCertStore` type (~4 min)
- In `server/tls.go`, add `import "sync/atomic"` and:
  ```go
  // NetworkCertStore publishes the current network->cert map for
  // GetCertificateByLocalAddr's per-handshake read, replacing a plain
  // captured map so a runtime-added network's cert doesn't race a live
  // TLS handshake reading the old map.
  type NetworkCertStore struct {
      certs atomic.Pointer[map[string]*NetworkCert]
  }

  func NewNetworkCertStore(initial map[string]*NetworkCert) *NetworkCertStore {
      s := &NetworkCertStore{}
      s.Store(initial)
      return s
  }

  func (s *NetworkCertStore) Store(certs map[string]*NetworkCert) { s.certs.Store(&certs) }

  func (s *NetworkCertStore) Load() map[string]*NetworkCert {
      if p := s.certs.Load(); p != nil {
          return *p
      }
      return nil
  }
  ```
- Files: `server/tls.go`

##### Task 1.2.1b: Change `GetCertificateByLocalAddr`'s signature (~3 min)
- Change `func GetCertificateByLocalAddr(certs map[string]*NetworkCert) func(*tls.ClientHelloInfo) (*tls.Certificate, error)` (line 128) to accept `store *NetworkCertStore` and call `store.Load()` inside the returned closure instead of closing over `certs` directly.
- Files: `server/tls.go`

##### Task 1.2.1c: Update the call site and add a test (~5 min)
- In `main.go`'s `startRemoteAccess` (around line 1295-1301), wrap `netCerts` in `server.NewNetworkCertStore(netCerts)` and pass the store to `server.GetCertificateByLocalAddr`.
- New test `TestNetworkCertStore_StoreThenLoad_ReflectsUpdate` and `TestGetCertificateByLocalAddr_ReadsLatestStore` in `server/tls_test.go` (check whether this file exists first via Glob; create it if not, following existing `server/*_test.go` conventions).
- Files: `main.go`, `server/tls_test.go`

---

### Epic 1.3: WebAuthn proactive registration seam
**Goal**: Give `HostnameDetector` a way to proactively register a newly-discovered, already-validated hostname as a trusted RPID, reusing (not duplicating) `webauthnForHost`'s existing add-only logic.

#### Story 1.3.1: Extract `registerRPIDLocked` and add exported `RegisterHostname`
**As a** `HostnameDetector`, **I want** to call one `Handler` method to register a hostname I've already verified, **so that** there is exactly one code path that decides how a hostname becomes a trusted rpID.
**Acceptance Criteria**:
- `RegisterHostname(hostname string) error` adds `hostname` to `h.rpIDs`/`h.webauthn` and its derived origin to `h.origins`, using the same logic `webauthnForHost` uses at request time — verified by both call paths producing an identical `*webauthn.WebAuthn` config for the same hostname.
  - *Given* a `Handler` constructed with `rpIDs=["onyx.local"]`, `origins=["https://onyx.local:8444"]`, *When* `RegisterHostname("netflix1.newwifi.local")` is called, *Then* `h.rpIDs` contains both hostnames and a later request with `Host: netflix1.newwifi.local:8444` succeeds in `webauthnForHost`'s first loop (no DNS lookup triggered).
- `RegisterHostname` is idempotent: registering an already-registered hostname is a no-op, not a duplicate `rpIDs` entry.
  - *Given* `"netflix1.newwifi.local"` is already registered, *When* `RegisterHostname("netflix1.newwifi.local")` is called again, *Then* `len(h.rpIDs)` is unchanged and no error is returned.
- `RegisterHostname` does **not** itself call `hostnameValidator` or consult `negativeCache`/`ipLimiter` — those guard the *reactive*, request-triggered path against an unauthenticated caller; `HostnameDetector` (the *only* caller of `RegisterHostname`) is responsible for calling `verifyHostnameOwnership` itself first (see Epic 2.2). Document this contract in `RegisterHostname`'s doc comment.
  - *Given* `RegisterHostname` is called directly in a unit test with an arbitrary string, *When* the call completes, *Then* it succeeds (or fails only on `originForHost` derivation, e.g. no configured origins) regardless of whether that string is DNS-resolvable — proving the method trusts its caller and does not re-validate.
**Files**: `server/auth/webauthn.go`

##### Task 1.3.1a: Extract `registerRPIDLocked` (~5 min)
- In `server/auth/webauthn.go`, extract lines 126-154 (the `h.mu.Lock()` body building `newOrigin`, `webauthn.New`, and appending to `h.webauthn`/`h.rpIDs`/`h.origins`) into a private method `func (h *Handler) registerRPIDLocked(hostname string) (*webauthn.WebAuthn, error)` that assumes the caller already holds `h.mu.Lock()` (mirrors the existing `webauthnForHost` call site, which already does `h.mu.Lock(); defer h.mu.Unlock()` right before this block).
- `webauthnForHost` calls `h.registerRPIDLocked(hostname)` in place of the extracted block.
- Files: `server/auth/webauthn.go`

##### Task 1.3.1b: Add exported `RegisterHostname` (~4 min)
- Add:
  ```go
  // RegisterHostname proactively registers hostname as a trusted rpID, using
  // the same add-only logic webauthnForHost applies reactively at request
  // time. Callers MUST have already validated hostname (e.g. via
  // verifyHostnameOwnership) -- unlike webauthnForHost's request path,
  // RegisterHostname performs no DNS validation and is not rate-limited,
  // since it has no untrusted caller (only HostnameDetector calls this).
  func (h *Handler) RegisterHostname(hostname string) error {
      h.mu.Lock()
      defer h.mu.Unlock()
      for _, rpID := range h.rpIDs {
          if rpID == hostname {
              return nil // already registered
          }
      }
      _, err := h.registerRPIDLocked(hostname)
      return err
  }
  ```
- Files: `server/auth/webauthn.go`

##### Task 1.3.1c: Tests (~5 min)
- Add `TestHandler_RegisterHostname_AddsNewRPIDAndOrigin`, `TestHandler_RegisterHostname_IdempotentOnRepeat`, `TestHandler_RegisterHostname_SubsequentRequestSkipsHostnameValidator` (the last one passes a `hostnameValidator` that panics/fails the test if called, then does `RegisterHostname` + a request through `webauthnForHost`, asserting the validator was never invoked) to `server/auth/webauthn_test.go`.
- Files: `server/auth/webauthn_test.go`

---

## Phase 2: HostnameDetector Core Loop

### Epic 2.1: Detector type + ticker/event loop
**Goal**: A fully unit-testable `HostnameDetector` with injectable detection/resolution/timer/event dependencies, wired into `main.go` without real subprocess calls in tests.

#### Story 2.1.1: `HostnameDetector` type with injectable dependencies
**As a** developer, **I want** `HostnameDetector.Run(ctx)` to be testable with fake clocks/events and no real DNS/subprocess calls, **so that** debounce and update logic are covered by fast, deterministic CI tests per this repo's `deterministic-fast-tests` skill.
**Acceptance Criteria**:
- `Run(ctx)` performs one detection cycle immediately (trigger=`TriggerStartup`), then loops on `select { <-ctx.Done(); <-tick; <-events }`, calling the same internal cycle function for every branch.
  - *Given* a `HostnameDetector` constructed with a fake `detectFn` returning `["10.0.0.5"]` and a fake `resolveFn` returning `["host.example.local"]` for that IP, and a manually-controlled `tick <-chan time.Time` and `events <-chan struct{}`, *When* `Run(ctx)` starts and the test sends one value on `tick`, *Then* the detector's `redetect` runs exactly twice total (once at startup, once for the tick) — asserted by a counter in the fake `detectFn`, not by wall-clock timing.
- `Run(ctx)` returns promptly when `ctx` is cancelled, even mid-cycle-wait.
  - *Given* `Run(ctx)` is started in a goroutine with no ticks/events sent, *When* the test cancels `ctx`, *Then* `Run` returns within the test's `goleak`-checked cleanup (no `time.Sleep`-based wait — join on a `done chan struct{}` closed by `Run`'s return).
- Each cycle computes the add-only union of the detector's own `networks map[string][]string` state, publishes the result to `srv.SetHostnames`, and logs the cycle summary (Epic 4.1) — but does **not** yet call `RegisterHostname` or update TLS certs (that's Epic 2.2, gated by validation).
  - *Given* the detector's `networks` state is `{"10.0.0.5": ["host.example.local"]}` and a cycle's `resolveFn` for a newly-detected IP `"10.0.0.9"` returns `["other.example.local"]`, *When* the cycle runs, *Then* `networks` becomes `{"10.0.0.5": ["host.example.local"], "10.0.0.9": ["other.example.local"]}` and `srv.GetHostnames()` includes both hostnames.
- `resolveFn` is re-run for **every** currently-detected IP on every cycle, not only newly-seen ones — an IP already present as a key in `networks` is never permanently skipped, so a transient resolution failure on first sighting cannot freeze discovery for that IP forever.
  - *Given* the detector's `networks` state is `{"10.0.0.5": ["host.example.local"]}` from a prior cycle, *When* a new cycle runs and `resolveFn("10.0.0.5")` this time additionally returns `"host2.example.local"`, *Then* `networks["10.0.0.5"]` becomes `["host.example.local", "host2.example.local"]` (add-only merge into the existing slice, not a skip because the IP was already a key).
  - *Given* `resolveFn(ip)` returned an empty result for `ip` on its first-ever sighting (a transient failure), *When* a later cycle runs with `resolveFn(ip)` now returning a hostname, *Then* that hostname is added to `networks[ip]` — the earlier empty result did not permanently exclude `ip` from being re-resolved.
- A manual trigger (Story 4.2.2) requests a cycle through the same serialized path as the ticker/event cases, never by calling `redetect` from another goroutine.
  - *Given* `Run(ctx)` is executing its `select` loop, *When* a caller sends a response channel on `d.manual`, *Then* `Run` calls `redetect(ctx, TriggerManual)` itself and sends the resulting `redetectCycle` back on that channel — `d.networks` is never touched by any goroutine other than the one running `Run`.
**Files**: `hostname_detector.go` (new)

##### Task 2.1.1a: Define types and constructor (~5 min)
- New file `hostname_detector.go` (package `main`). Define `type TriggerSource string` with `const (TriggerStartup TriggerSource = "startup"; TriggerTimer = "timer"; TriggerNetworkChange = "network-change"; TriggerManual = "manual")`.
- Define `HostnameDetector` struct with fields: `srv *server.Server`, `detectFn func() []string` (defaults to `detectLANIPs`), `resolveFn func(string) []string` (defaults to `resolveLANHostnames`), `networks map[string][]string`, `tick <-chan time.Time`, `events <-chan struct{}`, `manual chan chan redetectCycle` (unbuffered; the sole channel through which any caller outside `Run`'s own goroutine — e.g. the manual-trigger HTTP handler in Task 4.2.2a — may request a cycle, so every mutation of `d.networks` stays serialized inside `Run`'s single `select` loop), `done chan struct{}`.
- `NewHostnameDetector(srv *server.Server, initialNetworks map[string][]string, tick <-chan time.Time, events <-chan struct{}) *HostnameDetector` — sets `detectFn`/`resolveFn` to the real package functions, copies `initialNetworks` into its own map.
- Files: `hostname_detector.go`

##### Task 2.1.1b: Implement `Run` and `redetect` (~5 min)
- `func (d *HostnameDetector) Run(ctx context.Context)`: defer `close(d.done)`; call `d.redetect(ctx, TriggerStartup)`; loop `select { case <-ctx.Done(): return; case <-d.tick: d.redetect(ctx, TriggerTimer); case <-d.events: d.redetect(ctx, TriggerNetworkChange); case respCh := <-d.manual: respCh <- d.redetect(ctx, TriggerManual) }`. This `manual` case is the **only** way any goroutine other than `Run`'s own may trigger a cycle — it keeps every `redetect` call, and therefore every mutation of the unsynchronized `d.networks` map, serialized inside this single `select` loop (see Task 4.2.2a, which must send on `d.manual` rather than calling `redetect` directly).
- `func (d *HostnameDetector) redetect(ctx context.Context, trigger TriggerSource) redetectCycle`: snapshot `time.Now()`; call `d.detectFn()` for LAN IPs; for **every** currently-detected IP (not just newly-seen ones — see Story 2.1.1's acceptance criteria and Task 2.2.1a), call `d.resolveFn(ip)` and add-only-merge any newly-found hostnames into `d.networks[ip]`'s existing slice (dedup; "already a key" means "known, keep re-resolving," never "resolved once, skip forever" — a transient resolution failure on first sighting must not permanently freeze discovery for that IP); flatten `d.networks` into a single hostname slice; call `d.srv.SetHostnames(flattened)`; return a `redetectCycle{Trigger: trigger, Duration: time.Since(start), PrevCount: ..., NewCount: ..., Added: ...}` (logging wired in Epic 4.1). This means one more `resolveFn` subprocess call per already-known IP per cycle than a "new IPs only" approach — intentional, and still cheap against the 5-minute default interval (dozens of calls every cycle is well within the NFR budget).
- Files: `hostname_detector.go`

##### Task 2.1.1c: Unit tests with fakes (~5 min)
- New `hostname_detector_test.go`: `TestHostnameDetector_Run_StartupCycleRunsImmediately`, `TestHostnameDetector_Run_TickTriggersAnotherCycle`, `TestHostnameDetector_Run_EventTriggersAnotherCycle`, `TestHostnameDetector_Run_StopsOnContextCancel` (assert via closing `d.done` and `goleak.VerifyNone`), `TestHostnameDetector_Redetect_AddOnlyMergesNetworks`.
- Files: `hostname_detector_test.go`

#### Story 2.1.2: Wire `HostnameDetector` into `main.go`'s runtime phase
**As a** operator running `stapler-squad`, **I want** the detector to actually run in production, sharing the same shutdown-cancellable `ctx` as every other background loop, **so that** it starts and stops cleanly with the rest of the process.
**Acceptance Criteria**:
- `startRemoteAccess` returns the values `HostnameDetector` needs (the `waHandler`, the `NetworkCertStore`, and the initial `networks` map it already built) instead of only an `error`.
  - *Given* `remoteAccessFlag` is true, *When* `startRemoteAccess` returns successfully, *Then* the caller in `main.go`'s `"runtime"` phase has a non-nil `*serverauth.Handler` and `*server.NetworkCertStore` to pass into `NewHostnameDetector`.
- `HostnameDetector.Run` is started via `a.Go("hostname-detector", ...)`, the same mechanism as the existing `"http-server"` goroutine (`main.go:461`), so it is cancelled by the same `signal.NotifyContext`-derived `ctx`.
  - *Given* the process receives `SIGTERM`, *When* `app.Run(ctx)` unwinds, *Then* the `hostname-detector` goroutine observes `ctx.Done()` and returns (no goroutine leak — verified by `server/server_integration_test.go`-style `goleak` coverage if an existing integration test already starts the full app; otherwise a targeted test starting just the detector).
- `verifyHostnameOwnership` (extracted from the old `hostnameValidator` closure) is a package-level function passed both into `serverauth.NewHandler` (as before) and used directly by the detector's Epic 2.2 validation step — no second implementation.
  - *Given* `verifyHostnameOwnership("spoofed.example")` and `listNonLoopbackIPs()` returning `["192.168.1.5"]`, *When* `net.LookupHost("spoofed.example")` resolves to `["203.0.113.9"]` (not this host's IP), *Then* `verifyHostnameOwnership` returns `false`.
**Files**: `main.go`, `server/auth/webauthn.go` (no change here, just consumer), `hostname_detector.go`

##### Task 2.1.2a: Extract `verifyHostnameOwnership` (~4 min)
- In `main.go`, move the `hostnameValidator` closure body (lines 1312-1329) into a package-level function `func verifyHostnameOwnership(hostname string) bool`. `startRemoteAccess` now does `hostnameValidator := verifyHostnameOwnership` (or passes the function directly to `serverauth.NewHandler`) — no behavior change at this call site.
- Files: `main.go`

##### Task 2.1.2b: Change `startRemoteAccess`'s return shape (~5 min)
- Define a small result struct in `main.go`: `type remoteAccessResult struct { Handler *serverauth.Handler; CertStore *server.NetworkCertStore; Networks map[string][]string }`.
- Change `startRemoteAccess`'s signature to `func startRemoteAccess(ctx context.Context, srv *server.Server, localAddr string, cfg *config.Config, remotePort int) (*remoteAccessResult, error)`, returning `&remoteAccessResult{Handler: waHandler, CertStore: certStore, Networks: networks}, nil` on success (using the `certStore` introduced in Task 1.2.1c and the `networks` map already built at line 1288-1293).
- Update the call site (around `main.go:456`) to capture the result.
- Files: `main.go`

##### Task 2.1.2c: Start the detector goroutine (~5 min)
- The detector **always starts**, unconditionally, in the `"runtime"` phase — regardless of `remoteAccessFlag`/`cfg.PasskeyEnabled` — because periodic redetection of `Server.hostnames` is the feature's primary success metric and must not depend on remote access being enabled. Only `startRemoteAccess`'s own `if remoteAccessFlag || cfg.PasskeyEnabled` guard stays as-is (unchanged, still gates whether `waHandler`/`certStore`/`networks` exist at all).
- After the `"runtime"` phase's `if remoteAccessFlag || cfg.PasskeyEnabled { ... }` block (whether or not it ran), construct `detector := NewHostnameDetector(srv, initialNetworks, time.NewTicker(hostnameRedetectInterval()).C, netChangeEvents)` where `initialNetworks` is `remoteAccess.Networks` when that block ran, or an empty `map[string][]string{}` when it didn't (`hostnameRedetectInterval()` and `netChangeEvents` are stubbed/added properly in Phase 3/4 — for this task, `netChangeEvents` can be a `make(chan struct{})` never-written placeholder, wired for real in Story 3.1.2). Start it with `a.Go("hostname-detector", func(ctx context.Context) { detector.Run(ctx) })` outside and after that `if` block, so it runs on every install regardless of the remote-access/passkey setting.
- Only the `Handler`/`CertStore` fields passed into `NewHostnameDetector` are conditional: pass `remoteAccess.Handler`/`remoteAccess.CertStore` when the `if` block ran, else `nil`/`nil` — the detector's own `redetect` logic already nil-checks these before calling `RegisterHostname`/`EnsureNetworkTLSCerts` (Task 2.2.1a), so a nil-remote-access install still gets `Server.hostnames` kept current, just not RPID/TLS publication.
- Files: `main.go`

---

### Epic 2.2: Security-mandatory validation gate
**Goal**: No hostname coming out of `HostnameDetector` reaches `RegisterHostname` or a TLS SAN without passing `verifyHostnameOwnership` first — the control pitfalls.md identifies as "the actual control," not optional hardening.

#### Story 2.2.1: Gate `RegisterHostname`/TLS-SAN publication on `verifyHostnameOwnership`
**As a** security-conscious maintainer, **I want** every hostname the periodic detector discovers to be forward-DNS-verified as resolving to this host's own IP before it can become a trusted WebAuthn RPID or TLS SAN, **so that** a spoofed mDNS/PTR answer on an untrusted network can never plant a permanently-trusted identity (add-only, no expiry).
**Acceptance Criteria**:
- A hostname that fails `verifyHostnameOwnership` is logged and dropped — it still updates `Server.hostnames` (internal bookkeeping only, per requirements' "just make the internal list current" framing) but never reaches `Handler.RegisterHostname` or the `networks` map fed into `EnsureNetworkTLSCerts`.
  - *Given* `resolveLANHostnames` returns a candidate `"evil-onyx.local"` for a detected IP, and `verifyHostnameOwnership("evil-onyx.local")` returns `false` (it doesn't forward-resolve to this host's own IP), *When* a redetection cycle runs, *Then* `Server.GetHostnames()` may still include `"evil-onyx.local"`, but `waHandler.rpIDs` does not, and no TLS cert is (re)issued containing it in its SANs.
- A hostname that passes `verifyHostnameOwnership` is registered via `Handler.RegisterHostname` and folded into the `networks` map that's passed to `EnsureNetworkTLSCerts` on the next call.
  - *Given* `resolveLANHostnames` returns `"netflix1.newwifi.local"` for IP `10.0.0.9`, and `verifyHostnameOwnership("netflix1.newwifi.local")` returns `true`, *When* a redetection cycle runs, *Then* `waHandler.RegisterHostname("netflix1.newwifi.local")` was called and `EnsureNetworkTLSCerts` is invoked with `networks["10.0.0.9"]` containing that hostname.
- `Handler`/`CertStore` being `nil` (remote access disabled) short-circuits this step without panicking.
  - *Given* a `HostnameDetector` constructed with `Handler: nil, CertStore: nil`, *When* a redetection cycle runs and finds a new, verified hostname, *Then* it updates `Server.hostnames` only and does not attempt to call a nil `Handler`.
- A cert-issuance failure never rolls back RPID registrations already made in the same cycle.
  - *Given* `verifiedNew` contains `"netflix1.newwifi.local"` and `d.waHandler.RegisterHostname` succeeds for it, *When* the subsequent `server.EnsureNetworkTLSCerts(d.networks)` call in the same cycle returns an error, *Then* `waHandler.rpIDs` still contains `"netflix1.newwifi.local"` (not rolled back), `d.certStore.Store(...)` is not called this cycle, and a `log.Error` line is emitted.
**Files**: `hostname_detector.go`

##### Task 2.2.1a: Add validation + downstream publish to `redetect` (~5 min)
- Extend `HostnameDetector` with `waHandler *serverauth.Handler` and `certStore *server.NetworkCertStore` fields (nil-able) and a `validateFn func(string) bool` field (defaults to `verifyHostnameOwnership`, injectable for tests).
- In `redetect`, for each newly-discovered hostname (present in this cycle's resolve results but not in `d.networks[ip]` before this cycle), call `d.validateFn(hostname)`; only verified hostnames are appended to `d.networks[ip]` and collected into a `verifiedNew []string` slice; unverified ones are logged via `log.Warn("hostname-detect: dropped unverified candidate", "hostname", h)` and excluded from `d.networks` entirely (so they're never retried as "already known" — they'll simply be rediscovered and re-validated next cycle, which is fine since validation is cheap forward-DNS).
- After merging, if `d.waHandler != nil`, call `d.waHandler.RegisterHostname(h)` for each `h` in `verifiedNew`; if `d.certStore != nil`, call `caFile, newCerts, err := server.EnsureNetworkTLSCerts(d.networks)` (only when `d.networks` actually changed this cycle — reuse `EnsureNetworkTLSCerts`'s own `sanHash` short-circuit, so calling it every cycle is cheap when nothing changed, per stack.md).
- **`EnsureNetworkTLSCerts` error handling (deliberate, documented ordering choice):** if it returns a non-nil `err`, call `log.Error("hostname-detect: TLS cert issuance failed", "err", err)` and skip `d.certStore.Store(...)` for this cycle only — do **not** roll back the `RegisterHostname` calls already made for `verifiedNew` in this same cycle. Rolling back RPID registration mid-cycle would be worse than a transient TLS-SAN gap: `EnsureNetworkTLSCerts`'s own `sanHash` short-circuit means the very next cycle cheaply retries cert issuance for the same network set at no extra cost, so the gap self-heals, whereas un-registering an RPID a WebAuthn ceremony may already be relying on is a correctness regression with no equivalent auto-retry. This ordering is intentional, not an oversight.
- Files: `hostname_detector.go`

##### Task 2.2.1b: Update `NewHostnameDetector`'s constructor and Task 2.1.2c's call site (~4 min)
- Add `waHandler`/`certStore`/`validateFn` parameters to `NewHostnameDetector` (or a small `HostnameDetectorConfig` struct if the parameter list is getting long — prefer the struct per this repo's `primitive-obsession-checklist` skill guidance for >3 same-typed/optional params).
- Update Task 2.1.2c's call site to pass `remoteAccess.Handler`/`remoteAccess.CertStore` (nil when remote access is disabled).
- Files: `hostname_detector.go`, `main.go`

##### Task 2.2.1c: Adversarial test — the security-critical one (~5 min)
- `TestHostnameDetector_Redetect_UnverifiedHostnameNeverReachesRPIDOrTLS`: fake `resolveFn` returns a hostname; fake `validateFn` returns `false` for it; a real (or minimally faked) `*serverauth.Handler` and `*server.NetworkCertStore` are passed in; after `redetect` runs, assert the hostname is absent from `waHandler`'s rpIDs (may need a small `Handler` test-only accessor, or assert via a subsequent `webauthnForHost`-style request failing) and absent from `certStore.Load()`'s SANs.
- `TestHostnameDetector_Redetect_VerifiedHostnameReachesRPIDAndTLS`: mirror test with `validateFn` returning `true`, asserting presence instead.
- `TestHostnameDetector_Redetect_RepeatedNoOpCyclesAreStable` (covers requirements.md's idempotency Success Metric — "hostnames valid at boot remain valid after a re-detection cycle finds no change... no flapping"): with unchanged `detectFn`/`resolveFn` fakes, call `redetect` twice in a row and assert (1) `d.networks` is byte-identical after both cycles, (2) the second cycle's `redetectCycle.NewCount == PrevCount` and `Added` is empty, (3) `RegisterHostname` is not invoked again for already-registered hostnames on the second cycle — assert via a call-count on the fake `hostnameRegistrar`/`waHandler`, since `RegisterHostname` is documented as idempotent (Task 1.3.1b) but this test additionally proves `redetect` itself doesn't even re-call it for unchanged hostnames, (4) `EnsureNetworkTLSCerts` is not re-invoked with a changed SAN set on the second cycle — assert via a call-count on a fake `certPublisher`/`EnsureNetworkTLSCerts` seam if one exists (see architecture-review.md's `hostnameRegistrar`/`certPublisher` port recommendation), otherwise assert indirectly via the detector's own `redetectCycle.NewCount == PrevCount` and `Added` empty for the second cycle.
- Files: `hostname_detector_test.go`

---

## Phase 3: OS-Level Network-Change Trigger

### Epic 3.1: Adopt `tailscale.com/net/netmon`
**Goal**: Feed `HostnameDetector.events` from a real OS-level network-change signal on macOS and Linux, per build-vs-buy.md's recommendation, without hand-rolling route-socket/netlink parsing.

#### Story 3.1.1: Add the dependency and verify its cost
**As a** maintainer of a single-developer tool, **I want** to confirm `tailscale.com/net/netmon`'s transitive dependency weight before committing to it, **so that** the "minimize new dependency weight" unstated need from features.md is actually checked, not assumed.
**Acceptance Criteria**:
- `go.mod`/`go.sum` gain `tailscale.com` (pinned to a specific tagged release) and its transitive closure, and `go build ./...` succeeds.
  - *Given* `go get tailscale.com@<pinned-version>` has been run, *When* `go mod tidy` runs, *Then* `git diff go.mod go.sum` shows only the expected new entries (no unrelated version bumps to existing deps), and `go build ./...` exits 0.
**Files**: `go.mod`, `go.sum`

##### Task 3.1.1a: Add dependency, tidy, inspect (~5 min)
- Run `go get tailscale.com/net/netmon@latest` (pin to the resolved version), then `go mod tidy`. Read the resulting `go.mod`/`go.sum` diff.
- If the diff pulls in anything alarming (e.g. a huge unrelated subsystem, a license conflict), stop and re-flag Unresolved Question above rather than proceeding — this task's job is to produce evidence, not just run the command.
- Files: `go.mod`, `go.sum`

##### Task 3.1.1b: Compile-only sanity check (~2 min)
- `go build ./...` (and, if quick, `GOOS=darwin go build ./...` / `GOOS=linux go build ./...` from whichever platform this isn't already native on, per pitfalls.md's concern about the macOS path silently bit-rotting with no CI coverage — confirm `make ci`/`goreleaser-check.yml` already cross-compiles both `GOOS` values; if not, note it as a residual gap in the PR description, not a new task here since goreleaser config is out of this project's scope).
- Files: none (verification only)

#### Story 3.1.2: `NetworkChangeSource` + `tailscaleNetmonSource`
**As a** `HostnameDetector`, **I want** an events channel fed by real OS network-change notifications, **so that** redetection happens promptly on a network switch, not only on the periodic timer.
**Acceptance Criteria**:
- `tailscaleNetmonSource.RegisterChangeCallback` delivers a signal on the channel `HostnameDetector` selects on, coalesced by `netmon`'s own built-in debounce window (no separate hand-rolled debounce timer — this is the single, clearly-chosen coalescing mechanism per the requirements' Debounce Rabbit Hole).
  - *Given* a `tailscaleNetmonSource` wrapping a real `*netmon.Monitor`, *When* `mon.InjectEvent()` is called (netmon's own test-injection hook, per its public API) or a real interface change occurs, *Then* the registered callback fires at least once within netmon's coalescing window, and the wrapped `HostnameDetector.events` channel receives exactly one signal per coalesced burst (verified via a channel-receive-count assertion in a test using `netmon.NewFakeMonitor` or `InjectEvent`, not a real network change).
- `Close()` releases the underlying monitor cleanly — no goroutine leak.
  - *Given* a `tailscaleNetmonSource` has been created and its callback registered, *When* `Close()` is called, *Then* a `goleak.VerifyNone` check immediately after (with a baseline captured before creation) reports no leaked goroutines.
**Files**: `netchange_source.go` (new)

##### Task 3.1.2a: Define `NetworkChangeSource` and the netmon-backed implementation (~5 min)
- New file `netchange_source.go` (package `main`):
  ```go
  type NetworkChangeSource interface {
      RegisterChangeCallback(fn func()) (unregister func())
      Close() error
  }

  type tailscaleNetmonSource struct {
      mon *netmon.Monitor
  }

  func newTailscaleNetmonSource() (*tailscaleNetmonSource, error) {
      mon, err := netmon.New(logger.Discard) // exact logger param per netmon's actual constructor signature -- verify against the pinned version's godoc during implementation
      if err != nil {
          return nil, fmt.Errorf("create network monitor: %w", err)
      }
      return &tailscaleNetmonSource{mon: mon}, nil
  }

  func (s *tailscaleNetmonSource) RegisterChangeCallback(fn func()) (unregister func()) {
      return s.mon.RegisterChangeCallback(func(delta *netmon.ChangeDelta) { fn() })
  }

  func (s *tailscaleNetmonSource) Close() error { return s.mon.Close() }
  ```
  (Exact `netmon.New`/`RegisterChangeCallback`/`ChangeDelta` signatures must be confirmed against the pinned version's actual godoc during implementation — Story 3.1.1 pins the version first specifically so this task isn't guessing against a moving target.)
- Files: `netchange_source.go`

##### Task 3.1.2b: Wire into `main.go`'s detector construction (~4 min)
- In the `"runtime"` phase (replacing Task 2.1.2c's placeholder `netChangeEvents` channel), construct `netSource, err := newTailscaleNetmonSource()` (log+continue on error rather than failing startup — the timer arm alone still satisfies the requirement, degraded gracefully); create `events := make(chan struct{}, 1)`; register a callback that does a non-blocking send (`select { case events <- struct{}{}: default: }`, coalescing further at the channel level in case netmon's own window still bursts); register `a.OnStop`-equivalent cleanup to call `netSource.Close()` and the callback's `unregister()`.
- Files: `main.go`

##### Task 3.1.2c: Tests using netmon's fake/injection hooks (~5 min)
- `TestTailscaleNetmonSource_RegisterChangeCallback_FiresOnInjectedEvent` and `TestTailscaleNetmonSource_Close_NoGoroutineLeak` in `netchange_source_test.go`, using whichever fake/injection mechanism the pinned `netmon` version's test helpers expose (confirm during implementation; if none exists, test only `Close()`'s leak-free behavior and rely on Story 3.1.2's manual macOS verification for the event-firing behavior, explicitly noting in a code comment that CI cannot exercise the real OS event path per pitfalls.md §5).
- Files: `netchange_source_test.go`

---

## Phase 4: Observability & Operability

### Epic 4.1: Structured per-cycle logging
**Goal**: Every redetection cycle is visible in `logs/staplersquad.log`, whether or not anything changed, per features.md's "confidence the fix engaged" finding.

#### Story 4.1.1: Log every cycle
**As** Tyler debugging a network switch, **I want** to `grep` the log for "hostname-detect" and see every cycle, including a no-op one, **so that** I can confirm the mechanism is working without restarting the process to test it.
**Acceptance Criteria**:
- Every call to `redetect` ends with exactly one `log.Info("hostname-detect: cycle complete", ...)` call carrying `trigger`, `duration`, `prev_count`, `new_count`, `added`.
  - *Given* a cycle where `resolveFn` returns no new hostnames beyond what's already in `d.networks`, *When* `redetect` runs, *Then* a log line is emitted with `new_count` equal to `prev_count` and `added` empty (not suppressed).
**Files**: `hostname_detector.go`

##### Task 4.1.1a: Emit the log line from `redetect` (~3 min)
- At the end of `redetect` (after Task 2.2.1a's publish steps), call `log.Info("hostname-detect: cycle complete", "trigger", trigger, "duration", time.Since(start), "prev_count", prevCount, "new_count", newCount, "added", verifiedNew)`.
- Files: `hostname_detector.go`

##### Task 4.1.1b: Test the log content via a captured logger or return value (~4 min)
- Since `redetect` already returns a `redetectCycle` struct (Task 2.1.1b), assert its fields directly in the existing `hostname_detector_test.go` tests rather than parsing log output — add one assertion per existing test confirming `PrevCount`/`NewCount`/`Added` match expectations. No new test file needed.
- Files: `hostname_detector_test.go`

### Epic 4.2: Disable / interval / manual-trigger controls
**Goal**: The loop is trivially disableable (Risk Control) and trivially triggerable for manual verification (features.md's unstated need).

#### Story 4.2.1: `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE` / `_INTERVAL` env vars
**As an** operator, **I want** to disable or retune the redetection loop without a code change, **so that** a noisy/expensive OS-event integration discovered post-ship can be turned off immediately.
**Acceptance Criteria**:
- `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true` prevents `HostnameDetector.Run` from starting the ticker/event loop (it may still log a single "disabled" line, but performs no detection cycles).
  - *Given* the env var is set to `"true"` before process start, *When* the `"runtime"` phase runs, *Then* `a.Go("hostname-detector", ...)` either isn't started at all, or `Run` returns immediately after logging `"hostname-detect: disabled via STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE"`.
- `STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL=2m` overrides the default 5-minute ticker interval.
  - *Given* the env var is set to `"2m"`, *When* the ticker is constructed, *Then* `time.NewTicker` is called with `2 * time.Minute`, not the default.
  - *Given* the env var is set to an unparseable value (e.g. `"banana"`), *When* startup runs, *Then* the default 5-minute interval is used and a `log.Warn` is emitted (never a startup failure — this is a diagnostic knob, not a required config).
**Files**: `main.go` or `hostname_detector.go`

##### Task 4.2.1a: Add `hostnameRedetectInterval()` and disable check (~4 min)
- Small helper function (in `hostname_detector.go`): `func hostnameRedetectInterval() time.Duration` reads `STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL`, parses via `time.ParseDuration`, falls back to `defaultHostnameRedetectInterval = 5 * time.Minute` with a `log.Warn` on parse failure.
- In `main.go`'s wiring (Task 2.1.2c), check `os.Getenv("STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE") == "true"` before calling `a.Go(...)` at all; log the disabled message instead.
- Files: `hostname_detector.go`, `main.go`

##### Task 4.2.1b: Tests (~4 min)
- `TestHostnameRedetectInterval_DefaultWhenUnset`, `TestHostnameRedetectInterval_ParsesOverride`, `TestHostnameRedetectInterval_FallsBackOnParseError` using `t.Setenv` — acceptable here per `deterministic-fast-tests` since this is a pure parsing function, not a real sleep/timeout fixture.
- Files: `hostname_detector_test.go`

#### Story 4.2.2: Loopback-only manual trigger endpoint
**As** Tyler verifying the feature works without waiting 5 minutes, **I want** a way to force one redetection cycle immediately, **so that** I can confirm end-to-end behavior (log line, new hostname appearing) during manual testing.

**Scope note**: requirements.md does not explicitly ask for an HTTP trigger endpoint — this is a deliberate scope addition beyond its literal text, justified by features.md's "confidence the fix actually engaged" unstated-need framing, not a requirements-derived story. Flagged here so it's reviewed as such rather than assumed to be in-scope by default.

**Acceptance Criteria**:
- `POST /api/debug/redetect-hostnames` on the local (loopback-only) server sends a request on `HostnameDetector.manual` and returns the `redetectCycle` summary `Run`'s goroutine computed, as JSON — it never calls `redetect` directly from the handler goroutine (see Task 4.2.2a).
  - *Given* the detector is running and reachable from the handler, *When* a `POST` request arrives from `127.0.0.1`, *Then* the response body contains `{"trigger":"manual","added":[...],...}` and a matching log line appears.
- The endpoint rejects any non-loopback request, reusing the existing `isLocalhostRequest` check this codebase already applies elsewhere (per `server/auth/hostname_guard.go`'s `sourceIP` doc comment referencing it).
  - *Given* a request arrives with `RemoteAddr` `"203.0.113.9:54321"`, *When* it hits this handler, *Then* it receives `403 Forbidden` and no cycle runs.
- The endpoint respects `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true` (Story 4.2.1) — it does not silently keep working when the operator has explicitly disabled redetection.
  - *Given* `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true` was set at process start (so `Run`'s loop was never started), *When* a `POST` request arrives from `127.0.0.1`, *Then* it receives `503 Service Unavailable` and no send is attempted on `detector.manual`.
**Files**: `server/handlers.go` (or wherever `isLocalhostRequest` currently lives — confirm exact file via Grep during implementation), `main.go`

##### Task 4.2.2a: Register the route (~5 min)
- Locate `isLocalhostRequest` (Grep `server/` for its definition, referenced today only in a doc comment at `server/auth/hostname_guard.go:124`) and reuse it directly.
- Register `srv.Mux().HandleFunc("/api/debug/redetect-hostnames", ...)` in the `"runtime"` phase after the detector is constructed, closing over the `*HostnameDetector`. The handler:
  1. Returns `403 Forbidden` if `!isLocalhostRequest(r)`.
  2. Returns `503 Service Unavailable` (with a short JSON error body) if `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true` — the same env-var check Task 4.2.1a uses to skip starting `Run`'s loop. This is required, not optional: once this task routes the manual trigger through `d.manual` (below), a disabled/never-started `Run` goroutine means nothing is ever listening on that channel, so an ungated request would hang until the handler's own timeout fires instead of failing fast.
  3. Otherwise, does **not** call `detector.redetect(...)` directly (that would race `Run`'s own goroutine over the unsynchronized `d.networks` map — see the architecture review's Blocker A). Instead it creates `respCh := make(chan redetectCycle, 1)`, sends it on `detector.manual` with a context-bound send (`select { case detector.manual <- respCh: case <-r.Context().Done(): ...; case <-time.After(10*time.Second): ... }` — the 10s timeout guards against a stalled `Run` goroutine hanging the HTTP response indefinitely), then receives the resulting `redetectCycle` off `respCh` and writes it as JSON.
- Files: `main.go`

##### Task 4.2.2b: Test (~4 min)
- `TestRedetectHostnamesEndpoint_LoopbackRequestTriggersCycle` (asserts the response reflects a `redetectCycle` produced by a `Run` goroutine actually consuming `detector.manual`, not a direct `redetect` call), `TestRedetectHostnamesEndpoint_NonLoopbackRequestRejected`, `TestRedetectHostnamesEndpoint_DisabledReturns503AndDoesNotSendOnManualChannel` using `httptest.NewRequest`/`httptest.NewRecorder` with a fake `HostnameDetector` (fake `detectFn`/`resolveFn`, no real subprocess calls).
- Files: `main_test.go` (or a new `hostname_detector_endpoint_test.go` if `main_test.go` doesn't already exist — confirm via Glob)
