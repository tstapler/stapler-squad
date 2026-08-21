# Findings: Features

**Date**: 2026-04-21
**Scope**: UX patterns for multi-device enrollment — QR code, invite links, one-time tokens, credential management
**Input**: `project_plans/web-device-registration/requirements.md`, training knowledge of Tailscale, Bitwarden/Vaultwarden, GitHub, GitLab, and open-source WebAuthn UIs

---

## Summary

Four UX patterns exist for enrolling a second device: invite link only, QR code only, QR + link together, and email/push invite. Survey of comparable tools leads to three key findings:

1. **QR + link is the established best practice for LAN/self-hosted tools.** Tailscale, Vaultwarden, and the existing `ssq print-qr-codes` CLI all show QR + URL side-by-side. No comparable tool uses only one of the two. The redundancy is intentional: QR for nearby devices (phone scan), URL for remote/copy-paste scenarios.

2. **One-time tokens with short expiry are universally used.** Every surveyed system generates a short-lived token (minutes to hours) embedded in the invite URL. Tailscale auth keys can be reusable but default to single-use; Bitwarden's device approval is ephemeral; GitHub SSH enrollment has no token at all (key is added persistently). The one-time + expiry pattern is the correct baseline for a security-sensitive flow.

3. **Credential list + revoke is table-stakes for passkey management.** GitHub, Bitwarden, and every WebAuthn reference implementation expose a credential list with display name and creation date, plus per-credential revocation. The requirements already include this; it is confirmed to be necessary and expected by users who manage multiple devices.

---

## Options Surveyed

### Option A: Invite Link Only (copy/paste URL)

The generated URL (`https://host/login?setup_token=<token>`) is displayed as text with a copy button. No QR code is generated or shown.

**Who does this**: GitHub/GitLab SSH key enrollment does not use tokens at all — the user adds a key persistently from any device. Some simple WebAuthn demos (e.g., `webauthn.io`) provide only a share URL for cross-device passkey transfer. [TRAINING_ONLY — verify webauthn.io cross-device flow]

**Pros**:
- No dependency on a QR code generation library or image rendering
- Works perfectly for remote (non-collocated) device enrollment: paste link into a chat, email, or password manager
- Simpler UI — single text field + copy button

**Cons**:
- Unusable for the primary use case (phone enrollment on the same LAN): typing a 64-character URL with a setup token on a mobile keyboard is error-prone
- No visual representation increases the chance of token interception going unnoticed
- Requires copy/paste between devices, which implies a channel (clipboard sync, messaging app) that may not exist in all setups

**Verdict for Stapler Squad**: Insufficient alone. The requirements explicitly call for QR code because the primary use case is enrolling a phone in the same room as the server.

---

### Option B: QR Code Only (scan from authenticated device)

A scannable QR code is generated encoding the full registration URL. No raw URL is shown.

**Who does this**: The existing `ssq print-qr-codes` CLI is effectively QR-only (it renders to the terminal, which cannot be copy-pasted into a browser address bar). Some physical access workflows (printer labels, physical setup guides) use QR-only.

**Pros**:
- Frictionless for nearby device enrollment: point camera, tap notification, done
- QR codes are difficult to photograph accurately from a distance, which provides a weak form of physical proximity assurance
- The current CLI behavior, so it matches user muscle memory

**Cons**:
- Fails entirely for remote device enrollment: if the user is not physically near a screen showing the QR, they cannot scan it
- Screen readers and accessibility tools cannot interact with a QR code
- Copy-paste workflows (e.g., enrolling a VM, a headless device, or a device on a different network via Tailscale) are impossible

**Verdict for Stapler Squad**: Insufficient alone. Breaks the remote enrollment use case. The requirements explicitly require both.

---

### Option C: QR + Link Together (our chosen approach)

Both a scannable QR code and the raw URL are displayed simultaneously. The URL has a copy button. Both encode the same one-time setup token. An expiry countdown is shown below both.

**Who does this**:

**Tailscale**: The Tailscale admin console's "Auth Keys" section generates a key that can be used as `tailscale up --authkey=<key>`. The key is shown as text only (for CLI use), not as a QR, because Tailscale targets developer/server enrollment. However, Tailscale's mobile app enrollment uses a QR code flow: in the admin console, "Add device via mobile" shows a QR + URL pair for scanning. [TRAINING_ONLY — verify exact Tailscale mobile enrollment UI]

**Vaultwarden / Bitwarden**: The Bitwarden mobile app and Bitwarden's emergency access feature use a combination of a numeric code and a link. When adding an organization device, Bitwarden shows a QR code alongside a manual entry option. The emergency access grant uses a URL sent via email rather than QR. [TRAINING_ONLY — verify Bitwarden new-device approval flow specifics]

**Passkey cross-device authentication (FIDO2 hybrid transport)**: The W3C WebAuthn spec and FIDO2 "hybrid transport" (formerly "caBLE") flows use a QR code displayed on the relying party's page (desktop browser) that encodes a tunnel URL. The authenticator device (phone) scans the QR and establishes a proximity-verified Bluetooth/BLE tunnel to the desktop. This is the canonical "second device enrollment" pattern in the WebAuthn ecosystem as of 2023. [TRAINING_ONLY — verify current FIDO2 hybrid transport / WebAuthn Level 3 specification status]

**Proxmox VE two-factor enrollment**: The Proxmox web UI (a comparable self-hosted single-user admin tool) shows a QR code for TOTP enrollment alongside the manual entry secret. No URL is involved, but the QR + manual fallback pairing is the same pattern.

**FreeIPA / Keycloak OTP enrollment**: Both identity management systems show QR + manual key side-by-side on their TOTP setup screens, explicitly labeled "scan QR or enter manually." This is the established idiom for authentication enrollment in admin tools.

**Pros**:
- Handles both nearby (phone scan QR) and remote (copy URL) scenarios in one UI
- Matches the established pattern from Tailscale mobile enrollment, Proxmox, Keycloak, and FIDO2 hybrid transport
- No need for the user to choose which mode to use — both are always available
- The expiry countdown adds urgency context without requiring the user to understand token mechanics
- Matches user expectations set by the existing CLI (which also prints two QR codes and a URL)

**Cons**:
- More complex UI than either A or B alone — requires layout space for both QR image and URL text
- QR code generation requires an HTTP endpoint (`/auth/invite/qr.png` or equivalent); adds a backend roundtrip or base64 inline response
- Two-column or stacked layout needed; mobile layout requires care to ensure QR code is large enough to scan (minimum ~200×200px on screen)

**Verdict for Stapler Squad**: Confirmed correct. This is the industry standard for self-hosted tools, matches both use cases in the requirements, and aligns with the existing CLI behavior.

---

### Option D: Email / Push Invite

A registration link is sent to a known email address or pushed as a notification to a previously-enrolled device.

**Who does this**: Bitwarden sends emergency access grant links via email. Duo Security's device enrollment sends SMS/push to an enrolled device. Apple's "Trusted Device" and Google's "This was me" confirmations push to enrolled devices. GitHub sends email confirmation for new SSH key additions.

**Pros**:
- Eliminates the need for the user to be looking at the server UI at the time of enrollment
- Async flow: generate invite, pick up the link later on the new device
- Email copy provides a durable record

**Cons**:
- Requires an email/SMTP integration or a push notification channel — neither exists in Stapler Squad, and both are out of scope
- For a single-user self-hosted app with no external accounts, email introduces an unnecessary dependency
- Async nature means the token must be longer-lived (hours to days), increasing replay window
- For a LAN-only or Tailscale-only server, email may not be configured and may not route correctly

**Verdict for Stapler Squad**: Out of scope. No SMTP/push infrastructure. The synchronous QR + link flow (Option C) is the correct approach for this deployment model.

---

## Trade-off Matrix

| Option | Usability — Nearby Device (phone scan) | Usability — Remote Device (copy/paste) | Security (CSRF/replay risk) | Implementation Complexity |
|---|---|---|---|---|
| A: Link only | Low — must type long URL on mobile | High — copy/paste works well | Medium — token in URL; clipboard exposure | Low |
| B: QR only | High — one camera tap | None — cannot use remotely | Low — QR is ephemeral, hard to intercept remotely | Medium (QR endpoint needed) |
| C: QR + Link | High — camera tap | High — copy button | Medium — token in URL; mitigated by one-time + expiry | Medium-High (both UI elements + endpoint) |
| D: Email/Push | Medium — out-of-band delay | High (async) | Low-Medium (link is in email, requires email account) | Very High (SMTP/push infra needed) |

**Security note on Option C**: The setup token in a URL is a bearer credential. Risk mitigations already present in the requirements: one-time use (token invalidated after first use), short expiry (1-hour window matching existing CLI), and token regeneration (invalidates previous token). HTTPS is required on all Stapler Squad connections (TLS with self-signed CA), so the URL is encrypted in transit. The primary residual risk is token interception via browser history or shared clipboard; the expiry window limits exposure time.

---

## Risk and Failure Modes

### R1: QR code too small to scan on the screen it is displayed on
**Risk**: If the server UI is accessed from a small-viewport device (phone) and the user attempts to scan the QR from a second phone, the QR code image may be too small. The existing `GenerateQRPNG` produces 256×256 pixels; at high DPI this may render as approximately 80–100 CSS px, which is below the recommended minimum (~150px) for reliable scanning.
**Likelihood**: Medium — mainly an issue if the server UI is accessed from a phone to enroll another phone (unusual but possible).
**Mitigation**: Render the QR at a minimum CSS width of 200px (the `GenerateQRPNG` output is already 256px; ensure no CSS downscaling). Add a "Download QR" link as a fallback for when the on-screen QR is difficult to scan.

### R2: One-time token race condition (two enrollments from same invite)
**Risk**: If a user opens the registration URL in two tabs simultaneously (or a bot crawls the URL), both requests will arrive before the first invalidates the token.
**Likelihood**: Low — self-hosted single-user system; no automation expected.
**Mitigation**: The existing `SetupManager` should use a compare-and-swap or file lock when invalidating the token. The current `setup-token.json` pattern is file-based; verify that token invalidation is atomic (write new state before returning 200). [See pitfalls research for detailed analysis.]

### R3: Expired token shown without UI update (countdown stops but page stays open)
**Risk**: If the user loads the `/account` invite page and leaves it open past expiry, the countdown reaches zero but the token is still displayed. A user who scans the QR after expiry gets a 401 with no clear explanation.
**Likelihood**: Medium — plausible in real use (user is interrupted).
**Mitigation**: When countdown reaches zero, either: (a) automatically regenerate the token (simplest UX), or (b) replace the QR with an "Expired — regenerate" state. Option (b) is safer; the user should explicitly acknowledge they need a fresh token. The registration endpoint should return a clear error message on expired token (not just 401) so the new device gets actionable feedback.

### R4: CA cert QR missing from web invite
**Risk**: The existing CLI `print-qr-codes` prints two QR codes: one for the CA certificate download and one for the registration URL. The requirements note the CA cert URL must still be included. If the web invite omits the CA cert QR, new devices cannot install the self-signed certificate and the HTTPS connection will fail before they can even load the registration page.
**Likelihood**: Certain if not explicitly included — it is a distinct requirement from the registration QR.
**Mitigation**: The invite UI must display both QR codes side-by-side (or in a two-step flow): step 1 — install CA cert QR, step 2 — registration URL QR. This matches the existing CLI behavior.

### R5: Token regeneration revokes an in-progress enrollment
**Risk**: If a user regenerates the token while a second device is mid-enrollment (has scanned the QR but has not yet completed `register/finish`), the new device will fail at `register/finish` with a token mismatch.
**Likelihood**: Low but not negligible.
**Mitigation**: Display a warning on the regenerate button: "Regenerating will cancel any in-progress enrollment." Consider a short delay (3–5 seconds) with an undo option, or require confirmation.

### R6: Credential revocation orphaning the only registered passkey
**Risk**: The credential management UI allows revoking any credential. If the user revokes all credentials (or the only remaining one), they will be locked out of the system entirely and will need CLI access to recover — exactly the scenario this feature is meant to avoid.
**Likelihood**: Low — but permanently locked-out is a severe failure.
**Mitigation**: Prevent revoking the last credential via a UI guard (disable the revoke button on the last remaining entry, or show a confirmation warning: "This is your only passkey. Revoking it will require CLI access to recover.").

---

## Migration and Adoption Cost

| Work item | Estimated effort | Dependencies |
|---|---|---|
| `/auth/invite` endpoint (generate token, return JSON with token + expiry) | 0.5 day | `server/auth/setup.go` SetupManager |
| `/auth/invite/qr.png` endpoint (QR PNG for registration URL) | 0.5 day | `server/auth/qrcode.go` GenerateQRPNG |
| `/auth/invite/ca-qr.png` endpoint (QR PNG for CA cert URL) | 0.5 day | Same as above |
| `/auth/credentials` list + revoke endpoints | 1 day | `server/auth/store.go` credential CRUD |
| `/account` React page (invite card + credential list) | 2–3 days | New endpoints above |
| Expiry countdown component (React) | 0.5 day | None |
| QR code display + copy URL component | 0.5 day | None |
| Header nav link to `/account` | 0.25 day | Existing `Header.tsx` |
| "Add another device" entry on `/login` page | 0.5 day | Existing `login/page.tsx` |
| Token regeneration button + confirmation | 0.5 day | `/auth/invite` endpoint |
| Last-credential revoke guard | 0.25 day | Credential list component |
| **Total estimate** | **~7–8 days** | |

No new npm packages are required for QR display (the server generates the PNG; the UI renders an `<img>` tag). No new Go packages are required (`go-webauthn/webauthn` already handles WebAuthn; `GenerateQRPNG` already exists).

---

## Operational Concerns

**Token file persistence**: The existing `SetupManager` writes the token to `setup-token.json` on disk. If the server restarts while a token is active, the token survives. This is the correct behavior for the web invite flow — a restart should not silently invalidate a token a user just generated. Confirm the existing file-watch restart behavior handles this correctly.

**Concurrent invite generation**: The system is single-user, so concurrent invite generation is unlikely in practice. However, if the user opens two browser tabs and generates an invite in both, the second generation should invalidate the first (last-write-wins). The current `SetupManager` single-token model already enforces this.

**HTTPS requirement for WebAuthn**: WebAuthn requires HTTPS (or `localhost`). Stapler Squad always uses TLS with a self-signed CA. The CA cert QR must be scanned and installed on the new device before the registration URL QR can be used — this ordering must be clear in the UI.

**Token length and entropy**: The existing setup token generation (in `setup.go`) should be reviewed to ensure sufficient entropy (minimum 128 bits / 32 hex characters) for a URL-embedded bearer credential. [See pitfalls research for detailed analysis.]

---

## Prior Art and Lessons Learned

**Tailscale Auth Keys**: Tailscale generates single-use or reusable auth keys for device enrollment. The key is shown once and cannot be retrieved again (stored only as a hash server-side). The web console shows the raw key as text + a copy button. Tailscale's mobile app enrollment uses QR code display in the admin console. Key lesson: **show token once, hash-store server-side**. [TRAINING_ONLY — verify Tailscale key hashing behavior]

**Bitwarden / Vaultwarden new-device trust**: Bitwarden prompts for email verification or a "trusted device" approval from an already-logged-in device when a new device logs in. For self-hosted Vaultwarden, device trust can be disabled; new-device approval is handled via email link. Key lesson: **for self-hosted single-user systems, email-based verification is often disabled** — QR/token is the correct substitute.

**GitHub SSH key enrollment**: GitHub's SSH key page (`/settings/keys`) lists all keys with "Added on" date and a revoke button per key. The `Title` field serves as the display name. There is no expiry; SSH keys are persistent. Key lesson: **the credential list + revoke UX is minimal and works well** — a simple table with columns (name, added date, last used, revoke button) is sufficient; no elaborate UI needed.

**GitLab SSH / Personal Access Tokens**: GitLab's access token management shows token creation date, expiry date, last used, and scopes. Tokens near expiry are highlighted. Key lesson: **expiry date visibility in the credential list reduces surprise lockouts**. For passkeys, "created date" is more relevant than expiry (passkeys do not expire), but "last used" would be valuable if the WebAuthn credential store tracks it.

**FIDO2 Hybrid Transport (WebAuthn Level 3 / caBLE)**: The FIDO2 spec for cross-device authentication uses a QR code displayed on the relying party to establish a Bluetooth proximity channel to the authenticator. The QR encodes a tunnel URL + BLE advertisement data. This is directly analogous to the Stapler Squad flow (QR encodes the registration URL). Key lesson: **QR-encoded URL for device registration is the correct W3C-blessed pattern for WebAuthn cross-device flows**. [TRAINING_ONLY — verify WebAuthn Level 3 hybrid transport spec status and browser support]

**Proxmox VE TOTP setup**: Proxmox's two-factor enrollment page shows QR + manual key side-by-side with clear step labels ("Step 1: Scan QR code or enter key manually", "Step 2: Enter verification code"). Key lesson: **step labels reduce user confusion in multi-step enrollment flows**. The CA cert + registration URL two-QR flow should use explicit step labels.

**Keycloak OTP enrollment**: Keycloak's TOTP setup shows QR + secret string + a link to compatible apps. The "Unable to scan?" accordion expands to show the secret. Key lesson: **progressive disclosure for the manual fallback** — show QR prominently, hide the raw URL behind a "Can't scan?" toggle to reduce visual clutter while keeping the option available.

---

## Open Questions

1. Should the invite page show both CA cert QR and registration QR simultaneously, or as a two-step wizard? The two-QR layout is consistent with the existing CLI but may be confusing. A wizard with step 1 (install cert) and step 2 (register device) might be clearer for first-time users.

2. Should the setup token be stored only as a hash server-side (Tailscale model), or stored in plaintext (current `setup-token.json` model)? The hash model prevents a file-system read from revealing the token, but the plaintext model is simpler and already implemented. For a single-user self-hosted system with filesystem access already implying full control, this is a security posture decision, not a correctness issue.

3. Should the credential list show a "last used" timestamp? The `go-webauthn/webauthn` library tracks the credential's `Authenticator.SignCount` and `LastUpdated` (if implemented). If `LastUpdated` is populated in `store.go`, showing it in the UI would help the user identify stale credentials.

4. Should token expiry be configurable (e.g., 15 min / 1 hour / 8 hours) or fixed at 1 hour? The requirements say "matching or aligning with existing 1-hour CLI default." A fixed 1-hour expiry with a visible countdown is simpler and removes the need for a settings UI.

5. Can the user generate an invite while the previous one is still active? The current single-token model makes regeneration a destructive action. Should there be a UI affordance to see the existing active invite rather than immediately regenerating it?

---

## Recommendation

**Confirm Option C (QR + link together) as the correct approach.** This is validated by:

- Tailscale's mobile enrollment, Proxmox VE TOTP setup, Keycloak OTP enrollment, and FIDO2 hybrid transport all use QR + manual fallback simultaneously
- The existing Stapler Squad CLI already uses this pattern
- Neither QR-only nor link-only satisfies both the nearby-device and remote-device enrollment cases

**Additional confirmed design decisions from prior art:**

1. **One-time token + expiry countdown**: Correct and standard. Use the existing 1-hour default. Display a live countdown that disables the QR and URL (and shows a regenerate prompt) when it expires rather than silently leaving stale content.

2. **Two-QR layout (CA cert + registration)**: Required. Follow the CLI behavior. Use explicit step labels ("Step 1: Install certificate", "Step 2: Register device") rather than showing both QRs with no guidance.

3. **"Can't scan?" toggle for raw URL**: Follow the Keycloak progressive-disclosure pattern. Show QR prominently; reveal the raw URL + copy button behind a toggle. This reduces visual complexity while keeping the link-only path accessible.

4. **Credential list with display name, created date, and per-credential revoke**: Validated by GitHub, GitLab, and Bitwarden patterns. Keep it simple: a table, not a card grid.

5. **Last-credential revoke guard**: Required. GitHub and Bitwarden both enforce "cannot delete account's last credential." Implement this as a disabled revoke button with tooltip explanation.

6. **Token regeneration confirmation**: Warn the user before regenerating that any in-progress enrollment will be cancelled. A confirmation dialog (not just a toast) is warranted because the consequence is non-reversible.

---

## Pending Web Searches

The following queries should be run by the parent agent to verify training-only claims and fill gaps:

1. `tailscale "add device" mobile enrollment QR code admin console 2024 2025` — verify Tailscale mobile enrollment shows QR + URL, and whether the admin console stores keys as hashes
2. `bitwarden vaultwarden "new device" passkey webauthn enrollment flow 2024` — verify Bitwarden/Vaultwarden new-device approval flow and whether QR is used
3. `webauthn "hybrid transport" OR "caBLE" QR code cross-device registration Level 3 browser support 2024 2025` — verify FIDO2 hybrid transport spec status and which browsers implement it
4. `"setup_token" OR "invite link" webauthn passkey enrollment open source UI github 2024` — find open-source WebAuthn UIs that implement invite-link + QR enrollment for design reference
