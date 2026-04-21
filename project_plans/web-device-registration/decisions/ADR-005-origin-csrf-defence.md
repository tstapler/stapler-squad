# ADR-005: Origin Header Check as CSRF Defence on Invite Generation

**Status**: Accepted
**Date**: 2026-04-21
**Feature**: Web Device Registration

---

## Context

`POST /auth/invite/generate` is a state-mutating endpoint that is called from an authenticated browser session. If an attacker can cause a logged-in user's browser to send this POST request to the server (cross-site request forgery), the attacker can:

1. Silently overwrite the current invite, cancelling any in-flight legitimate invite.
2. (In theory) cause the server to generate a new token — but the attacker cannot read the response body due to CORS, so they cannot steal the new token.

The primary CSRF concern is therefore availability (silent invite cancellation), not credential theft.

The existing `/auth/*` endpoints set cookies with `SameSite=Strict`. The existing logout and registration ceremony endpoints do not add additional CSRF defences beyond `SameSite=Strict`.

OWASP (2024 CSRF Cheat Sheet, confirmed via research in findings-pitfalls.md) states that `SameSite=Strict` alone is not sufficient as a CSRF defence for state-mutating endpoints, because:
- `SameSite=Strict` does not protect against attacks from the same eTLD+1 (registrable domain).
- Historical browser implementation has been inconsistent for some top-level navigation scenarios.
- Non-browser API clients (curl, Postman, custom scripts) are not subject to the `SameSite` cookie policy.

---

## Decision

Add an `Origin` header verification check to `POST /auth/invite/generate`. Before processing the request, the handler verifies that the `Origin` header matches the server's own HTTPS origin (derived from `primaryDomain` + `port`).

```go
func (h *httpHandlers) verifyOrigin(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        // Some browsers omit Origin on same-origin requests. Allow if Referer matches.
        referer := r.Header.Get("Referer")
        return strings.HasPrefix(referer, h.httpsOrigin())
    }
    return origin == h.httpsOrigin()
}

func (h *httpHandlers) httpsOrigin() string {
    return fmt.Sprintf("https://%s:%d", h.primaryDomain, h.port)
}
```

If `verifyOrigin` returns false, the handler returns HTTP 403 Forbidden.

This check is applied only to `POST /auth/invite/generate`. Existing handlers (`logout`, `register/begin`, etc.) are not modified — they are out of scope for this feature.

---

## Rationale

**Defense in depth per OWASP 2024.** `SameSite=Strict` provides a strong first layer but is not sufficient alone. Adding an `Origin` header check is a well-established second layer (OWASP CSRF Cheat Sheet — "Verifying Origin with Standard Headers"). It costs one `r.Header.Get("Origin")` call and one string comparison per request.

**Low implementation complexity.** The `Origin` check is a ~10-line helper. No CSRF token infrastructure (storage, rotation, transmission in forms) is needed. The frontend already includes `credentials: "include"` on all auth fetch calls; browsers automatically include the `Origin` header on cross-origin `POST` requests with credentials.

**Protects the highest-value target.** Of all the new endpoints, `POST /auth/invite/generate` is the only one that creates new server state and could affect a concurrent legitimate user action (invite cancellation). `GET /auth/credentials` is read-only. `POST /auth/credentials/{id}/revoke` is also a candidate but is a lower-priority hardening target.

**Localhost bypass preserved.** When `isLocalhostRequest(r)` is true, the `Origin` check is skipped (localhost development environment). This matches the existing pattern for all `/auth/*` handlers.

---

## Consequences

**Accepted costs:**
- The `httpHandlers` struct needs `port int` to construct `httpsOrigin()`. This field is already required for invite URL generation (see research synthesis) and is added as part of the invite feature regardless.
- If a client calls `POST /auth/invite/generate` without an `Origin` header and without a matching `Referer`, the request is rejected with 403. This is the intended behaviour for non-browser API clients; legitimate browser requests always include `Origin` on cross-origin POSTs.

**Not accepted:**
- Double-submit cookie CSRF token. Requires generating, storing, and validating a separate token on every form interaction. Disproportionate for a single endpoint.
- Omitting the check on the basis that `SameSite=Strict` is sufficient. OWASP explicitly recommends against relying solely on `SameSite`.

---

## Alternatives Rejected

**`X-Requested-With: XMLHttpRequest` header check**: This is a legacy pattern that predates `SameSite`. It provides weaker protection than `Origin` verification because custom headers can be set in some cross-origin scenarios. `Origin` verification is the current OWASP-recommended approach.

**Full CSRF token (double-submit cookie or synchronizer token pattern)**: Provides stronger protection but introduces token generation, rotation, and validation infrastructure. Disproportionate for a single-user tool where the primary CSRF risk is invite cancellation, not credential theft.

**No additional CSRF defence (rely on `SameSite=Strict` only)**: Rejected per OWASP 2024 guidance. `SameSite=Strict` is a necessary but not sufficient CSRF control for state-mutating endpoints.
