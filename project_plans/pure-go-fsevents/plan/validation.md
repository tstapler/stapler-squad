# Validation: pure-go-fsevents

## Adversarial review

### Challenge 1: Does the purego callback thread-safety risk make Phase 1 a blocker?

**Yes.** If `purego.NewCallback` is not safe to call from a CoreFoundation-owned thread, the
entire approach fails. The plan addresses this correctly by making Phase 1 a spike with an
explicit "pivot to dispatch shim" fallback. The `ebitengine/oto` audio package (ships in
production games) uses the same pattern — dlopen CoreAudio + purego callback on an
audio-thread-owned run loop — which is strong precedent that it works. Risk is real but
well-mitigated.

**Verdict:** Accept Phase 1 as a required gate. Do not proceed to Phase 2 until the spike
passes with `-race` on macOS.

### Challenge 2: Is `internal/fsevents` the right location, or should this be a separate module?

The plan argues for `internal/` first. Counter-argument: there is no pure-Go FSEvents binding
in the Go ecosystem; this would be genuinely useful to others. However:
- A public API adds stable-API pressure before we know the right shape.
- The item's acceptance criteria say "package builds with CGO_ENABLED=0" — not "publish to pkg.go.dev".
- If and when the API stabilises, extracting it is a one-commit move.

**Verdict:** Stay `internal/fsevents`; document the extraction path in a TODO comment.

### Challenge 3: Does the scanner integration risk breaking Linux CI?

The plan uses `scanner_fsevents_darwin.go` / `scanner_fsevents_other.go` pairs. The `other`
stub must match the interface exactly. A compile check (`CGO_ENABLED=0 GOOS=linux go build
./...`) in CI will catch any mismatch. This is already the pattern used in `statdir_unix.go`
/ `statdir_other.go`.

**Verdict:** Valid; add an explicit CI check step.

### Challenge 4: Event volume — can a stream over a large monorepo produce enough events to saturate the channel?

A 1024-event buffered channel and a drop counter are plan mitigations. The backstop tick
(unchanged) provides eventual consistency if the channel overflows. This is acceptable: the
FSEvents stream is an optimisation (lower latency on change), not the correctness path.

**Verdict:** Accept the plan's mitigation. Require spike to measure real volume before
widening the channel.

### Challenge 5: The plan defers `FSEventStreamCreateRelativeToDevice` — is this safe?

Yes. Path-based streams work for all non-NFS volumes. Device-relative streams are an
optimisation (dedup across worktrees on the same device) that can be added in Tier B.

**Verdict:** Accept deferral.

## Pre-conditions to start implementation

- [ ] macOS machine available for spike (M-series or Intel, macOS 13+)
- [ ] `go test ./...` passes on main before branching
- [ ] Spike (Phase 1) completes with `-race` clean on macOS

## Definition of done

- All 10 acceptance criteria in `requirements.md` verified by green CI on macOS runner
- Spike report written to `research/spike-event-volume.md`
- `goleak.VerifyTestMain` passes in `internal/fsevents` test suite
- Linux CI remains green (no new build-tag escapes)
- `make lint` passes with no new suppressions
