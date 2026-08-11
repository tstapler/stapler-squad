# Adversarial Review: antigravity-history-translator

**Date**: 2026-08-08
**Verdict**: BLOCKED

## Blockers
- [ ] **Shell History Rewrite/Truncation** — Relying purely on `O_APPEND` is insufficient because shells frequently rewrite or truncate the entire history file on exit (based on `HISTSIZE` or `SAVEHIST`), which can silently overwrite or corrupt our injected events. **Recommendation**: We must define a strategy for concurrent writes (e.g., shell precmd/preexec hooks, or prompting the user to configure `inc_append_history` in zsh / `histappend` in bash).
- [ ] **Agent Execution of Redacted Commands** — The plan redacts secrets before sending them to the agent context. If an agent attempts to execute a previous command that contains `[REDACTED_CREDENTIAL]`, the command will fail. **Recommendation**: Define a mechanism for the execution layer to rehydrate the redacted secrets, or explicitly constrain the agent from executing redacted commands.

## Concerns
- [ ] **SQLite Driver Cross-Compilation** — The plan specifies SQLite but doesn't define the driver. Using CGO-based `go-sqlite3` will complicate cross-compilation and distribution of the CLI. **Recommendation**: Explicitly mandate a pure Go driver (e.g., `modernc.org/sqlite`) in Task 1.1.2a.
- [ ] **Zsh Proprietary Escaping (Metafication)** — Zsh history files use a proprietary metafication escaping mechanism (e.g., `\x83`) for specific characters, including newlines. A generic state machine parser will corrupt this data. **Recommendation**: Explicitly require implementing Zsh's `unmetafy` algorithm in Task 1.2.1a.
- [ ] **Regex Performance on Large Histories** — Using TruffleHog regex sets on massive historical shell logs can cause severe CPU spikes and memory exhaustion. **Recommendation**: Enforce a strict timeout or chunking limit per parsing run in the Inhibition Engine.

## Minors
- **TTL Scope Drift** — Task 1.1.2a introduces a database TTL. This wasn't explicitly requested and could result in unexpected data loss for users who expect the SQLite DB to act as an infinite archive.
- **Error Handling on Bad Lines** — The parsers don't define whether to skip corrupted lines or abort the entire parsing process. Should default to skipping and logging.
