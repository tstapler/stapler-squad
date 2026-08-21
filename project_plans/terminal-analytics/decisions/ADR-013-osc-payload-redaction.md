# ADR-013: OSC Payload Redaction in Escape Analytics Storage

## Status

Accepted

## Context

The escape analytics feature stores metadata about OSC (Operating System Command) escape
sequences emitted by programs running in stapler-squad terminal sessions. OSC sequences use
the format `ESC ] <code> ; <payload> BEL` and are commonly used by terminal applications for
purposes that frequently carry sensitive user data:

- **OSC 52** (`ESC ] 52 ; c ; <base64-data> BEL`): Clipboard read/write. The base64 payload
  is the raw clipboard content, which routinely contains API keys, passwords, authentication
  tokens, private keys, and other secrets that a user copies from a password manager or
  documentation.
- **OSC 0 / OSC 2** (`ESC ] 0 ; <title> BEL`): Window/tab title. Titles set by shells, editors,
  and multiplexers often embed file paths, hostnames, git branch names, or — when a user types
  a password into a visible shell prompt — the password itself.
- **OSC 7** (`ESC ] 7 ; file:///path BEL`): Current working directory as a `file://` URI,
  exposing the full filesystem path.

Two storage levels defined in FR-3 interact with this risk differently:

- `capture_level=full`: Stores verbatim `raw_bytes` for every sequence. An OSC 52 sequence
  with a 50 KB clipboard payload produces a 50 KB SQLite row. The pitfalls research identified
  that 10,000 such rows = 500 MB, far exceeding the NFR-1 estimate of ~100 bytes/row.
- `capture_level=summary`: Stores only metadata and a SHA-256 prefix hash of the payload.
  For OSC 0/2 window titles, the hash is **reversible by brute force**: a short title like
  `vim - README.md` or `root@prod-db` has an entropy of ~30–50 bits, well within offline
  attack range.

The mangle detection goal (FR-7) requires byte-level comparison of sequences between Stage 1
(PTY read) and Stage 2 (transport). For mangle detection purposes, an OSC 52 payload adds
no value: whether the clipboard content was "mangled" in transit is not a terminal rendering
concern. The functional purpose of the analytics feature — detecting corrupted escape
sequences that break terminal UI — is not served by persisting clipboard content.

## Decision

**Redact OSC 52 (clipboard) payload bytes by default. Controlled by a new config flag
`EscapeAnalyticsRedactOSCPayloads` (default `true`).**

The redaction policy is applied inside the `EscapeEventWriter` before any data reaches the
channel or SQLite, using the following rules:

### Default behavior (`EscapeAnalyticsRedactOSCPayloads=true`)

| OSC code | `capture_level=full` | `capture_level=summary` |
|---|---|---|
| OSC 52 (clipboard) | `raw_bytes` stored as `nil`; `payload_hash` stored as `""` (empty); `byte_length` preserved | Same as full — no hash, no bytes |
| OSC 0/2 (window title) | `raw_bytes` stored verbatim | `payload_hash` **not computed**; `byte_length` preserved only |
| OSC 7 (cwd URI) | `raw_bytes` stored verbatim | `payload_hash` **not computed**; `byte_length` preserved only |
| All other OSC | `raw_bytes` stored verbatim | `payload_hash` computed; `byte_length` preserved |

The OSC code is determined before the redaction decision — the parser already extracts the
numeric code from the OSC prefix (e.g., `52` from `ESC ] 52 ; ...`).

### Opt-out behavior (`EscapeAnalyticsRedactOSCPayloads=false`)

All OSC payloads are stored/hashed as-for non-OSC sequences. This is intended only for
debugging mangle detection on clipboard sequences and must be documented as a data-handling
risk in the config reference.

### Mangle detection impact

With OSC 52 payload bytes absent, Stage 1 and Stage 2 observations for clipboard sequences
will both have `payload_hash=""`. The `MangleCorrelator` treats `payload_hash=""` as an
unverifiable match — it records the sequence pair as `mangled=false` with a `mangle_type`
of `unverifiable_redacted` rather than attempting hash comparison. This is the correct
outcome: we cannot detect mangling of something we deliberately do not store.

For OSC 0/2 and OSC 7, byte_length is preserved and compared between stages. A change in
byte_length (e.g., a title truncated in transit) is still detectable as `mangle_type=truncated`
even without the payload hash, satisfying the core mangle detection goal for these sequences.

## Alternatives Considered

### Store all OSC payloads without redaction

Provides maximal data for mangle detection and future diagnostics. Rejected because:
- OSC 52 payload storage has no mangle-detection value (clipboard content is not a terminal
  rendering concern) but introduces severe data-sensitivity risk.
- A single paste of a long secret into the terminal would store that secret verbatim in the
  analytics SQLite database, a file on the user's local disk that may be synced, backed up,
  or included in bug reports.
- At `capture_level=full`, OSC 52 payloads can reach 50 KB/row, invalidating the 100-byte/row
  storage estimate by 500×.

### Never store any OSC payloads

Eliminates all PII/secret risk for OSC sequences but also eliminates mangle detection for
OSC 0/2 title sequences and OSC 7 CWD notifications. Terminal applications rely on these
sequences for shell integration features; truncation or stripping of these sequences is a
real and detectable mangle class. The byte_length preservation approach retains enough
information for truncation detection without storing the payload content.

### Store only a keyed HMAC of the payload (instead of SHA-256)

A keyed HMAC using a per-instance secret key would make brute-force reversal of OSC 0/2
title hashes computationally infeasible, allowing hash comparison for mangle detection
while protecting content. This approach was considered but rejected for Phase 1:
- It adds complexity (key generation, storage, rotation) for a diagnostic feature.
- OSC title mangle detection via byte_length comparison is sufficient for the Phase 1
  acceptance criteria.
- HMAC can be added in a later phase without changing the schema (replace `payload_hash`
  computation with a keyed variant; existing rows remain valid with empty/null hashes).

## Consequences

**Positive:**
- Clipboard secrets (API keys, passwords) are never written to the analytics database,
  eliminating the most severe data sensitivity risk.
- OSC 0/2 window titles are not hashed, preventing brute-force reversal of short titles.
- Storage estimates remain valid: OSC 52 rows contribute only fixed metadata fields (~50
  bytes), not multi-KB blobs.
- The default-on behavior means new deployments are safe without explicit configuration.

**Negative / Trade-offs:**
- OSC 52 mangle detection is not possible with `EscapeAnalyticsRedactOSCPayloads=true`.
  If clipboard sequence mangling becomes a reported issue, users must explicitly opt out of
  redaction to investigate. This is an acceptable trade-off given the sensitivity of the data.
- OSC 0/2 title mangling is detectable only at the byte_length level (truncation), not at
  the content level (mutation of specific characters). A title changed from
  `vim - foo.go` to `vim - bar.go` would not be flagged as mangled since byte_length is
  the same.
- The `EscapeAnalyticsRedactOSCPayloads=false` opt-out path requires explicit documentation
  of the privacy trade-off in the config reference and user-facing documentation.

## References

- FR-3: `EscapeAnalyticsRedactOSCPayloads bool` config field
- FR-2: `escape_event` schema — `raw_bytes` (optional), `payload_hash` (optional)
- NFR-1: ~100 bytes/row storage estimate; OSC 52 payloads violate this by up to 500× without redaction
- Pitfalls §5.1: Terminal escape injection via stored raw bytes
- Pitfalls §5.2: PII in OSC window titles and clipboard data (primary motivating finding)
- Pitfalls §5.3: Filename/path exposure via DCS and OSC sequences
- Pitfalls §3.2: Storage scale — `capture_level=full` + long OSC payloads = 500 MB edge case
- AC-3: Mangle detection correctly flags a stripped OSC sequence in an integration test
