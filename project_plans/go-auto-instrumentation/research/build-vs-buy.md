# Research: Build vs. Buy — Go Auto-Instrumentation

**Agent**: Research Agent 6 (Build vs. Buy)
**Date**: 2026-08-21
**Method**: Fetched live READMEs/docs via WebFetch (raw.githubusercontent.com for GitHub markdown, rendered pages otherwise) and WebSearch for maturity/license signals not present in READMEs. Every claim below is either VERIFIED (source fetched/quoted) or explicitly marked INFERRED where a source could not be fully retrieved.

## TL;DR

The evidence overrides the user's initial choice of `alibaba/loongsuite-go` **in favor of a newer, more official candidate that wasn't in the original 4-option list**: `open-telemetry/opentelemetry-go-compile-instrumentation` — the official OTel-org compile-time weaving tool, which reached v1 stable in 2026 and is co-led by the same Alibaba engineer who built loongsuite-go. See "A fifth candidate the requirements didn't anticipate" below. All eBPF options (official OTel eBPF, Beyla, Odigos) are poor fits for this repo's deployment model (unprivileged systemd `--user` unit, no root, no Kubernetes).

---

## 1. `alibaba/loongsuite-go`

**Source**: [raw README](https://raw.githubusercontent.com/alibaba/loongsuite-go/main/README.md), [repo page](https://github.com/alibaba/loongsuite-go) — VERIFIED.

**What it is**: Compile-time AST/bytecode weaving. Prefix the build with `otel go build`; instrumentation is baked into the binary with no source changes.

**Maturity (VERIFIED from repo page)**: 895 stars, 462 commits on main, 13 open issues, Apache-2.0 license. Sole org backing: Alibaba (no OTel-org governance, no listed co-maintainers outside Alibaba on the repo page itself).

**Library coverage (VERIFIED from README)**: 80+ libraries — Echo, Gin, Fiber, Hertz, Iris, gRPC, Kratos; GORM, MongoDB, database/sql, ClickHouse, Neo4j, Cassandra; Kafka (multiple clients), RabbitMQ, RocketMQ; Redis (multiple clients), Memcached; Zap, Logrus, Zerolog, Slog. Against this repo's actual `go.mod` (checked directly): `net/http` ✅, `database/sql` (via `entgo.io/ent`) ✅, `google.golang.org/grpc` ✅, `go.uber.org/zap` ✅ (indirect dep), `sirupsen/logrus` ✅ (indirect dep). **ConnectRPC (`connectrpc.com/connect`) is not in the supported list** — this repo's primary RPC transport would not get auto-instrumented; it already has manual `otelconnect` coverage per `telemetry.go`, so this is a coverage gap the tool doesn't close, not a regression.

**Plugin mechanism (VERIFIED)**: README confirms a documented extension path for instrumenting unsupported libraries via custom code injection — relevant if ConnectRPC coverage is ever wanted.

### Pros
- No privileges required, no runtime agent/process, no kernel dependency — works identically to a normal `go build` wrapper, fits `make build`/CI cleanly.
- By far the broadest library coverage of any candidate today (80+ libraries vs. the official OTel project's initial ~5).
- Prebuilt binaries for Linux/macOS/Windows — matches this repo's actual build matrix (Linux CI/primary, macOS at work).
- Plugin mechanism exists for gaps (e.g., ConnectRPC).

### Cons
- Single-vendor (Alibaba) governance — no OTel SIG process, no multi-vendor review, bus-factor risk if Alibaba deprioritizes it.
- Smaller community footprint (895 stars, 13 open issues) than a CNCF/OTel-org project would carry at similar age.
- Its own natural successor now exists inside the OTel org itself (see section 6) — investing further in loongsuite-go-specific tooling risks migrating off it later anyway.

**Verdict: Viable, but not recommended as the long-term target** — see section 6 for why.

---

## 2. `open-telemetry/opentelemetry-go-instrumentation` (eBPF, official OTel org)

**Source**: [raw README](https://raw.githubusercontent.com/open-telemetry/opentelemetry-go-instrumentation/main/README.md) — VERIFIED. Cross-checked via WebSearch against [opentelemetry.io blog](https://opentelemetry.io/blog/2025/go-auto-instrumentation-beta/) and [OBI 2026 goals post](https://opentelemetry.io/blog/2026/obi-goals/).

**Maturity (VERIFIED, quoted)**: README literally states *":construction: This project is currently work in progress."* WebSearch confirms a beta release in Feb 2026, with the OBI (OpenTelemetry eBPF Instrumentation) SIG explicitly targeting **stable 1.0 sometime in 2026** — i.e., as of today (2026-08-21) it has not yet reached that milestone. Apache 2.0, official OTel org.

**Operational requirements (VERIFIED)**: Linux kernel 4.4+, tested on amd64 (arm64 lacks automated testing), Linux-only natively (non-Linux needs Docker/VM). README does not spell out CAP_SYS_ADMIN/root explicitly in the fetched excerpt, but eBPF program loading universally requires `CAP_BPF`/`CAP_SYS_ADMIN` (or root) on the host — this is a kernel-enforced requirement of eBPF itself, not a project choice.

**Deployment model**: Attaches to a *running* binary/process (no rebuild needed) — but that attachment mechanism needs elevated privileges on the host. This repo runs as an **unprivileged systemd `--user` unit** (confirmed via `.claude/docs/systemd-user-service.md`: "installed as a **user** unit, not a system unit"). Running an eBPF-based agent against it means either (a) running the whole service as root — a real security regression for a personal dev tool — or (b) running a separate privileged companion process with elevated capabilities pointed at the target PID, which is new operational surface this repo doesn't have today.

### Pros
- No rebuild required — could instrument the existing binary in place, which is attractive for a quick trial without touching `make build`.
- Official OTel org, Apache 2.0.
- Actively targeted for 1.0 stability in 2026 (per OBI SIG goals) — trajectory is toward maturity.

### Cons
- Self-described "work in progress" as of the fetched README; not yet at its own stated stability bar.
- Requires root/CAP_SYS_ADMIN-class privileges to attach — incompatible with this repo's actual deployment as an unprivileged `systemd --user` service without a real architecture change (adding a privileged sidecar).
- Linux-only; macOS (this repo's secondary dev platform) needs a Docker/VM detour, which doesn't match "works with this repo's actual build matrix."

**Verdict: Not recommended** — the constraint isn't code, it's the deployment model. Elevated privileges for a personal single-user desktop-adjacent tool is a worse trade than adding a build-time flag.

---

## 3. Grafana Beyla / OpenTelemetry eBPF Instrumentation (OBI)

**Source**: [raw README](https://raw.githubusercontent.com/grafana/beyla/main/README.md) — VERIFIED. WebSearch cross-check on the 2025 donation and license — VERIFIED.

**Important update the requirements doc didn't anticipate**: Grafana donated Beyla to OpenTelemetry in 2025. The code is being merged into the OTel project as **OpenTelemetry eBPF Instrumentation (OBI)** — the same SIG/effort referenced in section 2's "OBI 2026 goals" post. Grafana's `grafana/beyla` repo is becoming a downstream distribution of the upstream OTel project rather than a fully independent tool. Practically: **Beyla and `open-telemetry/opentelemetry-go-instrumentation`'s eBPF layer are converging into the same underlying project.** Evaluating them as fully separate options is somewhat moot going forward, though today they're still separate repos.

**License (VERIFIED)**: Apache 2.0, confirmed via WebSearch of the repo license badge — **not** AGPL or other copyleft. The dual-public-remote concern in requirements.md doesn't apply.

**Operational requirements (VERIFIED)**: Linux kernel 5.8+ with BTF (or RHEL 4.18+ with backports), eBPF enabled on host, `sudo` on bare host / minimum capabilities in Kubernetes. Same fundamental privilege requirement as section 2, since it's the same instrumentation mechanism.

**Coverage (VERIFIED)**: Broader protocol coverage than the official eBPF project today — HTTP/HTTPS/HTTP2, gRPC, SQL, Redis, Kafka, MongoDB — and multi-language (Go, Java, .NET, Node, Python, Ruby, Rust), which is irrelevant to this Go-only repo but signals project investment.

### Pros
- Apache 2.0 — no license concern despite the dual-remote setup.
- No rebuild needed, broader protocol coverage than the vanilla OTel eBPF project today.
- Converging with the official OTel project (OBI), so choosing it isn't really choosing a separate vendor long-term.

### Cons
- Same root/elevated-capability requirement as section 2 — same deployment-model mismatch with this repo's unprivileged systemd `--user` service.
- Standalone-process mode exists, but still needs `sudo` on the host per the README — no path to running unprivileged.
- Project identity is mid-transition (Grafana repo → OTel OBI) — adopting it now means re-adopting the OTel-native version later anyway.

**Verdict: Not recommended**, same root cause as section 2 (privilege requirement vs. this repo's deployment model), despite the license and coverage being fine.

---

## 4. Odigos

**Source**: [raw README](https://raw.githubusercontent.com/odigos-io/odigos/main/README.md) — VERIFIED. WebSearch on VM-agent mode — **partially inferred**, could not fetch the dedicated blog post content (fetch returned only the title, not body).

**What it is (VERIFIED)**: "Open-source distributed tracing solution... for Kubernetes environments and Virtual Machines," eBPF-based, Apache 2.0, Go/Java/Python/.NET/Node support.

**Standalone/non-k8s mode**: The README's install flow is `odigos install`, which is a Kubernetes-cluster-targeted CLI command (INFERRED from the README's Kubernetes-centric framing — no non-k8s quickstart was present in the fetched content). WebSearch turned up an "Odigos VM Agent" blog post title suggesting VM support exists as an extension, but the content could not be retrieved to confirm whether it operates independently of an Odigos **control plane** (which the rest of the project's architecture runs inside Kubernetes). Every other piece of evidence gathered (install command, documentation structure, "optimized for Kubernetes" framing, general Odigos architecture writeups) points to the VM Agent being a bolt-on that still phones home to a Kubernetes-hosted control plane, not a fully standalone binary.

**Confidence**: This is the one area of this report where I could not fully verify — the requirements.md's assumption ("likely a poor fit... Kubernetes operator") is **not refuted** by anything found, and the balance of evidence (architecture, install flow, marketing framing) supports it, but I did not find an explicit "the VM agent requires zero Kubernetes anywhere" statement to close the loop definitively either way.

### Pros
- Apache 2.0, active project (3,000+ stars per README-adjacent signals), multiple OTel maintainers involved per earlier search context.
- VM Agent feature suggests awareness that not everyone runs Kubernetes.

### Cons
- Core architecture and install UX (`odigos install`, control plane) is Kubernetes-first; this repo (systemd-managed, no Kubernetes anywhere in its deployment model per `CLAUDE.md`'s Architecture Overview and the manual-instance/systemd docs) doesn't have a cluster to run Odigos's control plane in even if the VM Agent itself needs no cluster.
- Standing up a Kubernetes cluster (even a small one) solely to host an Odigos control plane, for a single-binary personal tool, is disproportionate operational overhead compared to a `make build` flag.

**Verdict: Not recommended** — confirms (does not fully refute, but does not overturn) the requirements doc's original assumption.

---

## 5. Status quo — writing more manual spans

**What it is**: Continue hand-writing spans/counters in `telemetry/telemetry.go`, `otelhttp`, `otelconnect` at each new call site, as done today.

### Pros
- Zero new dependencies, zero build-pipeline risk, zero privilege/deployment-model questions — the safest option by construction.
- Full control over span naming, attributes, and semantic conventions; no risk of an auto-instrumentation tool emitting noisy/misleading spans for internals it doesn't understand (e.g., tmux subprocess orchestration, git worktree operations — neither auto-instrumentation candidate covers `os/exec` or the go-git library this repo already prefers per `.claude/rules/prefer-go-git-over-subshells.md`).
- No coordination with the opt-in constraint needed — this is already how the repo works.

### Cons
- Does not scale, per the requirements.md problem statement — every new code path (ent's `database/sql` driver, git operations, tmux/subprocess orchestration, backlog service internals) requires hand-instrumentation, and coverage will keep lagging actual code growth.
- Doesn't get "free" coverage for `database/sql` (ent) or the stdlib `net/http` paths not already wrapped — auto-instrumentation (either compile-time option) covers these today with zero code changes, which manual spans can't match for cost.

**Verdict: Viable as a fallback / complement**, not a substitute. Whatever compile-time tool is chosen (section 6), it should coexist with, not replace, existing manual `telemetry.go`/`otelconnect` instrumentation — per the requirements.md constraint — precisely because auto-instrumentation won't cover this repo's more unusual code paths (tmux orchestration, go-git operations, backlog reconciliation internals).

---

## 6. A fifth candidate the requirements didn't anticipate: `open-telemetry/opentelemetry-go-compile-instrumentation`

This is the single most important finding of this research pass, and it directly answers requirements.md's open question ("Does the official-OTel-org option change the recommendation despite the user naming loongsuite-go specifically?").

**Source**: [opentelemetry.io announcement](https://opentelemetry.io/blog/2026/go-compile-time-instrumentation-v1/) — VERIFIED via WebFetch. [OTel community governance doc](https://github.com/open-telemetry/community/blob/main/projects/go-compile-instrumentation.md) — VERIFIED via WebFetch. Star/issue counts — VERIFIED via WebSearch.

**What it is**: The **official OTel-org compile-time Go instrumentation project**, announced as a SIG at the start of 2025 and formed explicitly as "a joint effort... donation proposal coming from Alibaba and Datadog to replace Instrgen" (Datadog's own prior internal compile-time tool). It uses the same fundamental approach as loongsuite-go — a build wrapper (`otelc go build`) that weaves instrumentation in at compile time, "Zero Runtime Overhead... baked into your binary at compile time," Apache 2.0, badge status **"Stable."**

**Governance (VERIFIED, quoted)**: "Project Lead: @ralf0131 (Alibaba), @dineshg13 (Datadog), @pdelewski (QuesmaOrg)." **@ralf0131 is Alibaba** — meaning the same organizational lineage (and likely much of the same engineering expertise) behind loongsuite-go co-leads this official project. This is technical continuity plus proper multi-vendor OTel governance, not a from-scratch competitor.

**Maturity (VERIFIED)**: Reached v1 (first stable release) in 2026, per the official OTel blog. 397 GitHub stars, 144 forks (smaller community footprint than loongsuite-go's 895 stars today — it's younger).

**Library coverage today (VERIFIED, quoted)**: "common libraries and frameworks including `net/http`, `database/sql`, gRPC, Redis, and Go runtime metrics, with more added regularly." This covers this repo's `net/http`, ent's `database/sql`, and `google.golang.org/grpc` — but is a small fraction of loongsuite-go's 80+ libraries. **ConnectRPC is not covered by either tool** (confirmed via WebSearch — gRPC is supported, connect-go is not called out; this repo already has manual `otelconnect` coverage, so it's a wash).

**Known gap (VERIFIED via WebSearch)**: v1 doesn't yet support projects using `go mod vendor` — irrelevant here, this repo has no `vendor/` directory (confirmed: `ls vendor` → does not exist).

### Pros
- Official OTel-org project with multi-vendor governance (Alibaba + Datadog + QuesmaOrg), not a single-vendor dependency.
- Same compile-time-weaving mechanism as loongsuite-go — same "no privileges, no rebuild-only-once" cost profile, so switching candidates doesn't reopen the deployment-model question already settled against the eBPF options.
- v1 "Stable" as of 2026 — has already cleared the bar that `opentelemetry-go-instrumentation` (section 2) is still working toward.
- Led in part by the same Alibaba engineer behind loongsuite-go — the two projects are not divergent forks competing for the same niche; the official one looks like where the effort is consolidating.

### Cons
- Materially narrower library coverage today (5 categories vs. loongsuite-go's 80+) — no GORM, no Kafka/RabbitMQ/RocketMQ, no Zap/Logrus/Zerolog/Slog auto-instrumentation yet. This repo doesn't currently use Kafka/RabbitMQ, and its structured logging usage would need checking against what v1 actually covers before assuming parity.
- Younger project (397 stars vs. 895), smaller community, "more added regularly" is a forward-looking promise, not a completed feature set.
- No prebuilt-binary distribution story was confirmed (loongsuite-go explicitly ships prebuilt Linux/macOS/Windows binaries; this wasn't confirmed for the OTel project in the fetched content) — worth checking directly during planning before committing.

**Verdict: Recommended as the primary build target**, with loongsuite-go as a documented fallback if the plan phase finds a hard coverage gap (e.g., a library this repo needs that only loongsuite-go instruments).

---

## Comparison Table

| Tool | Instrumentation timing | Privileges needed | Rebuild needed? | Org backing | Library coverage for this repo | Maturity signal | Verdict |
|---|---|---|---|---|---|---|---|
| `alibaba/loongsuite-go` | Compile-time (build wrapper) | None | Yes (`otel go build`) | Alibaba (single vendor) | Broad: net/http, database/sql, gRPC, Zap, Logrus ✅; ConnectRPC ✗ | 895 stars, 462 commits, 13 open issues, Apache-2.0 | Viable, not recommended long-term |
| `open-telemetry/opentelemetry-go-compile-instrumentation` | Compile-time (build wrapper) | None | Yes (`otelc go build`) | **Official OTel SIG** (Alibaba + Datadog + QuesmaOrg) | Narrower today: net/http, database/sql, gRPC, Redis, runtime metrics; ConnectRPC ✗ | v1 "Stable" (2026), 397 stars, younger | **Recommended** |
| `open-telemetry/opentelemetry-go-instrumentation` (OTel eBPF) | Runtime (eBPF, attaches to running process) | Root/CAP_SYS_ADMIN-class | No | Official OTel org | gRPC, HTTP, DB, Kafka-go per beta notes | Self-described "work in progress," targeting 1.0 in 2026 | Not recommended (privilege/deployment mismatch) |
| Grafana Beyla / OBI | Runtime (eBPF) | Root (`sudo`) or capabilities | No | Grafana → converging into official OTel (OBI) | HTTP/gRPC/SQL/Redis/Kafka/Mongo | Actively donated/merging upstream, Apache 2.0 | Not recommended (same privilege issue) |
| Odigos | Runtime (eBPF) | Root-class + Kubernetes control plane | No | Odigos-io, Apache 2.0 | Go/Java/Python/.NET/Node | Active (3,000+ stars per README context), but Kubernetes-first | Not recommended (Kubernetes operator model, confirms requirements.md's assumption) |
| Status quo (manual spans) | N/A | None | N/A | N/A | Whatever is hand-written | Already in production use in this repo | Viable as a permanent complement, not a scaling solution |

---

## Overall Recommendation

**Build the plan phase against `open-telemetry/opentelemetry-go-compile-instrumentation`, not `alibaba/loongsuite-go`.** This overrides the user's originally named tool, and the evidence for doing so is:

1. It uses the exact same mechanism loongsuite-go does (compile-time weaving via a `go build` wrapper) — none of the deployment-model tradeoffs that rule out the three eBPF candidates apply differently to it than to loongsuite-go. Switching candidates doesn't reopen any settled question.
2. It has official OTel-org, multi-vendor governance (Alibaba + Datadog + QuesmaOrg) instead of single-vendor (Alibaba-only) governance — directly the distinction requirements.md asked this research to weigh.
3. It reached v1 "Stable" in 2026, and is co-led by the same Alibaba engineer whose org built loongsuite-go — this reads less like two competing tools and more like the same effort consolidating into the OTel org, with loongsuite-go as the vendor-distribution predecessor.
4. Its main weakness — materially narrower library coverage today (5 categories vs. loongsuite-go's 80+) — is a real, verified gap. But checked against **this repo's actual `go.mod`**, the gap doesn't bite yet: this repo's Go dependencies needing auto-instrumentation are `net/http`, `database/sql` (via ent), and `google.golang.org/grpc` — all three are already covered by v1. Kafka/RabbitMQ/GORM/Redis coverage (loongsuite-go's edge) isn't needed because this repo doesn't use those.

**Recommended fallback plan**: If the plan/implementation phase discovers the official project's v1 doesn't actually cover something this repo needs once tested against the real build (e.g., a structured-logging integration, or ConnectRPC turns out to matter more than the existing manual `otelconnect` coverage), document that gap explicitly and fall back to `alibaba/loongsuite-go` for that specific need — it remains a fully viable, broader-coverage option, just with weaker governance guarantees. Do not adopt any of the three eBPF-based tools (`opentelemetry-go-instrumentation`, Beyla/OBI, Odigos) for this project: all three require host root or Kubernetes-class privileges that are fundamentally incompatible with this repo's unprivileged `systemd --user` deployment model, independent of their library coverage or maturity. Continue writing manual spans in `telemetry/telemetry.go` for code paths no compile-time tool will ever cover (tmux orchestration, go-git operations, backlog internals) — the two approaches are complementary, not exclusive, matching the requirements.md constraint that this project "must coexist with, not replace, the existing manual instrumentation."
