# UX Research: go-auto-instrumentation (DX perspective)

> **Superseded-tool notice (added post-Phase-3, Triad Review):** this document was written
> during Phase 2, before Phase 3 planning pivoted the target tool from loongsuite-go to
> `open-telemetry/opentelemetry-go-compile-instrumentation` (`otelc`) — see
> [ADR-001](../decisions/ADR-001-target-otelc-over-loongsuite-go.md). Everywhere below that
> names `otel`/loongsuite-go's CLI, its `curl | sudo bash` installer, or its GitHub org, treat
> it as **the DX pattern loongsuite-go exhibited**, not the literal command to run — `otelc`'s
> actual install/invocation is `implementation/plan.md`'s Story 1.1.1/2.1.1 (Toolexec Injection
> via `GOFLAGS`, not a `curl | sudo bash` prebuilt-binary install). The *shape* of this
> research (hard-fail missing-tool guard, distinctly-named output binary, no built-in
> "did it work" check) still holds for `otelc` and is why it's kept rather than rewritten.

No end-user UI surface. This is developer/operator-facing infrastructure: a new
`make` target, a CLI build wrapper (`otel go build` from
[loongsuite-go-agent](https://github.com/alibaba/loongsuite-go-agent) — see the
superseded-tool notice above), and a doc update. Treated as DX (developer-experience)
research per the task brief.

## 1. Comparable DX patterns already in this repo

The closest existing analog is **`make build-embedded`**
([Makefile:277-286](../../../Makefile#L277-L286)), which wraps a normal `go build`
with an extra build-time dependency (a pinned tmux binary) and a non-default build
tag:

```makefile
build-tmux: ## Build pinned tmux 3.4 binary from third_party/tmux submodule
	@./scripts/build-tmux.sh

build-tmux-embed: build-tmux ## Copy built tmux into the embed dir for go build -tags embed_tmux
	@mkdir -p session/tmux/embed
	@cp $(BIN_TMUX) session/tmux/embed/tmux
	@echo "✅ session/tmux/embed/tmux ready ($(shell $(BIN_TMUX) -V 2>/dev/null || echo unknown))"

build-embedded: build-tmux-embed ## Build stapler-squad with tmux bundled inside the binary
...
	@echo "✅ stapler-squad built with embedded tmux"
```

Three conventions to copy for `build-otel-auto`:

- **Naming**: `build-<variant>` (`build-embedded`, `build-mux`, `build-tmux`), not
  `otel-build` or `build_otel`. `build-otel-auto` fits this pattern directly.
- **`## comment` help text** on every `.PHONY`-listed target — required, since
  `make help` greps `^[a-zA-Z0-9._-]+:.*?## .*$` ([Makefile:63-66](../../../Makefile#L63-L66)).
  Every target must also be added to the `.PHONY:` line at
  [Makefile:60](../../../Makefile#L60) (a 15-target list already; this repo
  enumerates rather than uses `.PHONY: %` wildcards).
- **`@echo "✅ ..."` completion line** at the end of every non-trivial build
  target (`build-embedded`, `stapler-squad`, `build-tmux-embed`,
  `web-build`/`server/web/dist`) — the repo's de facto "did it work" signal for
  a developer watching `make` output scroll by.

A second minor precedent: **`coverage-integration`**
([Makefile:544-556](../../../Makefile#L544-L556)) already builds a differently-instrumented
binary via a plain `go build -cover -o stapler-squad-cov .` flag, i.e. "same
source, different build invocation, separate output binary" is an established
shape in this Makefile, not a new idea.

The doc precedent is **`.claude/docs/bundling-tmux.md`** — 12 lines, command-block
first, one line of prose per command, no exposition:

```markdown
# Bundling tmux (Single-Binary Deployment)

For shipping stapler-squad as a self-contained binary with tmux embedded:

​```bash
git submodule update --init third_party/tmux
make build-tmux             # Compile pinned tmux 3.4 (~30s)
make build-embedded         # Build stapler-squad with tmux embedded
make test-with-pinned-tmux  # Tests against pinned tmux (reproducible)
​```

All benchmarks MUST be run with `&` — see `.claude/docs/benchmarks.md`.
```

Recommendation: extend `.claude/docs/opentelemetry.md` with a matching short
section (or split into a new `.claude/docs/opentelemetry-auto-instrumentation.md`
if it grows past a few lines) rather than inventing new doc structure, and add it
to `CLAUDE.md`'s "Reference Documents Index" table (per existing convention —
every doc in `.claude/docs/` is indexed there).

## 2. User mental model

A developer's first-run expectation, based on how `make build-embedded` and
`make build-mux` already behave: **"same binary, one extra capability, still
just `./stapler-squad`."** They will expect `make build-otel-auto` to (a) not
require touching their `go.mod`/imports, (b) produce a binary at the same
`./stapler-squad` path or a clearly-named sibling (cf. `stapler-squad-cov` for
`coverage-integration`, `stapler-squad.prev` for `backup-binary`), and (c) work
via `OTEL_ENABLED=true` the same way the hand-written instrumentation already
does today (documented in `.claude/docs/opentelemetry.md`).

**What will surprise them**: loongsuite-go is *not* a `go install`-able Go
tool — it ships prebuilt binaries or a `curl | sudo bash` installer to
`/usr/local/bin/otel`, or requires `make`/`make install` from a source clone
(VERIFIED via `raw.githubusercontent.com/alibaba/loongsuite-go-agent/main/README.md`).
This breaks the mental model set by `make install-tools`
([Makefile:590-600](../../../Makefile#L590-L600)), where every other dev tool
(`nilaway`, `staticcheck`, `golangci-lint`, `gosec`, `checklocks`, ...) is a
one-line `go install ...@latest`. A developer who reflexively runs
`make install-tools` and then `make build-otel-auto` will find `otel` still
missing — the target needs its own explicit prerequisite check and instructions
(see §3), and the doc must call out up front that this is an **external binary
install**, not a Go module dependency.

A second surprise: `otel go build` is a *prefixed* command
(`otel go build -o app cmd/app`), not a drop-in `go build` replacement flag.
The Makefile target has to shell out to `otel go build ...` with the same
`-ldflags`/`-tags`/output-path arguments `stapler-squad`'s normal build target
uses, mirrored carefully — a divergence here (e.g. forgetting `-ldflags
"$(LDFLAGS)"`) would silently produce a binary reporting `dev` from `stapler-squad
version` instead of the real version, undermining §4's verification story.

## 3. Error states — missing `otel` CLI

This repo has two existing patterns for a missing external tool, both `@which
<tool>` guards at the top of the target body:

**Hard-fail pattern** (used for tools with no `go install` path — `shellcheck`,
`sg`/ast-grep — the same situation `otel` is in):

```makefile
lint-shell: ## Run shellcheck over all first-party shell scripts
	@which shellcheck >/dev/null 2>&1 || (echo "shellcheck not installed; run 'brew install shellcheck' (macOS) or see https://github.com/koalaman/shellcheck#installing" && exit 1)
```
([Makefile:653](../../../Makefile#L653), mirrored at
[Makefile:659](../../../Makefile#L659) for `sg`)

**Self-install pattern** (used only for `go install`-able tools —
`golangci-lint`, `checklocks`):

```makefile
checklocks: ## Enforce +checklocks: mutex-discipline annotations (explicit violations only)
	@if ! which checklocks >/dev/null 2>&1; then \
		echo "Installing checklocks..."; \
		go install gvisor.dev/gvisor/tools/checklocks/cmd/checklocks@latest; \
	fi
```
([Makefile:749-752](../../../Makefile#L749-L752))

Since `otel` cannot be `go install`ed (VERIFIED above), `build-otel-auto` must
use the **hard-fail pattern**, not the self-install one — auto-installing via
`curl | sudo bash` from a Makefile target would be a meaningfully different (and
riskier) action than `go install`, and shouldn't happen silently. Recommended
message, matching the existing tone/format exactly:

```makefile
build-otel-auto: ## Build stapler-squad with loongsuite-go compile-time auto-instrumentation (opt-in)
	@which otel >/dev/null 2>&1 || (echo "otel (loongsuite-go) not installed; run: sudo curl -fsSL https://cdn.jsdelivr.net/gh/alibaba/loongsuite-go@main/install.sh | sudo bash" \
		"  or see https://github.com/alibaba/loongsuite-go-agent#installation" && exit 1)
	otel go build -ldflags "$(LDFLAGS)" -o stapler-squad-otel .
	@echo "✅ stapler-squad built with loongsuite-go auto-instrumentation → ./stapler-squad-otel"
```

(Exact install one-liner needs a final check against the current
`alibaba/loongsuite-go-agent` README at implementation time — installer URLs on
fast-moving upstream projects drift.)

## 4. "How do I know it worked?"

Checked loongsuite-go's own docs for a built-in answer
(`docs/user/config.md`, `README.md`, both via raw.githubusercontent.com):
VERIFIED there is **no documented mechanism to confirm a specific binary was
successfully auto-instrumented** — no `otel verify <binary>`, no startup banner,
no symbol/manifest check. The only verification surface loongsuite-go documents
is pre-build: `otel version` confirms the *tool* is installed and working
(`otel set -debug` / `OTELTOOL_DEBUG=1` gives verbose *build-time* logging of what
the weaver is doing, not a post-hoc binary check).

This means stapler-squad has to invent its own "did it work" signal, and should
reuse mechanisms already in the repo rather than build new ones:

- **Binary identity, already present**: `stapler-squad version` prints
  `main.version` injected via `-ldflags` ([main.go:524-531](../../../main.go#L524-L531),
  [Makefile:16-23](../../../Makefile#L16-L23) for the `VERSION`/`LDFLAGS` vars).
  Cheapest fix: build `build-otel-auto` to a **differently-named binary**
  (`stapler-squad-otel`, matching the `stapler-squad-cov` precedent in
  `coverage-integration`) so `file ./stapler-squad-otel` / its mere presence at a
  distinct path is the first-order "which build is this" signal — no new code
  needed. This also directly satisfies the requirements doc's Risk Control
  section ("a separate, opt-in build artifact").
- **Runtime confirmation, the real verification loop**: per the requirements
  doc's own success metric, actually run the binary with `OTEL_ENABLED=true` and
  confirm spans appear for a path that's untraced in the hand-written
  instrumentation today (e.g. an ent `database/sql` query, per the problem
  statement) — this is the only verification that actually matters, and it's a
  test/measurement task (Research Agent covering testing/ops territory), not a
  DX-surface concern. Flag for the plan phase: don't let "binary built
  successfully" be mistaken for "instrumentation actually wove in" — those are
  two different checks, and loongsuite-go's own docs don't collapse them either.
- **Cheap pre-runtime sanity check worth adding to the doc** (not upstream-
  documented, but derivable from standard Go tooling): `go tool nm
  stapler-squad-otel | grep -c otel` or `strings stapler-squad-otel | grep -c
  go.opentelemetry.io` compared between the auto-instrumented and normal binary
  gives a quick "more otel symbols got linked in" signal before running the full
  trace-verification loop. Worth one line in the doc as a fast smoke check, with
  the caveat that it's a heuristic, not proof of correct span placement.

## 5. Accessibility / i18n

Not applicable — no UI surface. Stated explicitly per the task brief; no
sections force-fit below this point.
