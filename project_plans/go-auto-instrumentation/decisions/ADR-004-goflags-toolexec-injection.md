# ADR-004: Invoke the weaver via `GOFLAGS` toolexec injection rather than a duplicated build recipe

**Status**: Accepted (contingent — see Fallback)
**Date**: 2026-08-21
**Deciders**: SDD Phase 3 planning

## Context

`otelc` supports two invocation forms (`research/features.md` §4, from the project's own `docs/getting-started.md`):

1. **Wrapper Prefix Mode** — `otelc go build -o app .`, replacing the build verb. Same shape as loongsuite-go's `otel go build`.
2. **Toolexec Injection** — `export GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"` before an otherwise-unmodified `go build`. The docs explicitly recommend this form *"when the build command is owned by a Makefile, CI pipeline, or another tool you don't want to change."*

This repo's build line is non-trivial and carries real semantics: `go build -tags embed_tmux -ldflags "$(LDFLAGS)" -o stapler-squad .`, where `LDFLAGS := -X main.version=$(VERSION)` and `VERSION` is derived from `git describe` (`Makefile:22-23`, `Makefile:277-286`). On macOS both build targets additionally set `CGO_LDFLAGS="-sectcreate __TEXT __info_plist ..."` and then *verify* the Info.plist actually got embedded (`Makefile:148-153`, `Makefile:278-283`).

`research/ux.md` §2 names the concrete hazard of restating that line in a second target: a divergence — forgetting `-ldflags "$(LDFLAGS)"` — silently produces a binary whose `version` subcommand reports `dev` instead of the real version. That failure is invisible at build time and quietly undermines every downstream verification step that identifies which binary is under test.

Duplication is also how the macOS Info.plist branch and its verification would drift out of sync, since a copied recipe has to re-implement both.

## Decision

**Use Toolexec Injection, composed in one place: `scripts/otel-auto-build.sh`.**

The script exports the `GOFLAGS` value and then runs the caller-supplied `go build` argv unchanged. The Makefile target passes the *same* flag set the existing targets use, so `build-otel-auto` decorates the existing build rather than restating it. Structurally this is a Decorator (GoF): one behaviour (weaving) wrapped around an unchanged operation (the repo's real build invocation), rather than a second parallel implementation of that operation.

Concentrating the environment composition in the script also gives one place for the other **build-time** concerns to live: the `command -v otelc` hard-fail guard, the `-tags`/`-ldflags` passthrough, the echo of the exported `GOFLAGS` for reproducibility, and the Module Mutation Guard's pre/post `go.mod`/`go.sum` checksums.

**The script is scoped to build time only.** It must not set `OTEL_ENABLED`, `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER`, or any other runtime telemetry variable. An earlier draft of this ADR listed "the Exporter Toggle pairing if Spike D requires it" alongside the concerns above; that was a category error. The two kinds of concern only look alike because both are environment variables:

- `GOFLAGS` and the module checksums act on the `go build` process this script itself spawns — they work because the script is that process's parent, alive for the whole build.
- `OTEL_ENABLED` and the Exporter Toggle are read by `stapler-squad-otel` at *its* startup, which happens in a different process, launched separately, usually much later — by a developer, a smoke script, or the e2e harness. The build script has long since exited; nothing it exported is in that process's environment. Code here setting those variables would be inert, and worse than inert: it would read as if suppression were handled when nothing had been done.

Runtime suppression therefore lives where it can actually take effect — the documented **Run Recipe** in `.claude/docs/opentelemetry-auto-instrumentation.md` (plan Story 4.1.1), which gives the tracing-on and tracing-off launch lines, and is verified end-to-end by the Suppression Smoke Test (`scripts/otel-auto-smoke.sh --suppression`, plan Story 2.2.2) running the binary with tracing off against a liveness-checked collector and asserting zero spans arrive.

**Fallback**: if Spike A finds that Toolexec Injection does not correctly pass through `-tags embed_tmux` or `-ldflags`, the script switches internally to Wrapper Prefix Mode (`otelc go build ...`). The decision recorded here is about *where the weaving is composed* (one script, one place) — the invocation form inside it is an implementation detail Spike A settles empirically. Whichever form is used is recorded in `implementation/spike-verdicts.md`.

## Consequences

**Positive**
- The version-stamping drift `research/ux.md` §2 warns about cannot happen: there is only one recipe.
- The macOS Info.plist branch and its `otool` verification are inherited rather than re-implemented, if and when the woven build is needed on macOS.
- `GOFLAGS` is honoured by every `go` subcommand, so the same script also weaves `go test` builds — which is what makes the woven-binary benchmark comparison (Phase 3) possible at all. This is an assumption, not a guarantee, and the plan's Story 3.1.2a verifies it directly before relying on it.

**Negative**
- `GOFLAGS` is a blunt, process-wide instrument: anything the script execs inherits the weaving. The script therefore does exactly one thing and exits, rather than becoming a general-purpose dev shell.
- Build-time and runtime configuration now live in two places (this script, and the Run Recipe in the doc), so a reader looking for "how is telemetry turned off" will not find the answer in the build path. Mitigated by a comment in the script pointing at the Run Recipe, and by the Suppression Smoke Test using the *identical* variable set the doc prescribes, so the two cannot drift into testing different things.
- `-toolexec` is a single flag slot (`research/features.md` §2, failure mode 5). This repo uses no other `-toolexec` tool today, so there is no conflict now — but a future one would collide, and the doc should say so.
- The contingency makes this ADR partly provisional until Spike A reports. That is deliberate: recording a decision that a spike may overturn, with the overturn condition written down, beats deferring the decision and discovering it was implicitly made in the Makefile.
