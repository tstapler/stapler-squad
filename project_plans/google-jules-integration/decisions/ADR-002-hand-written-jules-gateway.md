# ADR-002: Hand-write a thin `jules/` REST gateway; no third-party SDK, no codegen

**Status**: Accepted
**Date**: 2026-09-01
**Project**: google-jules-integration

## Context

MVP needs three Jules endpoints: `ListSources`, `CreateSession`, `GetSession`.
Auth is a single `x-goog-api-key` header against `https://jules.googleapis.com/v1alpha`.

Options surveyed in `research/build-vs-buy.md`:

- **Official Google Go SDK** — does not exist. Jules is absent from
  `googleapis/google-api-go-client`; it is an alpha product outside the usual
  discovery-doc pipeline.
- **Community module `github.com/yuyu1815/jules-api`** — verified via
  `gh api repos/yuyu1815/jules-api`: 1 star, 0 forks, MIT, created 2025-10-04,
  last push 2025-10-08, single maintainer. Its README opens by apologizing for
  having previously presented itself as Google's official library.
- **OpenAPI codegen** — Google publishes no OpenAPI document for Jules, only
  prose REST docs. This repo has no codegen precedent for third-party REST
  (`grep` for `oapi-codegen`/`openapi-generator`/`swagger-codegen` and for
  `*openapi*.yaml|json` both came back empty); the only generation in the repo is
  protobuf/ConnectRPC for stapler-squad's *own* API, plus ent.

## Decision

Hand-write a thin gateway in a **new top-level `jules/` package** (sibling to
`github/`, `session/`, `server/` — the repo's convention for external-integration
adapters), using stdlib `net/http` + `encoding/json`, with no new `go.mod`
dependency.

Shape borrowed from the repo's existing precedents:

- **Overall skeleton** from `server/services/gemini_limits_client.go` — same
  vendor family, same header-based API-key auth, same
  build-request → decode-JSON → typed-result flow.
- **Cross-cutting concerns as an `http.RoundTripper` decorator** from
  `github/http_client.go`'s `rateLimitTransport`, so 429 handling covers every
  endpoint without per-call code.
- **Typed error classification from status codes** mirroring
  `classifyGHResponse`, producing sentinels (`ErrJulesNotConfigured`,
  `ErrJulesRateLimited`, `ErrJulesSessionNotFound`, `ErrJulesSourceNotRegistered`,
  `ErrJulesTransient`).

The package imports nothing from `session/` or `server/`. A test asserts this
(`go list -deps ./jules`) so the isolation boundary is enforced, not merely
intended.

Because the API is alpha and may drift, decoding is asserted against **recorded
JSON fixtures** in `jules/testdata/`, including a drift test using
`json.Decoder.DisallowUnknownFields()` that fails loudly when a new field
appears, rather than silently absorbing it into an out-of-date mock.

## Alternatives Considered

- **Import `yuyu1815/jules-api`** — Rejected. A dependency that is
  single-maintainer, ~11 months stale, essentially unused by anyone else, and
  which previously mislabeled its own provenance, is not a component to trust
  with a credential granting write access to every repo in the user's Jules
  account.
- **Author an OpenAPI spec, then codegen** — Rejected. Requires hand-writing the
  spec first (strictly more work than writing the ~150-250 line client), and would
  introduce the repo's first third-party-REST codegen toolchain plus a new
  generated-code `.gitignore` policy for a three-endpoint surface.
- **Shell out to a CLI, as `github/client.go` does for `gh`** — Not applicable.
  There is no `jules` CLI.

## Consequences

**Positive**

- Zero new dependencies; churn from an alpha API is confined to one directory.
- OTel tracing is available by wrapping the transport with `otelhttp.NewTransport`,
  using a dependency `go.mod` already has.
- Golden fixtures make schema drift a test failure with a named field, not a
  runtime surprise.

**Negative**

- Every future Jules endpoint must be written by hand. Acceptable: MVP needs
  three, and `sendMessage`/`approvePlan`/`Activities` are explicitly out of scope.
- Fixtures go stale unless someone re-records them. Mitigated by
  `jules/testdata/README.md` carrying the exact `curl` command and the recording
  date, and by the `jules unknown session state` log line firing at `Error` when
  the live API returns an enum value the code does not know.

## References

- `research/build-vs-buy.md` §1, §3, §4
- `research/stack.md` §1, §2, §3, §4
- `server/services/gemini_limits_client.go`, `github/http_client.go`, `github/rate_limit.go`
