# Validation Plan: network-hostname-redetect

**Date**: 2026-09-18

## Happy Path Scenario
Given the baseline state (the process resolved `netflix1.staplerhome.com` at boot and is now running unattended after the user switches Wi-Fi networks), when a periodic or OS-event-triggered redetection cycle runs `detectLANIPs`/`resolveLANHostnames` again and `verifyHostnameOwnership` confirms a newly-resolvable hostname on the new subnet, then that hostname becomes usable — present in `Server.GetHostnames()`, registered as a trusted WebAuthn RPID via `Handler.RegisterHostname`, and included in the next TLS cert's SANs via `EnsureNetworkTLSCerts` — without a process restart.

## Requirement → Test Mapping

This project's tasks (1.1.1b, 1.2.1c, 1.3.1c, 2.1.1c, 2.2.1c, 3.1.2c, 4.1.1b, 4.2.1b, 4.2.2b) already name most of these tests in `plan.md`; this table is the authoritative cross-reference from requirement to test, not a parallel list. Tests marked `[NEW]` fill a gap plan.md's task lists left uncovered (called out explicitly below and in the summary).

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Periodic redetection | `hostname_detector_test.go` | `TestHostnameDetector_Run_StartupCycleRunsImmediately` | Integration | Startup cycle runs immediately, trigger=`TriggerStartup` (Task 2.1.1c) |
| Periodic redetection | `hostname_detector_test.go` | `TestHostnameDetector_Run_TickTriggersAnotherCycle` | Unit | Happy path — fake ticker fires, `redetect` runs again with `TriggerTimer` (Task 2.1.1c) |
| Periodic redetection | `hostname_detector_test.go` | `TestHostnameRedetectInterval_FallsBackOnParseError` | Unit | Error path — unparseable `STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL` falls back to 5m default + `log.Warn`, never fails startup (Task 4.2.1b) |
| Periodic redetection | `hostname_detector_test.go` | `TestHostnameDetector_Run_StopsOnContextCancel` | Integration | Ticker/event loop shuts down cleanly on `ctx.Done()`, no goroutine leak via `goleak` (Task 2.1.1c) |
| OS-event-triggered redetection | `hostname_detector_test.go` | `TestHostnameDetector_Run_EventTriggersAnotherCycle` | Unit | Happy path — fake event channel fires, `redetect` runs with `TriggerNetworkChange` (Task 2.1.1c) |
| OS-event-triggered redetection | `netchange_source_test.go` | `TestNewTailscaleNetmonSource_ErrorDoesNotFailStartup` `[NEW]` | Unit | Error path — `netmon.New` failing at startup logs and degrades to timer-only rather than failing the process (per Task 3.1.2b's "log+continue on error" behavior — not an explicitly named test in plan.md's task list) |
| OS-event-triggered redetection | `netchange_source_test.go` | `TestTailscaleNetmonSource_RegisterChangeCallback_FiresOnInjectedEvent` | Integration | Real `*netmon.Monitor` (via its injection/fake hook) fires the registered callback, which lands exactly one signal per coalesced burst on `HostnameDetector.events` (Task 3.1.2c) |
| Add-only WebAuthn RPID registration | `server/auth/webauthn_test.go` | `TestHandler_RegisterHostname_AddsNewRPIDAndOrigin` | Unit | Happy path — proactive registration adds hostname to `rpIDs`/`origins` (Task 1.3.1c) |
| Add-only WebAuthn RPID registration | `server/auth/webauthn_test.go` | `TestHandler_RegisterHostname_IdempotentOnRepeat` | Unit | Error/edge path — re-registering an already-registered hostname is a no-op, not a duplicate entry (Task 1.3.1c) |
| Add-only WebAuthn RPID registration | `server/auth/webauthn_test.go` | `TestHandler_RegisterHostname_SubsequentRequestSkipsHostnameValidator` | Integration | Proactive registration + reactive `webauthnForHost` request path share state: a later request for the proactively-registered hostname never triggers DNS validation (Task 1.3.1c) |
| Add-only TLS SAN registration | `server/tls_test.go` | `TestNetworkCertStore_StoreThenLoad_ReflectsUpdate` | Unit | Happy path — `Store` then `Load` reflects the newly-published cert map (Task 1.2.1c) |
| Add-only TLS SAN registration | `hostname_detector_test.go` | `TestHostnameDetector_Redetect_CertIssuanceFailureDoesNotRollbackRPID` `[NEW]` | Unit | Error path — `EnsureNetworkTLSCerts` returning an error is logged and skips `certStore.Store` for that cycle only, without rolling back RPID registrations already made in the same cycle (Story 2.2.1's 4th acceptance criterion — not among the three tests Task 2.2.1c names explicitly) |
| Add-only TLS SAN registration | `server/tls_test.go` | `TestGetCertificateByLocalAddr_ReadsLatestStore` | Integration | A live-handshake-shaped read (`GetCertificateByLocalAddr`'s returned closure) sees a runtime-added network's cert without restarting the listener (Task 1.2.1c) |
| Hostname ownership validation gate | `hostname_detector_test.go` | `TestHostnameDetector_Redetect_VerifiedHostnameReachesRPIDAndTLS` | Unit | Happy path — a hostname that passes `validateFn` reaches both `RegisterHostname` and the TLS SAN set (Task 2.2.1c) |
| Hostname ownership validation gate | `hostname_detector_test.go` | `TestHostnameDetector_Redetect_UnverifiedHostnameNeverReachesRPIDOrTLS` | Unit | Error/adversarial path — a hostname that fails `validateFn` still updates `Server.hostnames` bookkeeping but never reaches `RegisterHostname` or a TLS SAN (Task 2.2.1c, the security-critical test) |
| Hostname ownership validation gate | `main_test.go` | `TestVerifyHostnameOwnership_RejectsNonMatchingIP` `[NEW]` | Integration | `verifyHostnameOwnership`, the single extracted function shared by `startRemoteAccess`'s `hostnameValidator` and `HostnameDetector`, rejects a hostname resolving to an IP that isn't this host's own (per Story 2.1.2's acceptance-criterion example — not an explicitly named test in plan.md's task list) |
| Thread-safe `Server.hostnames` | `server/server_test.go` (or `server/server_hostnames_test.go`) | `TestServer_SetHostnames_AddOnlyMerge` | Unit | Happy path — `SetHostnames` computes the add-only union and never shrinks the set (Task 1.1.1b) |
| Thread-safe `Server.hostnames` | `server/server_test.go` (or `server/server_hostnames_test.go`) | `TestServer_SetHostnames_ConcurrentAccessIsRaceFree` | Unit | Error path (race hazard) — concurrent writer/reader goroutines under `go test -race` report no data race (Task 1.1.1b) |
| Thread-safe `Server.hostnames` | `hostname_detector_test.go` | `TestHostnameDetector_Redetect_AddOnlyMergesNetworks` | Integration | `HostnameDetector.redetect` publishes into `Server.hostnames` via `SetHostnames`, and re-resolves every known IP every cycle rather than skipping already-seen ones (Task 2.1.1c) |
| Disable/interval controls | `hostname_detector_test.go` | `TestHostnameRedetectInterval_ParsesOverride` | Unit | Happy path — `STAPLER_SQUAD_HOSTNAME_REDETECT_INTERVAL=2m` overrides the default (Task 4.2.1b) |
| Disable/interval controls | `hostname_detector_test.go` | `TestHostnameRedetectInterval_FallsBackOnParseError` | Unit | Error path — unparseable value falls back to default + warns (Task 4.2.1b; shared with Periodic redetection row above) |
| Disable/interval controls | `main_test.go`/`hostname_detector_endpoint_test.go` | `TestRedetectHostnamesEndpoint_DisabledReturns503AndDoesNotSendOnManualChannel` | Integration | `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true` (loop never started) causes the manual-trigger endpoint to fail fast with 503 rather than hang on an unconsumed channel (Task 4.2.2b) |
| Idempotency / no-flap | `hostname_detector_test.go` | `TestHostnameDetector_Redetect_RepeatedNoOpCyclesAreStable` | Unit | Happy path — two consecutive no-change cycles leave `d.networks` byte-identical, `NewCount == PrevCount`, `Added` empty, and downstream `RegisterHostname`/`EnsureNetworkTLSCerts` are not redundantly re-invoked (Task 2.2.1c) |
| Idempotency / no-flap | `hostname_detector_test.go` | `TestHostnameDetector_Redetect_AddOnlyMergesNetworks` | Integration | A transient per-IP resolution failure on first sighting doesn't permanently freeze that IP's discovery — re-resolved and merged add-only on a later cycle (Task 2.1.1c; shared with Thread-safe `Server.hostnames` row above) |
| Manual trigger endpoint | `main_test.go`/`hostname_detector_endpoint_test.go` | `TestRedetectHostnamesEndpoint_LoopbackRequestTriggersCycle` | Unit | Happy path — a loopback `POST` request sends on `detector.manual` and returns the resulting `redetectCycle` as JSON (Task 4.2.2b) |
| Manual trigger endpoint | `main_test.go`/`hostname_detector_endpoint_test.go` | `TestRedetectHostnamesEndpoint_NonLoopbackRequestRejected` | Unit | Error path — a non-loopback `RemoteAddr` gets `403 Forbidden`, no cycle runs (Task 4.2.2b) |
| Manual trigger endpoint | `main_test.go`/`hostname_detector_endpoint_test.go` | `TestRedetectHostnamesEndpoint_DisabledReturns503AndDoesNotSendOnManualChannel` | Integration | The disabled-loop interaction described above (Task 4.2.2b; shared with Disable/interval controls row above) |

## UX Acceptance Tests
Omitted — this is a pure infrastructure project (no `design/ux.md`, no user-facing surface; the one HTTP endpoint added, `/api/debug/redetect-hostnames`, is a loopback-only debug/manual-trigger affordance, not a UX surface).

## Test Stack
- **Unit**: Go's standard `testing` package, table-driven where the plan calls for it, run via `gotestsum` per this repo's Makefile convention (`go test`-style `TestXxx` names/signatures underneath). `goleak.VerifyNone` for every test that starts a goroutine (`HostnameDetector.Run`, `tailscaleNetmonSource`). Fake `detectFn`/`resolveFn`/`tick`/`events`/`validateFn` injected per this repo's `deterministic-fast-tests` skill — no real subprocess calls, no real sleeps.
- **Integration**: same `testing` package, but exercising two or more real (non-faked) collaborators together — e.g. a real `serverauth.Handler` + `HostnameDetector`, a real `NetworkCertStore` + `GetCertificateByLocalAddr` closure, or a real `*netmon.Monitor` via its own fake/injection hook (`netmon.NewFakeMonitor`/`InjectEvent`) rather than a hand-rolled fake of the whole `NetworkChangeSource` interface. `httptest.NewRequest`/`httptest.NewRecorder` for the manual-trigger endpoint tests.
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% branch coverage on `redetect`'s validation-gate `if d.validateFn(...)` and `if d.waHandler != nil`/`if d.certStore != nil` branches specifically (security-critical per Epic 2.2) |

- All public service methods (`HostnameDetector.Run`, `Handler.RegisterHostname`, `NetworkCertStore.Load`/`Store`, `verifyHostnameOwnership`): happy path + error paths covered.
- All external integrations (`tailscale.com/net/netmon`, the OS subprocess calls inside `detectLANIPs`/`resolveLANHostnames`): unit mocked (fake `detectFn`/`resolveFn`/`NetworkChangeSource`) + at least one integration test using the real dependency's own test-injection hooks (`netmon.NewFakeMonitor`/`InjectEvent`).

## Migration Plan
N/A — no schema or persisted-data changes (per requirements.md's Risk Control: "no data migration, no persisted state format change").
