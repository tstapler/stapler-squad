# Findings: Pitfalls

## Summary

Adding web-based new-device passkey registration via a one-time QR-code invite
URL introduces a cluster of security risks that are largely absent from the
existing CLI-only `print-qr-codes` flow. The new flow runs entirely in-browser
on an already-authenticated session, which means the invite-generation endpoint
becomes a CSRF target. The token itself travels through multiple untrusted
channels (screen, clipboard, URL bar, shared links), and the self-signed TLS CA
adds a mandatory bootstrapping step that browsers will block unless the new
device completes the CA-import dance first. The good news: the existing
`SetupManager` design (single in-memory token, constant-time comparison,
explicit `Consume` call) is a solid foundation — the new risks are almost
entirely at the layer above (HTTP handler behavior, invite generation gate, and
operational UX).

---

## Options Surveyed

The following mitigation strategies were evaluated:

### O-1: CSRF token on invite-generation endpoint
The browser posts a request to a new `POST /auth/invite/generate` endpoint while
the user is authenticated. Without a CSRF defence the endpoint is reachable from
any origin that can send a cross-origin POST (since `SameSite=Strict` on the auth
cookie provides partial but not full protection in some non-browser clients or
future browser policy changes).

Mitigation: require a CSRF token (double-submit cookie pattern or
`X-Requested-With` header check) on the generate endpoint, or verify that the
`Origin` / `Referer` header matches the server's own origin.

### O-2: Short token TTL + explicit revocation
The current `SetupManager` uses a 1-hour TTL. For a web-initiated invite that is
immediately delivered via QR code, 1 hour is longer than necessary. The
generating user should be able to cancel/revoke before the TTL expires, and the
TTL itself can be shortened (e.g. 15 minutes) to reduce the window after a QR
is photographed or a URL is forwarded.

### O-3: Single-use enforcement (already present) vs. "begin+finish" race
`SetupManager.Consume` is called in `finishRegistration` only after the full
WebAuthn ceremony completes. `IsValid` (non-consuming) is used in `isAuthorised`
for both `beginRegistration` and `finishRegistration`. This is correct, but a
race exists between two concurrent devices both calling `beginRegistration` with
the same token: both receive a ceremony challenge before either call to
`Consume`. The first device to finish consumes the token; the second device's
`finishRegistration` call will pass `isAuthorised` (the token is not yet consumed
when `isAuthorised` runs) but the `Consume` call in the handler will then
return false, meaning the second device's credential is already persisted (by
`FinishRegistration` in `webauthn.go`) before `Consume` is checked.

Mitigation: Move `Consume` to occur atomically before `FinishRegistration`
persists the credential, or add an explicit "begin consumes" step that reserves
the token.

### O-4: CA cert bootstrapping — explicit first step in UI
The invite URL is an HTTPS URL under the server's self-signed CA. Before the
new device can even load the page, the browser will show a hard TLS error unless
the CA cert has been imported. The invite flow must embed the CA cert download as
a mandatory, clearly-sequenced step that happens before the user navigates to
the invite URL.

Options:
- Include a separate "step 0" URL that serves the CA cert over plain HTTP (or
  a well-known port) so the device can import it, then visit the HTTPS invite URL.
- Display the CA fingerprint in the QR/invite UI for manual verification.
- Use Let's Encrypt / ACME on the server so no CA import is needed at all
  (requires the server hostname to be publicly resolvable — impractical for LAN).

### O-5: Rate limiting invite generation
An authenticated user (or a CSRF-confused browser) could generate many tokens in
rapid succession. Since only one token is active at a time (overwriting the
previous), each new generation silently invalidates the previous invite. This is
mostly benign but could be used as a denial-of-service against a legitimate
in-flight invite. Rate limiting (e.g., one invite per 5 minutes per session)
prevents both abuse and the usability foot-gun of a user re-generating and
sharing an already-expired-by-overwrite token.

### O-6: Session invalidation on credential revocation
When an admin revokes a passkey credential (`RemoveCredential`), any devices
authenticated with a session token that was originally created by that credential
remain authenticated for up to 30 days. There is no link between a stored
`authSession` and the credential that created it.

Mitigation: Store the credential ID alongside each `authSession` record.
`RemoveCredential` then calls `RevokeSessionsByCredential(credID)`.

### O-7: Credential store write-corruption resilience
`CredentialStore.save()` already uses an atomic rename (`passkeys.json.tmp` to
`passkeys.json`). This means a crash during `RemoveCredential` leaves either the
old file (no revocation visible) or the new file (revocation applied). There is
no half-written state. The `flock`-based file lock prevents concurrent writers.
However, the in-memory `cs.data` is updated before `save()` returns; if `Rename`
fails, the in-memory and on-disk states diverge until the next process restart.

### O-8: Token exfiltration surface reduction
The setup token appears in the URL query string (`?setup_token=<hex>`). This
means it can appear in:
- Browser history
- Server access logs
- HTTP `Referer` headers sent by subsequent navigation
- Screen captures / shoulder surfing
- Shared screenshots of the QR code

Mitigation: After a successful `finishRegistration`, the server should redirect
to a clean URL (no `setup_token` query param) so the token does not persist in
browser history. The QR code should be displayed only in a modal with a clear
"close" action, and the invite UI should warn users not to share screenshots.

---

## Trade-off Matrix

| Mitigation | Security Strength | UX Friction | Implementation Complexity |
|---|---|---|---|
| O-1: CSRF token on generate endpoint | High — blocks cross-origin forgery | Negligible (JS adds header) | Low — one header check |
| O-2: Short TTL (15 min) + revocation | Medium — reduces exfiltration window | Low — UX shows countdown timer | Low — constant in setupTokenTTL + cancel endpoint |
| O-3: Atomic begin-side consume | High — closes concurrent-registration race | None | Medium — refactor isAuthorised to reserve token |
| O-4: CA cert bootstrapping step | Critical for usability (not strictly security) | Medium — extra import step | Medium — requires two-phase invite flow in UI |
| O-5: Rate limit invite generation | Low-medium — prevents silent overwrite DoS | None for normal use | Low — in-memory counter per session |
| O-6: Credential–session binding | High — ensures revocation is immediate | None | Medium — add credentialID field to authSession |
| O-7: In-memory/disk divergence on rename failure | Low — rename rarely fails locally | None | Low — reload from disk after failed save |
| O-8: Token purge from URL + QR modal | Medium — reduces exfiltration surface | None | Low — redirect after finish; modal in UI |

---

## Risk and Failure Modes

### R-1: CSRF on invite generation
**Likelihood**: Medium. `SameSite=Strict` on the auth cookie prevents most
cross-site POST requests in modern browsers. However, the protection is
browser-dependent and does not cover non-browser API clients or edge cases
(top-level navigation followed by a POST from a newly-loaded attacker page in
some browser versions). A malicious page that tricks a logged-in user into
clicking a link or submitting a form could trigger invite generation.

**Impact**: The attacker receives no token (they cannot read the response body
due to CORS). But they cause the existing token to be overwritten, silently
cancelling any in-flight legitimate invite.

**Current state**: No explicit CSRF defence on any `/auth/*` endpoint. The
`SameSite=Strict` cookie attribute provides partial mitigation.

### R-2: Token replay after first use
**Current state**: `Consume` marks `used=true` in-memory. The `GetCeremony` call
in `FinishRegistration` also deletes the ceremony key atomically (single `delete`
inside mutex). These together prevent replay of a completed ceremony.

**Gap**: As noted in O-3, two concurrent `beginRegistration` calls with the same
setup token both succeed (both pass `IsValid`). The first `finishRegistration`
to call `Consume` wins. The second, which has already received a valid ceremony
challenge, can complete `FinishRegistration` (adding a credential to the store)
even though `Consume` returns false, because the `Consume` check in the handler
happens after `wa.FinishRegistration(...)` has already called `store.AddCredential`.

**Evidence in handlers.go**:
```go
token, err := h.wa.FinishRegistration(ceremonyKey, r)  // credential already added here
// ...
if setupToken := r.URL.Query().Get("setup_token"); setupToken != "" {
    h.setup.Consume(setupToken)  // checked after the fact; false return is silently ignored
}
```
If `Consume` returns false, the handler still calls `setAuthCookie` and returns
`{"ok": true}`. The second device registers a credential and gets a session with
no error.

### R-3: Credential orphaning after revocation
**Current state**: `RemoveCredential` removes the stored passkey. No sessions are
invalidated. A device that registered via the invite link retains a 30-day auth
session. Even after the operator removes the credential, that device can continue
to use the app until its session expires or is explicitly revoked via
`RevokeAllSessions` (which is a nuclear option affecting all devices).

**Impact**: There is no targeted revocation path: you cannot revoke only the
sessions for credential X without also invalidating every other device.

### R-4: CA cert bootstrapping failure
**Current state**: The server generates a self-signed CA and serves the CA cert
at `/auth/ca.pem`. The invite URL is an HTTPS URL under this CA. A new device
that navigates to the invite URL before importing the CA cert sees a browser
hard-stop TLS error. On iOS/Android the user may be unable to proceed at all
without following a multi-step trust-store installation process.

**The invite flow must present the CA cert download as step 0**, before the user
attempts to visit the HTTPS URL. This could be:
- A plaintext HTTP link to `/auth/ca.pem` (acceptable since the CA cert is
  public-key material, not a secret) included alongside the QR code.
- A separate QR code displayed first: "Scan this to install the CA, then scan
  the invite QR."
- A fingerprint of the CA cert shown so the user can verify they installed the
  right cert.

Failure to address this will result in 100% failure rate for new device
registration unless the device somehow already trusts the CA.

### R-5: Token exfiltration via URL / screen
**Current state**: The setup token appears as a plain query parameter in the URL.
It is 32 hex characters (128 bits of entropy), which is sufficient to prevent
brute-force. The real exfiltration surface is:
- Server access logs (if the server logs full URLs, the token appears there)
- Browser history on the new device after visiting the invite URL
- `Referer` header if any third-party resource is loaded on the registration page
- Screen captures / photo of the QR code

**Likelihood of abuse**: Low in the LAN scenario (attacker must be on the same
network and capture the token within 1 hour). Higher if the invite URL is shared
via insecure channel (SMS, email, Slack).

### R-6: Race condition — concurrent invite generation
**Current state**: `SetupManager` is protected by a mutex. Concurrent calls to
`Init()` or `GenerateToFile()` are serialised. However, if the web UI generates
a new invite while the previous one is being used (device B is mid-ceremony when
device A generates a new token), the in-flight ceremony is unaffected (it uses
the ceremony key for the finish step), but the setup token validation for device
B's `isAuthorised` check will now fail (new token value, old token rejected).
Device B's `finishRegistration` call returns 401 even though its
`beginRegistration` succeeded.

**Impact**: Broken invite experience for the concurrent registrant; not a
security vulnerability.

### R-7: Credential store integrity on corrupt write
**Current state**: `save()` uses atomic rename. An interrupted write leaves the
`.tmp` file orphaned but the production file intact (or fully replaced). This is
robust for the common crash case.

**Gap**: If `os.Rename` itself fails (e.g. cross-device rename on some
filesystems), the `.tmp` file contains the intended new state, the production
file contains the old state, but `cs.data` in memory already reflects the new
state. Subsequent reads from disk (after process restart) would see the old state,
silently un-revoking a credential. A `log.Error` is emitted but no recovery
action is taken.

**Likelihood**: Very low on local filesystems. Higher if the config dir is on a
network mount or tmpfs.

---

## Migration and Adoption Cost

The new feature adds a single new endpoint (`POST /auth/invite/generate` or
equivalent) and a UI panel to the settings page. Existing CLI-based `setup-token`
flow is unaffected. The `SetupManager` reuse keeps server-side changes small.

Key adoption costs:
- **CA cert import UX**: Every new-device user must perform a one-time OS/browser
  trust-store operation. This is unavoidable with a self-signed CA and is the
  single highest-friction element of the flow. Clear, platform-specific
  instructions (Android, iOS, macOS, Windows) must be built into the UI.
- **Credential–session binding** (O-6): Requires adding a `credentialID` field to
  `authSession` and a new `RevokeSessionsByCredential` method. Existing sessions
  (created before this field is added) will not be revocable per-credential; a
  migration comment noting this is sufficient.
- **`finishRegistration` handler refactor** (O-3 race fix): Requires moving the
  `Consume` call to precede `wa.FinishRegistration` and failing the handler if
  consumption fails. Small change, low risk.

---

## Operational Concerns

- **Token visibility in server logs**: If request URLs are logged at INFO level,
  setup tokens will appear in `~/.stapler-squad/logs/stapler-squad.log`. The log
  should redact query parameters on `/auth/*` routes, or the invite URL should
  use a POST body / fragment instead of a query parameter.

- **QR code on shared screens**: The stapler-squad UI may be visible on a shared
  display (screen share, projector). The invite QR code should be rendered inside
  a modal that requires a deliberate click to open, not shown inline on the
  settings page permanently.

- **Token overwrite on re-generation**: If the user generates a second invite
  before the first is used, the first invite silently stops working. The UI
  should warn: "Generating a new invite will invalidate the current one."

- **CA cert rotation**: When the TLS certificate is regenerated (due to hostname
  change or 2-year expiry), the CA cert changes. Devices that imported the old
  CA cert must re-import. The CA cert itself is valid for 10 years
  (`generateCA` sets `NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour)`),
  so CA rotation is infrequent, but the admin should be notified when it occurs.

- **Single-user assumption**: The system assumes a single owner
  (`ownerUserID = "stapler-squad-owner"`). All registered passkeys are attached
  to this one user. This means credential revocation is a manual operation (no
  per-credential metadata like device name). The invite UI should prompt the user
  to assign a label to each registered credential so revocation is actionable.

---

## Prior Art and Lessons Learned

- **WebAuthn invite-link patterns** [TRAINING_ONLY — verify]: Products like
  1Password and Bitwarden use short-lived (often 7-15 minute) invite tokens
  delivered via email, not URL fragments visible in browser history. The token is
  consumed server-side before the credential ceremony begins (not after), closing
  the concurrent-registration race.

- **Self-signed CA bootstrapping on mobile** [TRAINING_ONLY — verify]: Apple
  requires a Configuration Profile (`.mobileconfig`) for iOS CA installation;
  a raw `.pem` file opened in Safari triggers a prompt but the installation path
  changed in iOS 16+. Android requires navigating to Settings > Security >
  Install certificates. Both flows are well-documented but require OS-specific
  guidance text in the UI.

- **SameSite=Strict is not a complete CSRF defence**: CSRF tokens or
  `Origin`-header checks remain best practice on state-mutating endpoints, because
  `SameSite=Strict` does not protect against cross-site attacks initiated from the
  same registrable domain, and browser implementation has historically varied for
  top-level navigations that are followed immediately by POSTs.
  [TRAINING_ONLY — verify with current OWASP CSRF cheat sheet]

- **Atomic single-use token race**: A well-known pattern for one-time tokens is
  to use a database `UPDATE ... WHERE used=false RETURNING id` (or equivalent
  compare-and-swap) to atomically mark the token used and detect the winner.
  The in-memory mutex in `SetupManager` provides equivalent atomicity for a
  single-process server; the race in the current code is above the mutex layer
  (between HTTP handler steps), not inside `SetupManager` itself.

---

## Open Questions

1. Should the invite URL use an HTTP fragment (`#setup_token=...`) instead of a
   query parameter to prevent the token from appearing in server access logs and
   `Referer` headers? (Fragments are not sent to the server, but are accessible
   to JavaScript on the page.)

2. Is a 15-minute TTL sufficient for the typical new-device registration UX,
   given that the user must first install the CA cert (which can take 2-5 minutes
   on mobile)?

3. Should credential labels (device names) be stored in `CredentialStore`
   alongside the `storedCredential` struct, or managed separately?

4. Should `RevokeAllSessions` be renamed or split so operators can revoke
   sessions for a single credential without logging out all other devices?

5. What is the intended behaviour when the server's hostname changes and the TLS
   CA is regenerated — should all existing passkeys and sessions be invalidated,
   or only sessions from remote devices?

6. Is there a requirement for audit logging of invite generation (who generated,
   when, whether it was used), separate from the existing `log.InfoLog` messages?

---

## Recommendation

### MUST implement

1. **CSRF defence on `POST /auth/invite/generate`** (O-1): Check that the
   `Origin` header matches the server's own origin, or add an `X-Requested-With:
   XMLHttpRequest` requirement. One-liner in the handler; prevents the silent
   token-overwrite CSRF scenario.

2. **Fix concurrent-registration race** (O-3): Move `setup.Consume(setupToken)`
   to run before `h.wa.FinishRegistration(...)` in `finishRegistration`, and
   return 401 if Consume returns false. This ensures at most one credential is
   registered per invite token.

3. **CA cert bootstrapping as step 0 in the invite UI** (O-4): The invite modal
   must display the CA cert download link (and fingerprint) before showing the
   invite QR code, with platform-specific import instructions. Without this the
   feature will not work for any new device.

4. **Token purge from URL after use** (O-8): After successful `finishRegistration`
   redirect the browser to a clean URL that does not contain `setup_token` in the
   query string. Prevents token persistence in browser history and `Referer`
   leakage.

### SHOULD implement

5. **Credential–session binding for targeted revocation** (O-6): Add
   `credentialID []byte` to `authSession`; implement `RevokeSessionsByCredential`.
   Without this, revoking a passkey does not immediately log out the associated
   device.

6. **Short TTL (15 minutes) with countdown timer in UI** (O-2): Change
   `setupTokenTTL` to 15 minutes for web-generated invites (keep 1 hour for
   CLI-generated ones). Show a countdown in the invite modal. Add a "Cancel
   invite" button that calls a revocation endpoint.

7. **Warn user on invite re-generation** (O-5 / operational): The UI must warn
   that generating a new invite invalidates the current one. This prevents the
   usability failure of a user sharing an already-overwritten token.

8. **Redact setup tokens from server access logs**: Do not log full request URLs
   on `/auth/*` endpoints at INFO level, or use a redaction wrapper.

### NICE TO HAVE

9. **Rate limiting on invite generation** (O-5): Maximum one new invite per
   session per 5 minutes. Low implementation cost, prevents abuse scenarios.

10. **Credential labels** (device names): Store a human-readable label with each
    credential so revocation is actionable without examining raw credential IDs.

11. **In-memory/disk divergence recovery on rename failure** (O-7): On `Rename`
    error, reload `cs.data` from the original file so in-memory state does not
    diverge. Low probability event but easy defensive fix.

12. **QR code display in a dismissible modal only** (operational): Never show the
    invite QR inline on a persistently-visible settings panel; require a deliberate
    user action to display it, minimising screen-capture exposure.

---

## Pending Web Searches

The following queries should be run to verify training-knowledge claims and fill
gaps:

1. `WebAuthn one-time invite token race condition concurrent registration FIDO2`
   — Confirm whether the FIDO2 spec addresses concurrent begin/finish with the
   same server session data.

2. `iOS 16 17 install CA certificate .pem mobileconfig Safari`
   — Verify current Apple procedure for importing a root CA cert on iOS 16+;
   confirm whether a `.pem` download from Safari still triggers the profile
   install prompt.

3. `Android 13 14 install user CA certificate settings path`
   — Confirm the current Android settings path for user CA installation and
   whether Android 14 changed the trust-store UX.

4. `SameSite=Strict CSRF protection limitations top-level navigation 2024`
   — Verify current browser behaviour and whether `SameSite=Strict` alone is
   considered sufficient by OWASP for state-mutating endpoints.

5. `WebAuthn passkey invite link URL fragment vs query param security`
   — Research whether using `#fragment` for the setup token (to avoid server log
   exposure) is a recognised pattern and what its trade-offs are.

6. `go-webauthn webauthn credential session concurrent race condition`
   — Check if the `go-webauthn/webauthn` library itself provides any
   concurrent-ceremony protection at the library level.

---

## Web Search Results

### iOS CA cert install (query 2)
- `.pem` downloads on iOS 17 do NOT reliably trigger a profile install prompt. Multiple Apple Community threads confirm that Safari only shows share options, not an install dialog, for raw `.pem` files.
- Correct flow: (1) Download `.pem` → Settings appears in Safari "downloaded" but no install prompt. (2) User must go to Settings → General → VPN & Device Management → tap downloaded profile → Install. (3) Then separately: Settings → General → About → Certificate Trust Settings → enable full trust.
- **Implication**: The invite modal MUST include explicit step-by-step CA cert instructions; a bare download link is not sufficient on iOS.
- Source: https://discussions.apple.com/thread/255752672, https://support.apple.com/en-au/102390

### Android 14 CA cert (query 3)
- Android 14 hardened system cert store (APEX path now immutable even with root). **User-installed CAs still work** via: Settings → Security & privacy → More security settings → Encryption & credentials → Install a certificate → CA certificate.
- Android 7+ apps can opt out of trusting user CAs via network security config — browsers (Chrome, Firefox) trust user CAs by default.
- Source: https://httptoolkit.com/blog/android-14-breaks-system-certificate-installation/

### SameSite=Strict CSRF (query 4)
- OWASP 2024: SameSite=Strict alone is **not** considered sufficient for state-mutating endpoints. Must be combined with CSRF token or Origin/Referer header verification for defense-in-depth.
- Existing `cs_auth` cookie uses `SameSite=Strict` — the invite-generation endpoint MUST also verify the `Origin` header.
- Source: https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html

### URL fragment vs query param (query 5)
- URL fragments are NOT sent to the server and do NOT appear in server logs or proxies — better for sensitive tokens.
- However: fragments can't be read by server-side code. Since setup_token must be passed to `/auth/register/begin` and `/auth/register/finish` (server reads it), query param is required for server-side validation.
- Mitigation: redirect to a clean URL after successful registration to strip the token from browser history (already listed as MUST-fix in pitfalls).
- Source: https://owasp.org/www-community/vulnerabilities/Information_exposure_through_query_strings_in_url

### go-webauthn concurrent ceremonies (query 6)
- go-webauthn does **not** provide built-in concurrent ceremony isolation. Session data (`*SessionData`) is managed entirely by the application. The library's `FinishRegistration` only validates the ceremony data passed to it — it has no knowledge of in-flight ceremonies or concurrent calls.
- Confirms: the race condition (two devices using one invite token both successfully registering) is a real application-level bug, not mitigated by the library.
- Source: https://pkg.go.dev/github.com/go-webauthn/webauthn/webauthn
