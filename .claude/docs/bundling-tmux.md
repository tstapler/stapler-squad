# Bundling tmux (Single-Binary Deployment)

For shipping stapler-squad as a self-contained binary with tmux embedded:

```bash
git submodule update --init third_party/tmux
make build-tmux             # Compile pinned tmux 3.4 (~30s)
make build-embedded         # Build stapler-squad with tmux embedded
make test-with-pinned-tmux  # Tests against pinned tmux (reproducible)
```

All benchmarks MUST be run with `&` — see `.claude/docs/benchmarks.md`.

## Bundling tymuxd alongside tmux

`tymuxd` (the terminal-multiplexer daemon from github.com/tstapler/tymux) can be
embedded the same way, via a fetched-not-compiled prebuilt binary (see ADR-001,
`project_plans/tymux-bundled-integration/decisions/ADR-001-prebuilt-tymuxd-binary-download.md`):

```bash
make fetch-tymuxd          # Download the pinned tymuxd release binary (no cargo/rustc needed)
make build-embedded-tymux  # Build stapler-squad with both tmux and tymuxd embedded
```

This is a separate target from `build-embedded` so existing `-tags embed_tmux`
consumers are unaffected. See `.claude/docs/bundling-tymuxd.md` for the full
reference: supervision, the rollout-safety flags, `--tymuxd-keep-server`, and
the accepted TCP-loopback security tradeoff.
