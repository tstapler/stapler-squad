# Adversarial Review: network-hostname-redetect (re-review pass)

**Date**: 2026-09-18
**Verdict**: CONCERNS

## Previously-Blocked Items Re-Checked

- [x] Blocker 1 (IP-keyed permanent freeze) — RESOLVED — Story 2.1.1's acceptance criteria now state: "`resolveFn` is re-run for **every** currently-detected IP on every cycle, not only newly-seen ones — an IP already present as a key in `networks` is never permanently skipped, so a transient resolution failure on first sighting cannot freeze discovery for that IP forever" (plan.md:243), with a matching Given/When/Then at plan.md:244-245. Task 2.1.1b's `redetect` spec now reads: "for **every** currently-detected IP (not just newly-seen ones...), call `d.resolveFn(ip)` and add-only-merge any newly-found hostnames... ('already a key' means 'known, keep re-resolving,' never 'resolved once, skip forever'...)" (plan.md:258). This matches the fix agent's claim exactly.

- [x] Blocker 2 (TLS cert error path) — RESOLVED — Task 2.2.1a now has an explicit subsection: "**`EnsureNetworkTLSCerts` error handling (deliberate, documented ordering choice):** if it returns a non-nil `err`, call `log.Error("hostname-detect: TLS cert issuance failed", "err", err)` and skip `d.certStore.Store(...)` for this cycle only — do **not** roll back the `RegisterHostname` calls already made... `EnsureNetworkTLSCerts`'s own `sanHash` short-circuit means the very next cycle cheaply retries cert issuance for the same network set at no extra cost, so the gap self-heals" (plan.md:314). This is present as claimed.
  - Sanity-checked the "self-heals via sanHash" claim against the actual implementation (`server/tls.go:58-121`): the per-network hash file (`hashFile`) is only written *after* a successful `generateServerCert`/`os.WriteFile` (tls.go:103-111), so a failed cycle never persists the new "want" hash. `certCurrent(certFile, hashFile, want)` (tls.go:87) therefore correctly evaluates false on the next cycle and forces a real reissuance attempt again — the retry is genuine, not a false-positive short-circuit that would silently give up. The mechanism is coherent with the add-only design elsewhere.
  - CONCERN (not a blocker, matches the fix agent's own framing): if cert issuance keeps failing on every cycle (e.g. persistent disk-full, CA regen failure), a verified hostname stays permanently trusted as a WebAuthn RPID with no covering TLS SAN — there is no cap on how many cycles this can persist, and no alert (only a repeated `log.Error` per Observability Plan, which explicitly has no metrics/alerts per plan.md:60-61). This is a real, indefinite-duration gap, but it is self-announcing (grep-able log line every cycle) and the underlying failure mode is rare/operator-visible rather than silent — consistent with the fix agent's own characterization. Worth one added sentence in the plan noting this residual risk explicitly, but not blocking.

- [x] Blocker 3 (idempotency test) — RESOLVED — Task 2.2.1c adds `TestHostnameDetector_Redetect_RepeatedNoOpCyclesAreStable`, explicitly described as covering "requirements.md's idempotency Success Metric," asserting: "(1) `d.networks` is byte-identical after both cycles, (2) the second cycle's `redetectCycle.NewCount == PrevCount` and `Added` is empty, (3) `RegisterHostname` is not invoked again for already-registered hostnames on the second cycle... (4) `EnsureNetworkTLSCerts` is not re-invoked with a changed SAN set on the second cycle" (plan.md:325). All four assertions the fix agent claimed are present verbatim.

## Related Concerns Re-Checked

- Manual-trigger endpoint gated on `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE` (503 when disabled) — VERIFIED FIXED. Task 4.2.2a's handler spec: "Returns `503 Service Unavailable` (with a short JSON error body) if `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true`... This is required, not optional: once this task routes the manual trigger through `d.manual`..., a disabled/never-started `Run` goroutine means nothing is ever listening on that channel, so an ungated request would hang until the handler's own timeout fires instead of failing fast" (plan.md:459). Story 4.2.2's acceptance criteria also codify this with a Given/When/Then expecting `503 Service Unavailable` and "no send is attempted on `detector.manual`" (plan.md:451-452). A corresponding test, `TestRedetectHostnamesEndpoint_DisabledReturns503AndDoesNotSendOnManualChannel`, is listed in Task 4.2.2b (plan.md:464).

- Story 4.2.2 has a scope note acknowledging it's a deliberate addition beyond requirements.md — VERIFIED FIXED. Immediately under the Story 4.2.2 heading: "**Scope note**: requirements.md does not explicitly ask for an HTTP trigger endpoint — this is a deliberate scope addition beyond its literal text, justified by features.md's 'confidence the fix actually engaged' unstated-need framing, not a requirements-derived story. Flagged here so it's reviewed as such rather than assumed to be in-scope by default." (plan.md:444).

## New Issues Found During Re-Check

None rising to BLOCKER or CONCERN beyond the one already logged above (the indefinite-cert-failure-gap point, folded into Blocker 2's re-check rather than listed separately since it's the same finding the task asked to sanity-check).

## Carried-Forward Concerns (unchanged, not re-reviewed this pass)

From the prior review's **Concerns** section (the first two of six were re-checked above and confirmed fixed; the remaining four were not re-reviewed this pass):
- Netmon's coalescing is the sole debounce mechanism, and the plan's own test task (3.1.2c) admits it may ship with zero CI verification of the event-firing path if the pinned `netmon` version exposes no fake/injection hooks.
- No context threading into the actual detection work (`detectFn`/`resolveFn` take no `ctx`) — shutdown latency is bounded only indirectly, via each subprocess's own internal timeout, not the outer `SIGTERM`-driven context.
- Unclear whether `newTailscaleNetmonSource()` construction/cleanup respects the disable flag, or whether a monitor gets constructed and left running with its callback registered but never consumed when the feature is turned off.
- `Server.hostnames`/`GetHostnames()` has exactly one production reader (`main.go:1280`, inside `startRemoteAccess`, which runs once at boot before the detector even exists) — post-boot, nothing in the running process ever reads it again, so Epic 1.1's atomic-publish work currently has no live read consumer.
- Asymmetric TLS SAN trust model between boot and runtime: boot-time `EnsureNetworkTLSCerts` is fed an unfiltered `networks` map with no `verifyHostnameOwnership` gate, while Epic 2.2 introduces that gate only for the runtime detector's post-boot discoveries — the plan doesn't call out this before/after inconsistency for the same SAN surface.

From the prior review's **Minors** section (not re-reviewed this pass):
- The `tailscale.com/net/netmon` go.sum-diff Unresolved Question is self-graded by the same task meant to gate it — proportionate for a single-developer tool, but no second reviewer or objective "alarming" threshold.
- No independent kill-switch for just the OS-event (netmon) arm — `STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE` disables both timer and event paths together.
- Confirm `redetectCycle.Added`'s semantics stay consistent between Task 2.1.1b (defined before validation exists) and Task 2.2.1a's `verifiedNew` (post-validation), so the log line's `added` field always means "verified and actually registered," not "raw candidate."
