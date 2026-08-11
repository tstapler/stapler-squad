# Bundling tmux (Single-Binary Deployment)

For shipping stapler-squad as a self-contained binary with tmux embedded:

```bash
git submodule update --init third_party/tmux
make build-tmux             # Compile pinned tmux 3.4 (~30s)
make build-embedded         # Build stapler-squad with tmux embedded
make test-with-pinned-tmux  # Tests against pinned tmux (reproducible)
```

All benchmarks MUST be run with `&` — see `.claude/docs/benchmarks.md`.
