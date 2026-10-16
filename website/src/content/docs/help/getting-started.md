---
title: 'Getting started'
description: 'Install Stapler Squad via Homebrew, the manual install script, or by building from source.'
---

# Getting started

Both Homebrew and manual installation will install Stapler Squad as `ssq` on your system.

## Homebrew

```bash
brew tap tstapler/stapler-squad https://github.com/tstapler/stapler-squad && brew install tstapler/stapler-squad/stapler-squad
```

This installs both `stapler-squad` and the `ssq` alias.

## Manual install script

Download and install the latest pre-built binary:

```bash
curl -fsSL https://raw.githubusercontent.com/tstapler/stapler-squad/main/install.sh | bash
```

This puts the `ssq` binary in `~/.local/bin`.

To use a custom name for the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/tstapler/stapler-squad/main/install.sh | bash -s -- --name <your-binary-name>
```

## Build from source

Build and install directly from source. The script installs Go via Homebrew if it isn't already present, then compiles the full application (web UI + server) and puts `ssq` in `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/tstapler/stapler-squad/main/install.sh | bash -s -- --from-source
```

Or step by step:

```bash
# 1. Install build dependencies (Go, Node.js, buf)
brew install go node buf

# 2. Clone the repository
git clone https://github.com/tstapler/stapler-squad.git
cd stapler-squad

# 3. Build (compiles proto code, Next.js web UI, and Go binary)
make build

# 4. Install to ~/.local/bin
cp stapler-squad ~/.local/bin/ssq
```

> `node` and `buf` are required to compile the web UI and protobuf definitions. `make build` will install them automatically via Homebrew if they are missing.

Hit a build error, or `ssq` doesn't start after installing? See [Debugging logs](/stapler-squad/help/debug-with-logs/) in Troubleshooting.
