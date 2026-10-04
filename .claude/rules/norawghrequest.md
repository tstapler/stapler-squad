---
globs:
  - "github/*.go"
---

# Build GitHub Requests via `NewConditionalRequest`/`NewConditionalRequestNoCache`/`newGHRequestForHostWithToken`, Not Raw `http.NewRequest`

Every native GitHub REST/GraphQL call must build its `*http.Request` through one of `github.NewConditionalRequest`, `github.NewConditionalRequestNoCache`, or `newGHRequestForHostWithToken` (`github/http_client.go`) — never `http.NewRequest`/`http.NewRequestWithContext` directly against a `github.GhBaseURL()`/`RestBaseURLForHost()`-derived URL. (`newGHRequest`, the github.com-only, non-host-aware predecessor to `newGHRequestForHost`/`newGHRequestForHostWithToken`, was removed once GHE-aware callers migrated off it — see `newGHRequestForHost` for the current single-host-arg entry point.)

**Wrong:**
```go
func fetchSomething(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GhBaseURL()+path, nil) // bypasses ETag plumbing
	if err != nil {
		return nil, err
	}
	return ghHTTPClient.Do(req)
}
```

**Right:**
```go
func fetchSomething(ctx context.Context, path string, cache *ETagCache) (*http.Response, error) {
	req, err := NewConditionalRequest(ctx, path, cache)
	if err != nil {
		return nil, err
	}
	return ghHTTPClient.Do(req)
}
```

If a call site genuinely cannot use the wrapper (e.g. it needs a request shape the constructor doesn't support yet), add `//nolint:norawghrequest` with a one-line justification rather than reaching for `http.NewRequest` silently.

## Why

`GetPRInfoConditional` (`github/etag_cache.go`) sends an `If-None-Match` header so GitHub can answer with a `304 Not Modified` that costs zero rate-limit quota when a PR hasn't changed — the codebase's main defense against exhausting the primary rate limit under frequent polling. A raw `http.NewRequest(WithContext)` builds a request with no `If-None-Match` header at all: it compiles, it passes tests (a `200` body is still a valid response), and it silently forces a full-cost request on every call forever. Nothing short of reading the diff line-by-line catches the omission — the same silent-but-wrong failure shape `instance-lock-free-reads.md` describes for an unguarded `i.Path` read.

The `norawghrequest` analyzer (`tools/lint/norawghrequest`, wired into `make lint-custom`) makes this structural: it flags any `http.NewRequest`/`http.NewRequestWithContext` call whose URL resolves to `GhBaseURL()`/`RestBaseURLForHost()`, outside the approved constructors' own calls (`newGHRequestForHostWithToken`, `NewConditionalRequest`, `NewConditionalRequestNoCache`, `newGHGraphQLRequestForHostWithToken` — exempted by function-declaration containment, mirroring `tools/lint/norawgitopen`'s precedent for `session/git.OpenRepo`).
