# Stack Research: Go Auto-Instrumentation

Research date 2026-08-21. Sources: `gh api` against each project's GitHub repo (releases,
issues, raw file contents) and raw README/docs fetches. Repo facts (go.mod, imports) verified
by reading this worktree at
`/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-otel_18cdec531eb4a615`.

## Repo baseline (what we're instrumenting)

- `go.mod` `go` directive: **`go 1.26.3`** (`go.mod:3`).
- ORM/DB: `entgo.io/ent v0.14.5` over `database/sql`, with two SQLite drivers registered:
  `github.com/mattn/go-sqlite3 v1.14.40` (CGO) and `modernc.org/sqlite v1.56.0` (pure Go) —
  `go.mod:35,61`. `go.mod:5` notes `CGO_ENABLED=1` is required for the CGO driver.
- No direct Redis/Kafka client imports found (`grep` over non-test, non-generated `.go` files
  for `sirupsen/logrus`, `go.uber.org/zap`, `log/slog` hits only this repo's own `log/`,
  `executor/`, `server/analytics/` packages, all of which import stdlib `log`/`log/slog` —
  `log/log.go:7-8`). `logrus`/`zap` in `go.mod` are `// indirect` (pulled in transitively by
  other deps, e.g. `grpc-ecosystem/grpc-gateway`), not used by this repo's own code.
- `google.golang.org/grpc v1.81.1` is `// indirect` (`go.mod:210`); no `.go` file directly
  imports `"google.golang.org/grpc"` — ConnectRPC (`connectrpc.com/otelconnect`) is the actual
  RPC layer, not raw grpc-go.
- Existing OTel deps already pinned: `go.opentelemetry.io/otel` v1.44.0,
  `go.opentelemetry.io/otel/sdk` v1.44.0, `otelhttp` v0.67.0, `otelconnect` v0.8.0,
  `otlptracegrpc` v1.39.0, `otlpmetricgrpc` v1.44.0.
- Build matrix per `CLAUDE.md`/`Makefile`: `-tags embed_tmux` (`Makefile:272,280,284`), custom
  `-ldflags "$(LDFLAGS)"`, cross-platform (Linux CI/primary, macOS at work), CGO-dependent
  sqlite build, and ent codegen (`session/ent/*.go`, gitignored) that must exist before any
  build.

---

## 1. loongsuite-go (Alibaba, compile-time weaving)

**Current release**: `v1.13.0`, published 2026-07-14 (confirmed via
`gh api repos/alibaba/loongsuite-go/releases`). Repo was formerly named
`opentelemetry-go-auto-instrumentation` (old name shows up in older issue titles).

**Install method**: three options, no `go install` path —
1. Prebuilt binaries (Linux amd64/arm64, macOS amd64/arm64, Windows amd64) — recommended.
2. One-line install script: `curl -fsSL https://cdn.jsdelivr.net/gh/alibaba/loongsuite-go@main/install.sh | bash`.
3. `make` / `make install` from source (the tool's own `go.mod` pins `go 1.24.0`, i.e. the
   *tool's build* requires Go ≥1.24 — separate from what Go versions it can weave into).

**Go toolchain compatibility (target application) — THE BLOCKING FINDING**: per
`docs/user/compatibility.md`, loongsuite-go "follows Go's official support policy" and
currently **tests/supports only Go 1.23–1.24**, on Ubuntu/macOS 13+/Windows, amd64/386/arm64.
This repo's `go.mod` pins **`go 1.26.3`** — two major versions ahead of loongsuite-go's tested
range. The doc explicitly disclaims other versions: *"this project should work for other
systems, no compatibility guarantees are made."* This is the single biggest open risk for
adopting loongsuite-go here and should be verified empirically (spin up `otel go build` against
this repo) before committing further design work to it.

**Supported libraries — mapped against this repo's actual deps**:

| This repo uses | loongsuite-go coverage | Notes |
|---|---|---|
| `database/sql` via `entgo.io/ent` | ✅ Yes — explicit `database/sql` instrumentation rule | A closed PR (`#737`, "fix: stop emitting false errors for SQLite in database/sql instrumentation") shows SQLite-over-`database/sql` is a directly tested path — matches this repo's `mattn/go-sqlite3`/`modernc.org/sqlite` setup. |
| `net/http` (server + client) | ✅ Yes | Listed explicitly; coexists with existing `otelhttp` manual instrumentation (needs verification it doesn't double-span). |
| `log/slog` (this repo's actual logger) | ✅ Yes — "Slog" listed alongside Zap/Logrus/Zerolog | Directly relevant since `log/log.go` uses stdlib `slog`. |
| gRPC (`google.golang.org/grpc`, indirect only) | ✅ Yes, but **not applicable** — repo has no direct grpc-go usage; RPC layer is ConnectRPC via `otelconnect`, which loongsuite-go does not separately claim to instrument (not in its library list). | Auto-instrumentation would add no coverage here beyond what `otelconnect` already provides. |
| Redis / Kafka | ✅ Both covered (goredis/redigo/rueidis; kafka franz-go/segmentio/sarama) | **Not applicable** — repo has no direct Redis or Kafka client. |
| `safeexec`/tmux/git subprocess layer | ❌ Not in the supported-library list (it's exec.Cmd-based, not a "library" loongsuite-go ships a rule for) | This is the one path requiring loongsuite-go's **plugin/hook mechanism** (see below), matching the requirements doc's "if feasible within appetite" scope item. |

Net: of this repo's actual instrumentable surface, loongsuite-go's out-of-the-box coverage is
useful almost entirely for the `database/sql`/ent path — the requirement doc's stated target
(ent's `database/sql` driver) is directly covered. gRPC/Redis/Kafka coverage is moot since the
repo doesn't use those clients directly.

**Plugin/hook mechanism** (`docs/dev/hook.md`): hook modules live under `pkg/rules/<name>/`
(e.g. `pkg/rules/mux`) as "just regular Go code." A hook function uses `//go:linkname` to bind
to a target function, takes `api.CallContext` as its first parameter followed by parameters
matching the target function's inputs (entry hook) or outputs (exit hook), and mutates behavior
via `CallContext.SetParam()`/`SetReturnVal()`. No framework beyond referencing an existing rule
package as a template — moderate Go skill required, no special tooling. This is the path that
would need to be built for the `safeexec`/tmux/git subprocess layer mentioned in-scope.

**Build-flag passthrough**: `otel go build` accepts standard `go build` flags — docs show
`-gcflags` and `-o` explicitly; `-tags`/`-ldflags`/`CGO_ENABLED` are not explicitly demonstrated
in `docs/user/config.md` but the tool is described as a drop-in wrapper for `go build`, so
`-tags embed_tmux -ldflags "$(LDFLAGS)"` should pass through — **needs empirical verification**,
not just doc-inference, since the docs don't show these two specific flags in an example.

**Known risk — dependency version rewriting** (confirmed via GitHub issues, not inferred):
- **Issue #386** (open): `otel go build` silently upgraded a project's pinned `grpc` from
  v1.61.0 → v1.71.0 during weaving, breaking startup. Root cause per maintainer: the tool's own
  OTel SDK hook dependencies pull in their own grpc version and the merge isn't pinned to the
  user's version.
- **Issue #568** (open, proposal only, unresolved): "Resolving Hook Module Dependency
  Conflicts" — the tool's hook code can require different versions of a shared dependency than
  the user's project, forcing an upgrade and "numerous unexpected compilation errors." No fix
  shipped yet.
- Direct relevance to this repo: we already pin exact OTel versions (`otel` v1.44.0,
  `otlptracegrpc` v1.39.0, `otelconnect` v0.8.0) plus `google.golang.org/grpc v1.81.1`
  indirect. `docs/user/compatibility.md` states explicit version pairing requirements per
  loongsuite-go release (e.g. "tool v1.10.0 requires OTel v1.40.0 and OTel Contrib v0.65.0") —
  our pinned v1.44.0 SDK will need to be checked against whatever OTel version `v1.13.0`
  (current release) requires, and a mismatch is a documented failure mode, not a hypothetical
  one.

**Build tags / go:embed / cross-compilation**: no explicit documentation or issue found stating
either compatibility or incompatibility with build tags, `go:embed`, or cross-compilation
specifically (`gh api search/issues` over the repo for "embed" turned up no matches about
`go:embed`). Since the tool works as a `go build` wrapper rather than a `-toolexec` plugin
(see below), it likely operates by staging/copying the module before invoking a modified build
— a shape that has historically been the failure mode for `go:embed` (relative embed paths can
break if the tool relocates source), so this needs a direct empirical test with
`-tags embed_tmux` and the embedded tmux binary in `session/tmux/` rather than being assumed
safe from absence-of-issues.

**`-toolexec` mechanism**: notably, issue **#389** ("[Proposal] Instrument via
`go build -toolexec=otel`") is still **open**, meaning the current mechanism is *not* Go's
standard `-toolexec` plugin hook — it's a full `go build` command wrapper (`otel go build ...`)
that does its own source/module manipulation. This matters for CI integration: it can't simply
be dropped into an existing `go build -toolexec=...` invocation; the Makefile target has to
literally swap `go build` for `otel go build`.

**Module dependency footprint**: loongsuite-go is a standalone CLI tool (binary or built via
`make`), **not** a `go.mod` dependency of the instrumented project — confirmed by the install
methods above (no `go install` / no module import path documented for consumption). It does,
however, rewrite the *target* project's `go.mod`/`go.sum` during the weave (per issues #386/#568
above), so treat the woven build as needing its own `go.mod`/`go.sum` diff review, not as a
side-effect-free wrapper.

---

## 2. `open-telemetry/opentelemetry-go-instrumentation` (official OTel org, eBPF)

**Maturity**: **still explicitly "work in progress" / experimental** per its README
(`:construction: This project is currently work in progress.`). Confirmed active (repo not
archived, last push 2026-08-21, 1024 stars). Latest release **`v0.24.0`**, published
2026-04-27 (`gh api repos/open-telemetry/opentelemetry-go-instrumentation/releases`); an
earlier `sdk/v1.2.1` tag exists for a separate SDK submodule. Version numbering is still
sub-1.0, consistent with the "work in progress" self-description — this has not graduated to
a stable/GA release in the >1 year since the `v0.14.0-alpha` tag referenced in older issues.

**Go/kernel requirements**: design goal claims support for "Go version 1.12 and above,"
including binaries stripped via `-ldflags "-s -w"`. Kernel requirement: "any Linux kernel above
4.4." (Both from `docs/how-it-works.md`.)

**Deployment model — confirmed concretely, not assumed**: it's a **separate agent process**
(own binary or container image `otel/autoinstrumentation-go`) that attaches to an
already-running target process via eBPF, driven entirely by `OTEL_*` env vars — critically
`OTEL_GO_AUTO_TARGET_EXE` (non-standard, not part of the general OTel env var spec) pointing at
the target executable's path. It supports three deployment shapes per `docs/getting-started.md`:
Kubernetes sidecar/shared-process-namespace, Docker Compose (`privileged: true`, `pid: "host"`),
and directly "on a Linux host" — the last of which is the relevant shape for this repo's
single systemd-managed binary (could run as a companion systemd unit pointed at the deployed
`stapler-squad` binary's path).

**Root/CAP_SYS_ADMIN requirement — confirmed, not assumed**: `docs/getting-started.md`
explicitly states *"run the instrumentation with elevated privileges"* and *"Run ... with root
privileges,"* and the Kubernetes example sets `runAsUser: 0` + `privileged: true`; Docker
Compose examples add `privileged: true`. Filed issue **#1141** (open) is a real-world report of
someone trying to avoid `privileged: true`/host PID namespace by running the agent in the same
container as the app — confirming the community also treats this privilege requirement as
the norm, not an edge case. For a systemd-managed non-container deployment, "root privileges"
would concretely mean either running the whole `stapler-squad` service as root (a real security
regression) or granting the companion agent process `CAP_SYS_PTRACE` (Linux, best-effort — not
explicitly confirmed as sufficient by the docs, which only mention "root"/`privileged: true`).

**Build tags / go:embed / cross-compilation**: no mentions found in README or `how-it-works.md`;
since this tool operates entirely on the *compiled binary* post-hoc (no rebuild step), build
tags and `go:embed` are moot for it — they're resolved at compile time before this tool ever
runs, so there is no interaction to test. This is actually a structural advantage over
loongsuite-go on this specific axis.

**Module dependency footprint**: standalone agent binary, built via `make build` from its own
repo, not a `go.mod` dependency of the target project. Zero interaction with this repo's
`go.mod`/`go.sum` since it never touches source or triggers a rebuild.

---

## 3. Grafana Beyla

**Current version**: `v3.33.0`, published 2026-08-20 (`gh api repos/grafana/beyla/releases`) —
actively maintained, weekly release cadence visible in the release list. **License**:
Apache-2.0 (`gh api repos/grafana/beyla --jq '.license.spdx_id'`).

**Deployment model**: runs as a **standalone process** (`./beyla` with env-var configuration)
on a plain Linux host, in addition to Kubernetes manifests under `deployments/` — so unlike
Odigos (which the requirements doc already expects is a poor fit, being k8s-operator-centric),
Beyla is a legitimate non-k8s option for a single systemd-managed binary.

**Root/eBPF/privilege requirements — confirmed, not assumed**: README states *"Running Beyla
requires sudo"* on host systems; Docker Compose needs either privileged mode or the
`SYS_ADMIN` capability; Kubernetes needs its documented minimum capability set. eBPF must be
enabled on the host kernel.

**Minimum kernel version**: **Linux kernel 5.8+ with BTF enabled** (verify via
`/sys/kernel/btf/vmlinux`), with a documented exception for RHEL 4.18 kernels build 348+ (and
CentOS/AlmaLinux/Oracle Linux equivalents) via backports. This is a **materially higher**
kernel floor than `opentelemetry-go-instrumentation`'s stated "4.4+" — worth checking against
the actual kernel on whatever host(s) run the deployed stapler-squad instance (not verified in
this research pass; check with `uname -r` / BTF presence on the target machine before treating
Beyla as viable).

**Go version note**: README states target Go binaries must be "compiled with at least Go
1.17" — a much lower floor than loongsuite-go's 1.23–1.24 ceiling, since (like
`opentelemetry-go-instrumentation`) Beyla instruments the already-compiled binary rather than
reweaving source, so it isn't sensitive to this repo's `go 1.26.3` directive the way
loongsuite-go is.

**Build tags / go:embed / cross-compilation**: no specific issues or doc mentions found for
either topic. Same structural reasoning as `opentelemetry-go-instrumentation` applies: Beyla
observes a running binary via eBPF post-build, so there's no rebuild/source-manipulation step
for `-tags embed_tmux` or `go:embed` to interact with.

**Module dependency footprint**: standalone binary/container, not a `go.mod` dependency.
Note Beyla vendors eBPF probe code via an "OBI" git submodule in its own repo (visible from the
frequent "Update OBI submodule" PRs) — irrelevant to consumers, just confirms it's a
self-contained binary distribution, not something that touches a consuming project's Go module
graph.

---

## 4. Build-tag / cross-compilation / go:embed compatibility — cross-tool summary

| Tool | Interacts with build/source? | `-tags embed_tmux` / `go:embed` risk |
|---|---|---|
| loongsuite-go | Yes — wraps and re-executes `go build`, manipulates `go.mod`/`go.sum` | **Real, unverified risk.** No issue-tracker evidence either way; the tool's own dependency-rewriting bugs (#386, #568) show it does mutate build state that other tools leave untouched. Must be tested directly against a `make build-tmux-embed`-equivalent invocation before relying on it. |
| opentelemetry-go-instrumentation | No — attaches to an already-built binary via eBPF | Not applicable; no interaction with build tags/embed since it runs post-build. |
| Grafana Beyla | No — same as above, post-build eBPF attach | Not applicable, same reasoning. |

This is a meaningful differentiator not called out directly in either eBPF tool's docs but
follows necessarily from "no rebuild required" being the design goal both state up front.

## 5. Go module / dependency-conflict summary

All three tools are confirmed **standalone CLI/agent binaries, not `go.mod` dependencies** —
none of them appear as an importable Go package a consuming project would add to `go.mod`.
The practical dependency-conflict risk differs sharply by mechanism:

- **loongsuite-go**: real, documented risk — it rewrites the target's `go.mod`/`go.sum` during
  the weave and has two open, unresolved issues (#386, #568) about it silently bumping
  transitive dependency versions (observed concretely with `google.golang.org/grpc`,
  directly relevant since this repo pins `otlptracegrpc`/`otlpmetricgrpc`/`otelconnect` to
  specific OTel versions that must match loongsuite-go's per-release compatibility table).
- **opentelemetry-go-instrumentation** and **Beyla**: no dependency-conflict risk — neither
  touches the target's module graph at all; they operate on the compiled artifact/running
  process.

---

## Bottom line for the plan phase

1. loongsuite-go is the only one of the three that can produce **new spans inside** a
   previously-untraced code path like ent's `database/sql` driver by weaving instrumentation
   into the binary — which is exactly what the requirements doc's success metric asks for. But
   its 1.23–1.24 tested-version ceiling against this repo's `go 1.26.3` directive, plus the
   documented dependency-version-rewriting bugs against our already-pinned OTel/grpc versions,
   are the two concrete risks to validate empirically (a spike build) before committing further
   design effort.
2. Both eBPF options (`opentelemetry-go-instrumentation`, Beyla) sidestep the build-time risks
   entirely but require root/CAP_SYS_ADMIN/`SYS_PTRACE`-class privileges on the host running
   stapler-squad — a real operational/security trade-off for a systemd-managed personal-infra
   service, and neither can reach into `database/sql`-level detail the way source-weaving can
   (they instrument at the network/syscall boundary, not internal library calls) — worth
   confirming during the plan phase whether "ent's `database/sql` driver" specifically is even
   observable via eBPF-level instrumentation the way it explicitly is via loongsuite-go's
   documented `database/sql` rule.
3. `opentelemetry-go-instrumentation` is still self-described as "work in progress" after
   several years and sub-1.0 versioning — a real maturity signal against choosing it over the
   more actively-released, non-experimental Beyla if an eBPF path is chosen at all.
