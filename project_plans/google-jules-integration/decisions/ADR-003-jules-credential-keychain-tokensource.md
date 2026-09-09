# ADR-003: Store the Jules API key in the OS keychain, reached through `CredentialChain` via a `JulesTokenSource` seam

**Status**: Accepted
**Date**: 2026-09-01
**Project**: google-jules-integration

## Context

A Jules API key is account-wide: it lets an agent open pull requests in every
GitHub repo the user's Jules account can reach. Requirements classify it as
**confidential**.

This repo has three existing patterns for such a secret, all verified in Phase 2:

1. **OS keychain via `github.com/zalando/go-keyring`** — `github/keychain.go`
   (service `"stapler-squad"`, multi-account `AccountRef{Username, Host}` model)
   and `session/sshremote/keystore.go` (service `"stapler-squad-ssh"`, single
   identity per remote, with a 5s D-Bus-hang timeout guard at
   `session/sshremote/keystore.go:36`).
2. **AES-256-GCM in `config.json`** — `session/backlog_crypto.go`'s
   `EncryptToken`/`DecryptToken`, used for the Slack webhook and workflow
   secrets.
3. **`CredentialChain`** — `server/services/credentials.go:100`, an ordered set of
   `CredentialSource`s (env var, config file, OAuth-derived, ADC-derived) used by
   the Gemini and Anthropic clients.

Pattern 2 is materially weaker than it looks: the AES key
(`Config.MachineEncryptionKey`) is generated and persisted **in the same
`config.json`** as the ciphertext (`config/config.go:1426`), so it only defends
against readers who do not read the whole file.

Separately, `research/pitfalls.md` §2 found there is **no** centralized
outbound-HTTP secret-redaction middleware in this repo (the only redaction infra,
`executor/audit.go`'s `redactArgs`, is for subprocess argv). Leak prevention is
therefore the new adapter's own responsibility.

## Decision

Three parts.

**1. The key lives in the OS keychain**, under its own service namespace
`"stapler-squad-jules"` — distinct from `"stapler-squad"` (GitHub) and
`"stapler-squad-ssh"`, following `session/sshremote/keystore.go`'s own reasoning
that separate credential domains should not share a keychain service. The
implementation copies that file's shape, including its timeout-raced keyring
operations, because Jules' single global key matches its one-identity model
better than GitHub's `AccountRef{Username, Host}` multi-account model.

Explicitly **not** `EncryptToken`/`config.json` — that path's key sits next to
the ciphertext, which is fine for a webhook signing secret and insufficient for
repo-write access across a user's whole account.

**2. It is reached through the existing `CredentialChain`**, via a new
`JulesCredentialSource` (`Name() == "jules_keychain"`) registered in
`NewDefaultChain` for provider `"jules"`, plus `JULES_API_KEY` recognized by the
existing `EnvVarCredentialSource`. This keeps credential resolution uniform with
the Gemini/Anthropic clients and preserves the chain's env-var-wins ordering for
scripted use.

**3. `jules.Client` depends on a one-method `JulesTokenSource` interface**
(`APIKey(ctx) (JulesAPIKey, error)`), not on `*CredentialChain`. `server/services`
imports `jules`; the reverse would be an import cycle, and would drag `server/`
types into a package that ADR-002 requires to stay self-contained. `jules/`
ships `KeyringTokenSource` as the default implementation;
`server/services` supplies a chain-backed one where chain semantics are wanted.

Leak prevention is enforced by construction, not discipline alone:

- `JulesAPIKey` is a newtype whose `String()` returns
  `"jules-api-key(redacted)"`. The real value is reachable only through an
  unexported `reveal()`.
- A test (`jules/secrets_guard_test.go`) scans the package and fails unless
  `reveal()` appears exactly once, inside the request builder, and never inside a
  `slog`/`fmt.Print`/`log.` call.
- Error classification records status code and path only; response bodies are
  truncated to 512 bytes and headers are never included.

## Alternatives Considered

- **`EncryptToken` into `config.json`** — Rejected; decryption key colocated with
  ciphertext (`config/config.go:1426`).
- **Plaintext in `config.json`** — Rejected; the file is screenshotted into
  support threads, backed up, and synced.
- **Copy `github/keychain.go` verbatim** — Rejected as the primary template. Its
  `AccountRef{Username, Host}` multi-account indirection has no meaning for
  Jules, which has one global key per user (max 3 keys per account, no host
  dimension). `sshremote/keystore.go` is the closer and more recent analog.
- **`jules.Client` holding `*CredentialChain`** — Rejected; import cycle and a
  breach of the package's isolation boundary.

## Consequences

**Positive**

- The highest-value credential in the feature gets the strongest storage the repo
  offers, and resolution stays consistent with the other API clients.
- The redaction gap identified in pitfalls §2 is closed for this adapter by a
  type-level guarantee plus an enforcing test, rather than by a convention.

**Negative**

- Headless/CI environments with no Secret Service must use `JULES_API_KEY`. The
  chain's env-var-first ordering already covers this, and it is documented in the
  how-to.
- A keyring read can hang on a broken D-Bus. Mitigated by the 5s timeout race
  copied from `sshremote/keystore.go`; a timeout degrades to
  `ErrJulesNotConfigured` — feature off — never a server hang.

## References

- `research/pitfalls.md` §2, §6
- `research/build-vs-buy.md` §4
- `github/keychain.go:10`, `session/sshremote/keystore.go:21,36,171`
- `server/services/credentials.go:100,110`
- `session/backlog_crypto.go:12`, `config/config.go:1426`
