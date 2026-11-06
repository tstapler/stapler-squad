# Architecture Review: ssq-cli-subcommands
**Date**: 2026-10-18
**Verdict**: CONCERNS (0 blockers, 3 concerns, 3 nitpicks)

kibitzer was available but was not run — these are all pre-code design-time files, and kibitzer
operates on compiled/indexed source. Text-only review applied across all four lenses.

---

## Blockers

None.

---

## Concerns

- [ ] **Story 3.1 — `--wait` liveness hole: terminal states cause silent false success**

  The plan specifies that `--wait` polls `GetSession` and exits when `status != SESSION_STATUS_CREATING`.
  `SessionStatus` has multiple terminal states beyond `ACTIVE` that creation can fail into:
  `SESSION_STATUS_STOPPED` (7), `SESSION_STATUS_CRASHED` (10), `SESSION_STATUS_PERMANENTLY_FAILED` (11).
  If the creation pipeline fails and the session transitions to any of these, the loop exits the
  `!= SESSION_STATUS_CREATING` check, prints `"session-id: <uuid>"`, and returns exit 0 — signalling
  success to the calling script when the session is actually dead.

  **Remediation**: Change the wait-loop condition to a two-branch check:
  `status == SESSION_STATUS_ACTIVE` → success; `status` is a terminal non-active state →
  print `"error: session creation failed with status <status>"` to stderr and exit non-zero.
  The terminal set is: STOPPED (7), CRASHED (10), PERMANENTLY_FAILED (11).
  Any other non-CREATING status (PAUSED, HIBERNATED, RESTORING) is ambiguous — treat as success
  for now and document in `--help` that only ACTIVE is the expected terminal state for a fresh create.

- [ ] **Story 3.2 — `--status` flag missing invalid-input rejection**

  The plan maps three string values (`"active"` / `"paused"` / `"all"`) to proto enum values but
  does not specify what happens when the user passes `--status crashed` or `--status bogus`.
  The proto field is `optional SessionStatus status = 1` — the Go enum's zero value
  (`SESSION_STATUS_UNSPECIFIED = 0`) would be sent, returning all sessions regardless of status.
  The user would see no error and no filtering, which is confusing.

  **Remediation**: Add a validation step in Story 3.2's implementation notes — after flag parsing
  and before building the request, validate `statusFlag` against the known set and return
  `fmt.Errorf("--status must be one of: active, paused, all (got %q)", statusFlag)`.
  Apply the same parse-at-boundary discipline to `--status` in any future commands that accept enum
  flags.

- [ ] **Story 1.1 — `NewSessionClient` uses `http.DefaultClient`, preventing test injection**

  `NewSessionClient(serverURL string) sessionv1connect.SessionServiceClient` hard-codes
  `http.DefaultClient` as the transport. This makes every `RunE` in the new commands untestable
  without a live server. The existing `get_session.go` has the same issue, but it uses the `var`
  (not constructor) pattern and had no tests — the new commands are being written as constructors
  precisely to enable testing (see the constructor pattern rationale in the plan).

  A unit test for `--wait` timeout handling, `--status` flag mapping, or `FormatConnectError`
  formatting cannot intercept the HTTP layer in the current design.

  **Remediation**: Add an optional `http.Client` parameter to `NewSessionClient`, or define a
  minimal interface over `sessionv1connect.SessionServiceClient` and inject a fake in tests.
  The lightest fix is `NewSessionClientWithHTTP(serverURL string, httpClient *http.Client)` plus a
  convenience wrapper `NewSessionClient(serverURL string) sessionv1connect.SessionServiceClient`
  that calls it with `http.DefaultClient`. This keeps the public API unchanged while opening a
  seam for tests without adding a new dependency.

---

## Nitpicks

- **`steer --help` note is slightly inaccurate.** The plan's `Long` field says steering "has no
  effect on the running process" for non-autonomous sessions. The pitfalls research
  (`research/pitfalls.md §3`) documents that on non-autonomous sessions the message is typed into
  the raw PTY — it is *not* silently dropped. The corrected wording: "On a non-autonomous session
  the message is sent directly to the terminal input (as if typed at the keyboard) rather than
  queued by the autonomous driver."

- **JSON output inconsistency across commands is underdocumented.** The plan notes the
  `get-session` / new commands divergence (one uses `encoding/json`, the rest use `protojson`) in
  §Unresolved Questions, but `--help` on `get-session` gives no hint that its `--json` emits
  integer-valued enums while `list-sessions --json` emits string-valued enums. A one-line note in
  `get-session`'s `Long` field prevents confusion: "Note: --json output uses encoding/json; enum
  fields are integers. Use list-sessions --json for string enum names."

- **`ResolveServerURL` contract omits trailing-slash normalisation.** If a user sets
  `SSQ_SERVER=http://localhost:8543/api/` (trailing slash) or `--server http://localhost:8543`
  (no `/api` suffix), ConnectRPC will construct incorrect procedure URLs. The plan does not specify
  whether `ResolveServerURL` trims or rejects these forms. Add a note to Story 1.1's implementation
  notes to document the expected contract (exact URL, no normalisation) or add a `strings.TrimRight`
  and a validation that the path ends in `/api` — the latter is less ergonomic but prevents a
  hard-to-debug "404 from connect" failure at runtime.
