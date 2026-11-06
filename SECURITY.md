# Security Policy

## Supported Versions

Stapler Squad ships continuously via [release-please](https://github.com/googleapis/release-please) off a single rolling `main` branch — there are no LTS or maintenance branches. Only the latest release is supported; please upgrade before reporting a vulnerability.

## Reporting a Vulnerability

Please **do not open a public GitHub issue** for a security vulnerability.

Preferred: use GitHub's [private vulnerability reporting](https://github.com/tstapler/stapler-squad/security/advisories/new) ("Report a vulnerability" under the repo's Security tab). This opens a private advisory visible only to maintainers until a fix is ready.

If that's unavailable, email **tystapler@gmail.com** with:

- A description of the vulnerability and its potential impact
- Steps to reproduce (minimal repro case if possible)
- The `stapler-squad version` output and OS/installation method (Homebrew, pre-built binary, or source)

You should receive an acknowledgment within a few days. We'll keep you updated as a fix is developed and credit you in the release notes unless you'd prefer otherwise.

## Scope

Stapler Squad runs AI coding agents (Claude Code, Codex, Gemini, Aider, etc.) in local tmux sessions and git worktrees, and exposes a local web UI with optional remote access (WebAuthn passkey auth over HTTPS). Security-relevant areas include:

- The approval rules engine and review queue (anything that could let an agent action bypass approval)
- Remote access / WebAuthn authentication and the `--remote-access` HTTPS server
- Session/workspace isolation (git worktrees, tmux socket isolation)
- Credential or token handling (GitHub auth, agent API keys)

Vulnerabilities in a third-party AI agent CLI itself (Claude Code, Codex, Aider, Gemini) are out of scope here — please report those upstream.
