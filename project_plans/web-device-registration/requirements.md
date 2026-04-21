# Requirements: Web Device Registration

**Status**: Draft | **Phase**: 1 — Ideation complete  
**Created**: 2026-04-21

## Problem Statement

Registering a new client device to use the Stapler Squad web interface currently requires running a CLI command (`ssq print-qr-codes`) on the server host to generate a setup token and QR code. Users who primarily interact via the web UI (e.g., on mobile or a remote machine) cannot bootstrap a second device without shell access. The goal is to allow an already-authenticated client to generate an invitation for a new device entirely within the web UI.

## Success Criteria

- An authenticated user can generate a new-device invite (QR code + link) from the web UI in under 10 seconds with no CLI access
- The new device can complete passkey registration by scanning the QR or visiting the link — same end result as the existing CLI flow
- Authenticated users can list and revoke registered passkeys from the web UI
- The main application header provides a discoverable route to `/account` for device management

## Scope

### Must Have (MoSCoW)

- **Invite generation**: Authenticated client generates a one-time setup token via the web UI (no CLI required)
- **QR code + link**: Invite displayed as both a scannable QR code (PNG) and a copyable raw URL — shown side by side
- **Expiry countdown**: UI shows remaining validity time; token expires after a fixed window (matching or aligning with existing 1-hour CLI default)
- **Token regeneration**: Button to invalidate current invite and generate a fresh one without CLI
- **Credential management**: `/account` page lists all registered passkeys with display name / creation date and allows revocation of individual credentials
- **Account navigation**: Authenticated users can reach `/account` from a persistent header link in the main app
- **Login-page entry point**: The `/login` page exposes an "Add another device" flow when the user is already authenticated (or when accessed while authenticated)

### Out of Scope

- Multi-user accounts — all passkeys belong to a single owner; no roles or per-user namespacing
- Push notifications on new-device registration
- Audit log of historical registration events
- Remote server installation wizard

## Constraints

- **Tech stack**: Go backend (existing `server/auth/` package), React/Next.js frontend; existing WebAuthn library (`go-webauthn/webauthn`) and `@simplewebauthn/browser`
- **CSS**: All new component styles must follow ADR-009 — use vanilla-extract `.css.ts` files; no new CSS Modules for new components
- **Setup token mechanism**: Backend should build on or align with the existing `SetupManager` / `setup-token.json` pattern in `server/auth/setup.go`
- **QR generation**: Existing `GenerateQRPNG` / `qrcode.go` utility should be reused for the web endpoint
- **Timeline**: Not fixed; iterative delivery acceptable
- **Dependencies**: New packages allowed if necessary, but prefer reusing existing

## Context

### Existing Work

- `server/auth/setup.go` — `SetupManager` generates and validates one-time setup tokens, watches token file via `fsnotify`
- `server/auth/qrcode.go` — `GenerateQRPNG(url)` produces a 256×256 PNG; `PrintQRToTerminal` for CLI
- `server/auth/handlers.go` — `/auth/register/begin`, `/auth/register/finish`, `/auth/login/begin`, `/auth/login/finish`, `/auth/ca.pem`; no existing "generate invite" HTTP endpoint
- `server/auth/store.go` — `credentials.json`; credential CRUD exists but no list/revoke HTTP endpoint exposed
- `web-app/src/app/login/page.tsx` — handles `?setup_token=` param for initial registration; foundation for new-device flow
- `main.go:550-618` — `print-qr-codes` command: generates token, prints two QR codes (CA cert URL + registration URL)
- The CA cert download URL (`/auth/ca.pem`) must still be included alongside the registration QR so new devices can import the self-signed cert

### Stakeholders

- Solo operator / owner of the Stapler Squad instance (single-user system)
- Primary use case: enrolling a phone or secondary laptop without needing SSH access

## Open Questions

- Should the invite page also include the CA cert QR (as the CLI does), or can we assume the new device already has the cert installed?
- Should invites be rate-limited (e.g., one active invite at a time, matching current single-token model)?
- Token expiry: keep the existing 1-hour default or make it configurable?

## Research Dimensions Needed

- [ ] Stack — existing WebAuthn + setup token implementation details; what backend changes are needed
- [ ] Features — how comparable tools (Vaultwarden, Bitwarden, Tailscale) handle multi-device enrollment UX
- [ ] Architecture — new HTTP endpoints needed, credential list/revoke API design, React page structure
- [ ] Pitfalls — CSRF risks on invite generation, token replay, credential orphaning on revoke
