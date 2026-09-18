# Contributing

Thank you for contributing to Stapler Squad!

## Development Setup

**1. Install Homebrew prerequisites**

```bash
brew install tmux gh
```

> `go`, `buf`, and `node` are installed automatically by the Makefile via Homebrew (or [asdf](https://asdf-vm.com) if you have it).

**2. Clone and build**

```bash
git clone https://github.com/tstapler/stapler-squad.git
cd stapler-squad

# Build (auto-installs go, buf, node via Homebrew if missing)
make build

# Install analysis and dev tools (nilaway, staticcheck, golangci-lint, etc.)
make dev-setup

# Run the server
./stapler-squad
```

**3. Rebuild after changes**

```bash
make restart-web   # Rebuild web UI + restart server
```

> **Note:** Never pipe or redirect `make restart-web` output — it will block forever. Run it plain.

## Code Standards

```bash
make pre-commit    # Format + vet + test + lint before committing
make quick-check   # Build + test + lint (faster)
```

## Testing

```bash
make test          # Run all tests
make test-coverage # Coverage report (coverage.html)
```

Please include tests for new features and bug fixes.

## Adding a new built-in agent detector

Status detection for a specific agent CLI (Claude Code, Aider, Gemini, etc.) lives in
`session/detection/binaries/*.go`, one small file per agent implementing
`dtypes.BinaryDetector`:

```go
type BinaryDetector interface {
    Name() string                        // binary name, e.g. "claude"
    Patterns() dtypes.StatusPatterns     // regex patterns per status category
    FilterContent(content string) string // optional per-binary output filtering
}
```

See `session/detection/binaries/claude.go` for a fully worked example — `Patterns()` returns
a `dtypes.StatusPatterns` struct with one `[]dtypes.StatusPattern` slice per status category
(`Idle`, `Active`, `NeedsApproval`, `Error`, etc.), each pattern a `Name`/`Pattern` (Go RE2
regex)/`Description`/`Priority`.

Don't have a live session of the target agent handy? You can also drop a local
`~/.stapler-squad/detectors/*.toml` file instead (see `session/detection/plugins.go`) — no
code change or PR required, and it hot-reloads. That path is the right fit for a private/
internal agent CLI you can't upstream; a `binaries/*.go` PR is the right fit once the
patterns are proven and you want them shipped as a default.

**Expedited review**: a PR that only touches `binaries/*.go` (no changes to
`session/detection`'s core registry/loader/snapshot logic) has a bounded blast radius — a
wrong regex affects status detection for one agent, not the trust/execution model (detector
content is regex-only, never executed — see `detector-plugins` ADR-004). Such PRs don't need
to wait for full review-queue cycling; any maintainer can review and merge directly once
`make quick-check` passes and the new/changed patterns have at least one test case (a sample
terminal-output fixture → expected status, following the existing `*_test.go` files
alongside each detector).

## Questions?

Open an issue at https://github.com/tstapler/stapler-squad/issues.
