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

## Questions?

Open an issue at https://github.com/tstapler/stapler-squad/issues.
