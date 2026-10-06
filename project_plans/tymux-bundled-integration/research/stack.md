# Research: Bundling Stack for tymuxd

Scope: technology-stack research only (SDD Phase 2). Answers the "Open Questions" in
`../requirements.md`. No code changes made.

## (a) How tmux's embed/bundle mechanism works today

Three-stage pipeline, entirely Makefile + `go:embed`, no third-party Go embedding library:

1. **`git submodule update --init third_party/tmux`** — pins tmux at tag `3.4` via
   `.gitmodules` (`[submodule "third_party/tmux"] url = https://github.com/tmux/tmux.git
   branch = 3.4`).
2. **`make build-tmux`** → `scripts/build-tmux.sh` — compiles tmux from C source with
   autotools (`./configure && make -j$(nproc)`), ~30s. It does *not* run
   `third_party/tmux`'s own build system as a submodule op; it downloads the matching
   version's release tarball to get pre-generated `configure`/`Makefile.in` (avoiding an
   `autogen.sh`/`libtoolize` hang seen on macOS) and layers it over the submodule checkout
   with `cp -rn` (no-clobber, so git-tracked submodule files win). Output: `bin/tmux`.
   Requires system C toolchain + `libevent`/`ncurses`/`pkg-config`/`automake` (auto-installed
   via Homebrew on macOS; must already be present via apt on Linux — the script `err`s out
   with an install command otherwise, it does not install for you on Linux).
3. **`make build-tmux-embed`** (Makefile:274-277) — depends on `build-tmux`, then
   `cp $(BIN_TMUX) session/tmux/embed/tmux`. This populates the file that
   `session/tmux/binary_embedded.go:22`'s `//go:embed embed/tmux` directive reads at
   **Go compile time** — a plain stdlib `embed.FS`-style directive (`_ "embed"` import,
   `//go:embed embed/tmux` on a `var tmuxEmbedded []byte`), gated by the `embed_tmux` build
   tag (`//go:build embed_tmux` at the top of the file). No non-tagged build ever attempts
   to embed anything — `session/tmux/binary.go` (untagged) presumably resolves the tmux
   binary via `TMUX_BIN` env var / `PATH` lookup instead (not read in full here; out of
   scope — the tagged/untagged split is the relevant fact).
4. **`make build-embedded`** (Makefile:279-288) — depends on `build-tmux-embed`, then
   `go build -tags embed_tmux -ldflags "$(LDFLAGS)" -o stapler-squad .`. On Darwin it also
   `sectcreate`s an `Info.plist` into `__TEXT` (TCC entitlement plumbing, irrelevant to
   tymuxd).
5. **Runtime extraction** (`session/tmux/binary_embedded.go:34-71`, function `Binary()`):
   `TMUX_BIN` env var still overrides everything (dev/test escape hatch). Otherwise, on
   first call (`sync.Once`), the embedded bytes are written to
   `$UserCacheDir/stapler-squad/tmux/$GOOS_$GOARCH/tmux` (0755), skipping the rewrite if a
   file of identical length already exists there (best-effort content check, not a hash).
   On any extraction error, it falls back to the bare string `"tmux"` (rely on `PATH`)
   rather than crashing.

So the "mechanism" is: **compile-from-source into a well-known file path → `go:embed` that
exact file into the binary at Go compile time → extract-once-to-cache-dir-at-runtime**. There
is no goreleaser, no `packr`/`statik`, no custom embed tooling — just stdlib `embed` plus a
Makefile pipeline that guarantees the embedded file exists before `go build` runs.

## (b) Does this transfer to a Rust binary?

**The extraction/runtime half transfers unchanged.** `go:embed` doesn't care what the bytes
are — a compiled Rust binary embeds exactly like a compiled C binary. `binary_embedded.go`'s
`Binary()`/`extractEmbeddedTmux()` pattern (cache-dir extraction, once, with a length-check
skip and a `TMUX_BIN`-style env override) is directly reusable as `TymuxdBinary()` /
`extractEmbeddedTymuxd()` with a `TYMUXD_BIN`-style override (there's already a `TYMUXD_ADDR`
env var for the *daemon address*; a binary-path override would be a new, distinct var —
watch primitive-obsession here: don't let a single env var try to mean both "path to exec"
and "address to dial").

**The "populate the embed source file" half does *not* transfer as a submodule-compile.**
tmux is C compiled with autotools against system libs (`libevent`, `ncurses`) already
available via Homebrew/apt on every dev/CI platform — a *cheap*, well-trodden compile.
tymuxd is a 5-crate Cargo workspace (`tokio`, `tonic`/`prost` for gRPC, `portable-pty`,
`vt100`) requiring a Rust toolchain this repo has never needed. Two of tymux's own CI legs
exist specifically to prove this is nontrivial: `musl-build` (cross-linking for
`aarch64-unknown-linux-musl` needs `taiki-e/setup-cross-toolchain-action`, not just
`rustup target add`) and the Darwin release legs run only on `macos-latest`
(`aarch64-apple-darwin` native + `x86_64-apple-darwin` cross-linked from the same arm64
host) — `macos-13` Intel runners were tried and **queued 50+ minutes with zero progress**,
called out in `release.yml`'s own comments as "a real capacity/deprecation issue, not
flakiness." Building tymuxd from source inside stapler-squad's own CI would require
reproducing this cross-compilation matrix knowledge, not just adding `cargo build`.

**Verdict:** the *embed+extract* mechanism transfers; the *source-compile* stage should not
be copy-pasted — tymux's own CI has already solved cross-compilation and now publishes the
result as GitHub Release binaries (see (c)/(d)/(e) below), which is strictly cheaper to
consume than to re-solve.

## (c) Submodule-compile-from-source approach, adapted for cargo

Sketch, for completeness / cost comparison — **not the recommendation** (see (e)):

```gitmodules
[submodule "third_party/tymux"]
	path = third_party/tymux
	url = https://github.com/tstapler/tymux.git
	branch = main   # or a pinned tag once tymux starts cutting stable tags regularly
```

```makefile
build-tymuxd: ## Build pinned tymuxd binary from third_party/tymux submodule
	@./scripts/build-tymuxd.sh

build-tymuxd-embed: build-tymuxd
	@mkdir -p session/tymux/embed
	@cp $(BIN_TYMUXD) session/tymux/embed/tymuxd

build-embedded: build-tmux-embed build-tymuxd-embed
	go build -tags "embed_tmux embed_tymuxd" -ldflags "$(LDFLAGS)" -o stapler-squad .
```

`scripts/build-tymuxd.sh` would need: `rustup`/`cargo` present (or install it, mirroring
`build-tmux.sh`'s Homebrew auto-install for missing C deps — but there's no Homebrew
equivalent one-liner for "the right pinned Rust toolchain"; `rustup-init.sh` is the
closest, and it's a fair amount of new shell-script surface), `protoc` (tymux's own CI
installs via `arduino/setup-protoc@v3` for its tonic/prost build), then
`cargo build --release --bin tymuxd` inside `third_party/tymux/crates/tymuxd` (or workspace
root, then locate the binary in `target/release/`). Cross-compiling for a *different*
target than the host (e.g. building an `aarch64-unknown-linux-musl` tymuxd on an
`x86_64` dev laptop to embed into a Linux/arm64 stapler-squad release) needs
`taiki-e/setup-cross-toolchain-action`-equivalent tooling that has no simple non-CI
analogue — a real gap versus tmux's C toolchain, which cross-compiles far more casually.

**Cost this imposes:** every contributor doing an `embed_tymuxd` build needs a working Rust
toolchain locally (not just CI); `make build-tymuxd` is O(minutes) not O(30s) for a cold
cargo build of a 5-crate workspace with `tokio`/`tonic`; CI needs a new job (`dtolnay/rust-toolchain`
+ `Swatinem/rust-cache@v2`, mirroring tymux's own `ci.yml`) for every platform stapler-squad
ships (see (f)).

## (d) Prebuilt-binary-download alternative — concrete sketch

**tymux already publishes exactly this.** `gh api repos/tstapler/tymux/releases/latest` (run
during this research, 2026-08-25) shows a `v1.0.0` "Latest" release
(https://github.com/tstapler/tymux/releases/tag/v1.0.0) with four asset tarballs, each
bundling *both* `tymuxd` and the `tymux` CLI binary (per `release.yml`'s
`taiki-e/upload-rust-binary-action` step: `bin: tymuxd,tymux`, `archive: tymux-$target`):

| Asset | Size |
|---|---|
| `tymux-aarch64-apple-darwin.tar.gz` | 3.25 MB |
| `tymux-x86_64-apple-darwin.tar.gz` | 3.31 MB |
| `tymux-x86_64-unknown-linux-musl.tar.gz` | 3.80 MB |
| `tymux-aarch64-unknown-linux-musl.tar.gz` | 3.75 MB |

These are built by tymux's own `release.yml` (`.github/workflows/release.yml` in
`~/Programming/tymux`, triggered on `push: tags: ["v*"]`), using `dtolnay/rust-toolchain@stable`
+ `taiki-e/create-gh-release-action` + `taiki-e/upload-rust-binary-action` — a pattern common
across the Go-tool ecosystem too (`goreleaser` does the same job when the *consumer* is a Go
project publishing its own prebuilt binaries; here tymux is the upstream doing it, so
stapler-squad only needs to be a *consumer* of an existing release pipeline, not build one).

Sketch of a `scripts/fetch-tymuxd.sh` (no cargo/rustc dependency at all):

```bash
#!/usr/bin/env bash
set -euo pipefail
VERSION="${TYMUX_VERSION:-v1.0.0}"   # pin like third_party/tmux pins branch 3.4
case "$(uname -s)-$(uname -m)" in
  Darwin-arm64)  TARGET=aarch64-apple-darwin ;;
  Darwin-x86_64) TARGET=x86_64-apple-darwin ;;
  Linux-x86_64)  TARGET=x86_64-unknown-linux-musl ;;
  Linux-aarch64) TARGET=aarch64-unknown-linux-musl ;;
  *) echo "no prebuilt tymuxd for $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac
URL="https://github.com/tstapler/tymux/releases/download/${VERSION}/tymux-${TARGET}.tar.gz"
curl -fsSL -o /tmp/tymux.tar.gz "$URL"
# checksum verify (release.yml would need to also publish a .sha256 — it does not
# today; see Recommendation below for what to add upstream) before extracting
tar xzf /tmp/tymux.tar.gz -C session/tymux/embed tymuxd
chmod +x session/tymux/embed/tymuxd
```

`make build-tymuxd-embed` becomes a thin wrapper calling this script instead of `cargo
build`. No Rust toolchain, no `protoc`, no cross-linker setup, no new CI runner matrix — just
`curl` + `tar`, which every existing CI runner and dev machine already has.

**Gap found:** tymux's `release.yml` does not currently publish a checksum file
(`sha256sum.txt` or per-asset `.sha256`) alongside the tarballs — `taiki-e/upload-rust-binary-action`
supports a `checksum:` input but it isn't set. Verified by inspecting the actual asset list
above (`gh api repos/tstapler/tymux/releases/latest --jq '.assets[].name'`, 2026-08-25) — no
`.sha256`/`.sig` entries present. Pinning to a specific tag+asset-name is still safe against
*tag* tampering (a tag's git ref is immutable once other refs point at it, and GitHub Release
assets are content-addressed by upload, not mutable-by-name in the normal case), but there's
no cryptographic verification today. If this approach is chosen, filing a small upstream
`tstapler/tymux` change to add `checksum: sha256` to the release workflow is a cheap
prerequisite, not a blocker — the plan phase should decide whether to gate on it.

**No Windows target.** stapler-squad's own release matrix
(`.github/workflows/build.yml:592-597`) ships `goos: [linux, darwin, windows]` × `goarch:
[amd64, arm64]` (windows/arm64 excluded). tymux's release matrix has **no Windows leg at
all** — only the four Unix targets above. This is a real gap for *either* bundling approach
(source-compile or prebuilt-download), not specific to one: a Windows stapler-squad build
today would ship with no tymuxd to embed, meaning the tymux backend must remain
unavailable/disabled on Windows regardless of which mechanism is chosen, until tymux's own
release matrix grows a Windows target. Flag this for the plan phase — it's a scope decision
("tymux backend is Unix-only for now"), not a technical blocker to solve here.

## (e) Recommendation

**Prebuilt-binary-download (d), not submodule-compile-from-source (c).**

Reasoning:
1. **tymux already does the hard part.** Its own CI (`ci.yml`'s `musl-build` job, and
   `release.yml`'s macOS-cross-linking comment about the `macos-13` runner-queueing dead
   end) has already solved the exact cross-compilation problems that (c) would make
   stapler-squad re-solve. Consuming the output is strictly cheaper than re-deriving the
   toolchain knowledge.
2. **Zero new toolchain requirement.** (d) needs `curl`/`tar`, already present everywhere
   this repo builds. (c) needs `cargo`/`rustc` on every machine that runs an `embed_tymuxd`
   build — dev laptops and CI both — which is exactly the cost the requirements doc flags as
   the open question to resolve, and (c) is the answer that imposes it.
3. **tmux's own precedent argues for its choice, not against alternatives.** tmux is
   source-compiled because there is *no simpler option* — nobody publishes prebuilt tmux
   3.4 binaries for stapler-squad's exact platform matrix with a stable download URL. tymux
   is different: its maintainer (the same person, in this case) already ships exactly that.
   The constraint in the requirements doc ("tmux has no simpler option... research must
   surface what [(c)] costs, and evaluate whether [(d)] avoids that cost") is answered:
   yes, decisively, because the prebuilt path already exists and needs no new work upstream
   beyond an optional checksum addition.
4. **Versioning is still explicit and reproducible.** Pinning `TYMUX_VERSION=v1.0.0` in the
   fetch script is the same "pinned dependency" discipline as `third_party/tmux`'s `branch
   = 3.4` gitlink — just resolved via a release tag instead of a submodule SHA.

The one piece of new work this recommendation creates (upstream checksum verification) is
small and optional-but-advisable; it does not change the recommendation.

## (f) CI / toolchain cost analysis

**If (c) submodule-compile were chosen:**
- New required action: `dtolnay/rust-toolchain@stable` (or `actions-rs/toolchain`, now
  largely superseded by `dtolnay/rust-toolchain` — tymux's own CI already uses the latter,
  so mirroring that choice avoids introducing a second, less-maintained action).
- New required action for musl cross: `taiki-e/setup-cross-toolchain-action@v1` (Linux
  arm64 leg specifically).
- `cargo build --workspace` cost in tymux's own CI is not separately timed in the workflow,
  but a 5-crate `tokio`/`tonic`/`portable-pty` workspace cold-build is realistically several
  minutes even with `Swatinem/rust-cache@v2` warming subsequent runs — materially more than
  tmux's ~30s C compile.
- Would need a **new build matrix leg per platform** stapler-squad ships
  (`build.yml:592-597`'s `linux/darwin/windows × amd64/arm64`), each needing the Rust
  toolchain + protoc + (for musl) the cross-linker step — a nontrivial expansion of
  `build.yml`, not a one-line addition.
- Every contributor who wants to build `embed_tymuxd` locally needs `rustup` installed —
  a new onboarding step this Go-centric repo has never required (`make install-tools`
  today only installs Go-ecosystem tools per `Makefile`/`CLAUDE.md`).

**If (d) prebuilt-download is chosen:**
- No new GitHub Action, no new toolchain install step anywhere.
- `curl -fsSL <url> | tar xz` is a few seconds per platform, dominated by network latency,
  not compute — negligible versus tmux's ~30s compile, let alone a Rust cold-build.
- The existing `build.yml` matrix's `windows` legs simply skip the embed step (or build
  without `embed_tymuxd` for now, see (d)'s Windows-gap note) — no new matrix dimension
  needed, only a conditional skip.
- Onboarding cost for a contributor: none beyond what tmux's `bin/` cache pattern already
  requires (a writable temp/cache dir + network access to `github.com` at build time,
  exactly like `scripts/build-tmux.sh`'s own tarball-download fallback path already
  requires today).

(d) is strictly cheaper on every axis measured. This directly answers the Open Question
"Should CI gain a new cargo/rustc-toolchain job, or can the bundling approach avoid that
entirely" — yes, it can be avoided entirely.

## (g) Health-check / RPC surface tymuxd exposes today

**No dedicated health/ping RPC exists.** `proto/tymux/v1/tymux.proto` (in
`~/Programming/tymux`) defines `TymuxService` with 11 RPCs — `CreateSession`,
`ListSessions`, `KillSession`, `ReviveSession`, `CapturePane`, `SearchScrollback`,
`SplitPane`, `ClosePane`, `CreateWindow`, `WatchWindow` (server-streaming), `Attach`
(bidi-streaming) — grepped for `Health`/`Ping`/`health`/`ping` across the whole `.proto`
file and found none (matches were only in unrelated doc-comment prose like "programmatic
callers" and "ANSI-scraping"). There is no gRPC standard health-checking protocol
(`grpc.health.v1.Health`) wired up either — not present in `Cargo.toml`'s dependency list
(`tonic`, `prost`, no `tonic-health`).

**Closest usable substitute:** a cheap unary call — `ListSessions` with no filter is the
natural choice (mirrors `session/tmux/tmux.go`'s own pattern of using an existence/listing
check, e.g. `TmuxServerRegistry.SessionExists`/`registryConfirmsExists`, as its liveness
signal rather than a dedicated ping). A successful `ListSessions` response (even an empty
list) proves: the process is up, listening on the expected loopback port, and correctly
speaking gRPC/h2c — the three things a health check needs to prove. `session/tymux/transport.go`'s
`rpcTransport` interface (lines 27-35) already exposes `ListSessions` as one of its five
narrow methods, so the plumbing to call it exists; what's missing is only the
"call it, on a timer/on-demand, and interpret connection-refused vs. timeout vs. success"
logic — i.e. stapler-squad's own analogue of `tmux.go`'s `EnsureServerRunning`/
`ensureServerRunningWithRetry` (tmux.go:602-666), which already has the exact shape needed
(bounded retries with exponential backoff, distinguishing "not running yet" from "genuine
error") and is the closest existing precedent named in the requirements doc.

**Recommendation for the plan phase:** don't block on tymux gaining a real health RPC —
`ListSessions` is sufficient and already reachable through the existing `rpcTransport` seam.
If a dedicated `Health`/`Ping` RPC is wanted later for clarity (e.g. to avoid the
supervision code depending on session-listing semantics), that's a small, separately-scoped
upstream `tstapler/tymux` change, not a prerequisite for this project.
