# ADR-001: Use brace-scan pattern for ParseHeadlessTriageResult

**Date**: 2026-06-23
**Status**: Accepted

## Context

`ParseHeadlessTriageResult` in `session/backlog_triage.go` strips only leading
triple-backtick fences before calling `json.Unmarshal`. The triage prompt instructs
Claude to run 4 parallel research subagents and then synthesize output — a multi-step
flow that commonly produces preamble prose before the final JSON block. The leading-fence
strip fails in this case: `json.Unmarshal` receives the preamble + JSON and fails with
a parse error, leaving `triage_result` empty and the UI stuck on "failed".

## Decision

Replace the fence-strip approach with the brace-scan pattern already used by
`ParseHeadlessVerdictResult` in `session/backlog_review.go`:

```go
start := strings.Index(raw, "{")
end := strings.LastIndex(raw, "}")
```

This finds the first `{` and last `}` in the entire output string, extracting the
outermost JSON object regardless of any surrounding text.

## Rationale

- The pattern already exists in this codebase (`ParseHeadlessVerdictResult`) and is
  proven in production for the review gate flow.
- Using `strings.LastIndex` for `}` correctly selects the FINAL JSON block in output
  that has multiple JSON fragments (e.g., intermediate research JSON followed by the
  final synthesis JSON).
- No new dependencies. No regex. Simple and auditable.
- The triage prompt says "output ONLY a JSON object" — the model generally complies
  for the outermost object, so brace-scan is reliable in practice.

## Trade-offs / Risks

- **Fragile for deeply nested JSON with trailing content**: If the final character
  after the real JSON is a `}` from a nested object, `strings.LastIndex` would still
  find the correct position because `}` at the outermost level IS the last `}`.
  However, if the model appends prose after the closing `}`, the scan would capture
  the last `}` in the prose, not the JSON's `}`. In practice this has not occurred
  with the review gate pattern.
- This is the same risk accepted for `ParseHeadlessVerdictResult`. Consistency
  outweighs the marginal risk.

## Alternatives Considered

- **`json.Decoder` stream scan**: More correct but adds code complexity and the
  `json.Decoder` approach for partial streams is non-trivial in Go.
- **Regex extraction**: More fragile and harder to read than the brace scan.
- **Prompt change to guarantee pure JSON**: Would require re-running existing
  triage sessions. The parser fix is more robust independently of prompt behaviour.
