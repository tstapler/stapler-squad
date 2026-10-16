# ADR-001: Fetch a prebuilt `tymuxd` binary from GitHub Releases, not compile-from-submodule

**Status**: Accepted
**Date**: 2026-08-25

## Context

`tymuxd` is a Rust daemon from the sibling repo `tstapler/tymux`. Today an operator must
`cargo build` it out-of-band before the tymux backend (`session/backend_tymux.go`) is usable
at all. This project needs to bundle/ship `tymuxd` the way `.claude/docs/bundling-tmux.md`
bundles tmux: `git submodule` + `make build-tmux` + `go:embed`. tmux is compiled from C source
via autotools against system libs already present via Homebrew/apt on every platform this repo
builds for — a cheap (~30s), well-trodden compile with no simpler alternative (nobody publishes
prebuilt tmux 3.4 binaries for this repo's exact platform matrix).

`tymuxd` is different. It's a 5-crate Cargo workspace (`tokio`, `tonic`/`prost`, `portable-pty`,
`vt100`) requiring a Rust toolchain (`cargo`/`rustc`) this repo has never needed anywhere —
not in CI, not in `make install-tools`, not in developer onboarding. `tymux`'s own CI
(`ci.yml`'s `musl-build` job, `release.yml`'s macOS cross-linking) has already solved
non-trivial cross-compilation problems (musl target for Linux, `aarch64-apple-darwin` +
cross-linked `x86_64-apple-darwin` from the same host — Intel macOS runners were tried and
queued 50+ minutes with zero progress, a documented "real capacity/deprecation issue" in
`release.yml`'s own comments). `tymux` already publishes the result: `v1.0.0`
(https://github.com/tstapler/tymux/releases/tag/v1.0.0) ships four platform tarballs
(`aarch64-apple-darwin`, `x86_64-apple-darwin`, `x86_64-unknown-linux-musl`,
`aarch64-unknown-linux-musl`), each bundling both `tymuxd` and the `tymux` CLI, built via
`taiki-e/upload-rust-binary-action`.

## Decision

Bundle `tymuxd` by **fetching the prebuilt release tarball at build time**, not by adding a
`third_party/tymux` submodule and compiling from source.

- `scripts/fetch-tymuxd.sh` (new): resolves `$(uname -s)-$(uname -m)` → the matching release
  target triple, downloads `tymux-<target>.tar.gz` from
  `https://github.com/tstapler/tymux/releases/download/${TYMUX_VERSION}/`, verifies its
  SHA-256 against a pinned value checked into this repo (`scripts/tymuxd-checksums.txt`,
  one line per `<version> <target> <sha256>` — see Consequences), then extracts `tymuxd` to
  `session/tymux/embed/tymuxd`.
- `TYMUX_VERSION` pins the release the same way `.gitmodules`' `branch = 3.4` pins tmux — a
  single version string bumped deliberately, defaulting to `v1.0.0`.
- `make build-tymuxd-embed` wraps the script (mirrors `make build-tmux-embed`); `go:embed` under
  a new `embed_tymux` build tag reads `session/tymux/embed/tymuxd` at Go compile time, exactly
  like `session/tmux/binary_embedded.go` does for tmux.
- No new CI job, no new toolchain, no cross-compilation matrix in this repo — `curl` + `tar`,
  already present everywhere this repo builds.
- **Windows is out of scope for the tymux backend.** `tymux`'s release matrix has no Windows
  target. Until upstream adds one, a Windows `stapler-squad` build skips the `embed_tymux` step
  entirely and the tymux backend is simply unavailable there — a scope decision, not a defect.

## Consequences

- **Positive**: zero new toolchain requirement anywhere (dev laptops or CI); `tymux`'s own CI
  already absorbed the cross-compilation cost; fetch is O(seconds), dominated by network
  latency, versus a multi-minute cold Cargo build even with `Swatinem/rust-cache@v2`.
- **New supply-chain surface, mitigated**: unlike tmux's embed (bytes already compiled into the
  Go binary at `go build` time with no separate integrity check), a fetched binary needs its own
  checksum verification. `tymux`'s `release.yml` does not currently publish a `.sha256` per
  asset (`taiki-e/upload-rust-binary-action` supports a `checksum:` input but it's unset).
  Mitigation: pin the SHA-256 for each `(version, target)` pair we consume in
  `scripts/tymuxd-checksums.txt`, computed once at implementation time via
  `gh api repos/tstapler/tymux/releases/tags/v1.0.0` + downloading and hashing each asset, and
  fail the fetch script loudly on any mismatch. A follow-up upstream PR to `tstapler/tymux`
  adding `checksum: sha256` to `release.yml` is a cheap, separately-scoped improvement (not a
  blocker for this decision) that would let future versions self-verify without a hand-pinned
  checksum file.
- **Version-pin discipline needed across two repos**: `session/tymux/transport.go` already
  imports generated gRPC types from `github.com/tstapler/tymux/clients/go/gen/tymux/v1`, pinned
  independently via `go.mod`. `TYMUX_VERSION` (the binary pin) and the `go.mod` require (the
  client pin) must be bumped together — nothing enforces this automatically today. The
  supervision code's health check (Phase 2 of the implementation plan) partially compensates by
  verifying the daemon answers `ListSessions` correctly at startup, but does not detect a
  genuine RPC-shape mismatch; documented as an accepted gap.
- **macOS codesigning is a separate, empirically-verified-first question**: `tymuxd` extracted
  to disk and `exec`'d at runtime could hit Gatekeeper on a machine where it isn't already
  trusted, independent of anything stapler-squad's own `StaplerSquadDev` self-signed cert
  (`.claude/docs/codesigning.md`) addresses. The implementation plan's Epic 1.0 runs a spike
  (download, extract, and `exec` a real v1.0.0 release binary on macOS) *before* Epic 1.1/1.2's
  fetch-script/embed/Makefile plumbing is built out, specifically so this question is answered
  early rather than discovered late; if Gatekeeper blocks it, Task 1.0.1b records the chosen
  fallback (ad-hoc codesign at fetch time, or macOS-only local compilation) directly in this
  ADR's Consequences.

## Alternatives Considered

**Submodule + `cargo build` (mirroring tmux exactly).** Rejected. Costs: `rustup`/`cargo` on
every dev machine and CI runner building `embed_tymux`; `protoc` (tonic/prost); a new
per-platform build matrix leg in `.github/workflows/build.yml` (today `linux/darwin/windows ×
amd64/arm64`, none with a Rust toolchain); musl cross-linking needing
`taiki-e/setup-cross-toolchain-action`-equivalent tooling with no simple non-CI analogue; a
cold 5-crate workspace build realistically several minutes versus tmux's ~30s C compile. tmux
is source-compiled because there is no simpler option for its exact platform matrix; `tymux` is
different because its own maintainer already publishes exactly the artifact this project needs.

**Vendor the prebuilt tarballs directly into this repo's own git history**, as a third option
between "compile from submodule" and "fetch from GitHub Releases at build/CI time" — i.e. commit
the four pinned `v1.0.0` platform tarballs (or extracted binaries) into `stapler-squad` itself
instead of fetching them from `tstapler/tymux`'s releases at build time. This would remove the
runtime/build-time dependency on `tstapler/tymux`'s release artifacts continuing to exist and
stay reachable — the real "technology bet" risk in this project, since `tstapler/tymux` is a
single-maintainer personal repo with no mirror and no availability SLA, unlike a package
registry. Rejected for now: ~14MB of repo growth per pinned version across the four platform
targets (a rough estimate — each tarball bundles both `tymuxd` and the `tymux` CLI at native-code
sizes), multiplied every time `TYMUX_VERSION` is bumped, permanently inflating `git clone` size
for a binary artifact `git`'s delta-compression can't meaningfully shrink (unlike source diffs).
The fetch-at-build-time approach's checksum verification (`scripts/tymuxd-checksums.txt`)
already closes the *integrity* half of the risk this would address; it does not close the
*availability* half (a deleted or renamed upstream release breaks the fetch). If
`tstapler/tymux`'s releases prove unreliable in practice, vendoring is the natural fallback to
revisit — noted here so that risk isn't undiscussed, even though it isn't adopted today.
