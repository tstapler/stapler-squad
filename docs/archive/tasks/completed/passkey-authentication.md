# Feature Plan: Passkey Authentication and Remote Access

**Epic ID**: EPIC-PASSKEY-001
**Priority**: P1 - Enables core remote access use case
**Status**: Planning Complete, Ready for Implementation
**Target Branch**: `stapler-squad-passkey`

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [Requirements](#requirements)
3. [Architecture Decision Records](#architecture-decision-records)
4. [Epic-Level Analysis](#epic-level-analysis)
5. [Story Breakdown](#story-breakdown)
6. [Atomic Task Decomposition](#atomic-task-decomposition)
7. [Known Issues and Proactive Bug Identification](#known-issues-and-proactive-bug-identification)
8. [Dependency Visualization](#dependency-visualization)
9. [Context Preparation Guides](#context-preparation-guides)

---

## Problem Statement

Stapler Squad currently binds exclusively to `localhost:8543`, making it inaccessible from any machine other than the host. Users who run stapler-squad on a development server, homelab, or headless machine cannot access the web UI from their phone, tablet, or laptop. There is also no authentication layer, because localhost-only access was the implicit security boundary.

**Goal**: Allow users to securely access the stapler-squad web UI from non-local machines using passkey (WebAuthn/FIDO2) authentication, with a QR-code-based enrollment flow for initial device registration.

---

## Requirements

### Functional Requirements (User Stories)

#### US-001: Remote Network Access
**As a** developer running stapler-squad on a remote machine,
**I want to** access the web UI from my phone/laptop over the network,
**So that** I can manage AI agent sessions without being physically at the host.

**Acceptance Criteria** (Given-When-Then):
- GIVEN stapler-squad is started with `--remote-access` or `listen_address` configured,
  WHEN a user navigates to the server's IP/hostname from another device,
  THEN the web UI loads (behind an auth gate).
- GIVEN remote access is enabled,
  WHEN the server starts,
  THEN it logs a warning that the server is listening on a non-localhost address.
- GIVEN remote access is NOT enabled (default),
  WHEN the server starts,
  THEN it binds to `localhost:8543` exactly as today (zero behavior change).

#### US-002: Passkey Registration via QR Code
**As a** user setting up stapler-squad for the first time with remote access,
**I want to** scan a QR code on my phone to register a passkey,
**So that** I can authenticate from that device on future visits.

**Acceptance Criteria**:
- GIVEN no passkeys are registered and the user visits the UI,
  WHEN the setup page loads,
  THEN a QR code is displayed containing the registration URL.
- GIVEN the user scans the QR code on their phone,
  WHEN the phone opens the registration URL,
  THEN the WebAuthn registration ceremony begins on the phone's browser.
- GIVEN the user completes biometric/PIN verification on the phone,
  WHEN the credential is created,
  THEN the server stores the credential and the user is authenticated.

#### US-003: Passkey Authentication on Return Visits
**As a** user who has already registered a passkey,
**I want to** authenticate using my passkey when I visit the UI,
**So that** only authorized users can access my sessions.

**Acceptance Criteria**:
- GIVEN a user navigates to the UI from a registered device,
  WHEN the login page loads,
  THEN the WebAuthn authentication ceremony starts automatically.
- GIVEN the user completes biometric/PIN verification,
  WHEN the assertion is verified,
  THEN a session cookie is set and the user is redirected to the dashboard.
- GIVEN the user has an active session cookie,
  WHEN they navigate to any page,
  THEN they bypass the login page.

#### US-004: Localhost Bypass
**As a** developer accessing stapler-squad from localhost,
**I want to** skip authentication entirely,
**So that** the local development experience is unchanged.

**Acceptance Criteria**:
- GIVEN a request originates from 127.0.0.1 or ::1,
  WHEN the auth middleware processes the request,
  THEN the request is allowed without authentication.
- GIVEN `--remote-access` is NOT enabled,
  WHEN the server runs,
  THEN no auth middleware is loaded at all.

#### US-005: Manage Registered Passkeys
**As a** user with registered passkeys,
**I want to** view and revoke registered passkeys from the settings page,
**So that** I can remove lost devices or rotate credentials.

**Acceptance Criteria**:
- GIVEN the user is authenticated,
  WHEN they navigate to settings,
  THEN a list of registered passkeys is displayed with device name and registration date.
- GIVEN the user clicks "Revoke" on a passkey,
  WHEN the confirmation dialog is accepted,
  THEN the passkey is removed from storage.
- GIVEN the user revokes their last passkey,
  WHEN they next visit from a remote device,
  THEN the setup/registration page is shown again.

### Non-Functional Requirements

#### Security (MUST)
- NFR-SEC-001: All remote connections MUST use TLS (self-signed cert auto-generated or user-provided).
- NFR-SEC-002: Session tokens MUST be stored in httpOnly, Secure, SameSite=Strict cookies.
- NFR-SEC-003: WebAuthn challenge values MUST be cryptographically random and single-use.
- NFR-SEC-004: Passkey credential storage MUST use file permissions 0600 (owner-only).
- NFR-SEC-005: Failed authentication attempts MUST be rate-limited (max 10/minute per IP).
- NFR-SEC-006: Localhost requests MUST bypass authentication (backward compatibility).

#### Usability (MUST)
- NFR-UX-001: First-time setup MUST complete in under 2 minutes.
- NFR-UX-002: QR code MUST be scannable from 30cm distance on standard phone cameras.
- NFR-UX-003: Authentication on return visits MUST complete in under 5 seconds.
- NFR-UX-004: Login page MUST be accessible (WCAG 2.1 AA).

#### Performance (SHOULD)
- NFR-PERF-001: Auth middleware overhead SHOULD be < 1ms per request for authenticated sessions.
- NFR-PERF-002: WebAuthn ceremony SHOULD complete in < 2 seconds server-side.

#### Reliability (SHOULD)
- NFR-REL-001: Passkey store SHOULD survive server restarts (file-based persistence).
- NFR-REL-002: Corrupt passkey store SHOULD trigger re-enrollment, not a crash.

### MoSCoW Prioritization

| Requirement | Priority | Rationale |
|---|---|---|
| US-001: Remote Network Access | **Must** | Core enabler; without this, nothing else matters |
| US-002: Passkey Registration (QR) | **Must** | Primary enrollment mechanism |
| US-003: Passkey Authentication | **Must** | Core security gate |
| US-004: Localhost Bypass | **Must** | Backward compatibility, zero-friction local dev |
| US-005: Manage Passkeys | **Should** | Important for security hygiene but not day-1 blocker |
| NFR-SEC-001: TLS | **Must** | WebAuthn requires secure context (HTTPS or localhost) |
| NFR-SEC-006: Localhost bypass | **Must** | Backward compatibility |
| Auto-generated self-signed TLS | **Must** | Users should not need to manage certs for personal use |
| User-provided TLS certs | **Could** | Power users may want their own certs |
| Multiple user support | **Won't** (this release) | Single-user tool, defer multi-user |

---

## Architecture Decision Records

### ADR-001: Authentication Mechanism Selection

**Status**: Proposed

**Context**: Claude-squad needs an authentication mechanism for remote access. Options considered:

| Option | Pros | Cons |
|---|---|---|
| **WebAuthn/FIDO2 Passkeys** | Phishing-resistant, no passwords to leak, modern UX, biometric on phones | Requires HTTPS, complex server implementation, rpID binding complexity |
| **TOTP (Authenticator App)** | Simple to implement, widely understood, works offline | Phishable, requires shared secret storage, manual code entry every time |
| **Device-Bound Tokens** | Simple implementation (random token in config), no crypto library needed | Token can be leaked, no biometric, copy-paste UX |
| **Magic Links (Email)** | Zero setup, familiar pattern | Requires email infrastructure, latency, not offline-capable |
| **mTLS (Client Certificates)** | Very strong security, no session management | Terrible UX for phone enrollment, cert distribution pain |

**Decision**: **WebAuthn/FIDO2 Passkeys** with a device-bound token fallback for environments where WebAuthn is unavailable.

**Rationale**:
1. Passkeys are the direction the industry is moving (Apple, Google, Microsoft all support them natively).
2. The QR code enrollment matches the user's request exactly.
3. Phishing resistance is valuable -- stapler-squad controls real shell sessions.
4. Modern phones (iOS 16+, Android 9+) have built-in passkey support with biometrics.
5. The added implementation complexity is justified by the security properties for a tool that controls terminal sessions.

**Fallback**: A `--auth-token` CLI flag generates a random bearer token printed to the console. This covers headless/scripted access and environments where WebAuthn is not supported.

**Consequences**:
- Requires TLS (WebAuthn mandates secure context).
- Requires the `go-webauthn/webauthn` library.
- rpID must be carefully managed (see Known Issues).

---

### ADR-002: Network Exposure Strategy

**Status**: Proposed

**Context**: Users need to access stapler-squad from non-local machines. Options:

| Option | Pros | Cons |
|---|---|---|
| **Direct bind to 0.0.0.0** | Simple, no external dependencies, full control | Requires firewall config, exposes to entire LAN, needs TLS |
| **Tailscale/WireGuard VPN** | Already encrypted, authenticated, works across NAT | External dependency, not all users have Tailscale, docs-only approach |
| **ngrok-style tunnel** | Works through NAT/firewalls, provides HTTPS | External dependency, latency, third-party trust, cost |
| **Reverse proxy (nginx/caddy)** | Standard approach, full TLS/domain support | Extra infra to manage, over-engineered for personal tool |

**Decision**: **Direct bind with auto-TLS**, with documentation for Tailscale and reverse proxy as optional enhancements.

**Rationale**:
1. Claude-squad is a personal developer tool. Direct bind is the simplest approach that requires zero external dependencies.
2. Auto-generating a self-signed TLS cert makes HTTPS work out of the box.
3. Tailscale is excellent but cannot be a requirement -- documented as recommended enhancement.
4. Users on home networks or VPNs get immediate value with `--listen 0.0.0.0:8543`.

**Implementation**:
- New config field: `listen_address` (default: `localhost:8543`).
- New CLI flag: `--listen <addr>` overrides config.
- New CLI flag: `--remote-access` shorthand for `--listen 0.0.0.0:8543` with auto-TLS.
- When binding to non-localhost, auto-generate self-signed cert if no cert is configured.
- Print clear warning at startup when listening on non-localhost.

**Consequences**:
- Need TLS certificate generation code (Go's `crypto/tls` + `crypto/x509`).
- CORS middleware must handle non-localhost origins.
- WebAuthn rpID must match the hostname/IP used for access.

---

### ADR-003: Credential and Session Storage

**Status**: Proposed

**Context**: Passkey credentials and session tokens need persistent storage. The main branch previously removed SQLite.

| Option | Pros | Cons |
|---|---|---|
| **JSON file** | Simple, matches existing pattern (state.json), no dependencies | No query capability, manual file locking, potential corruption |
| **SQLite** | ACID, query support, battle-tested | Was removed from main branch, adds CGO dependency |
| **BoltDB/bbolt** | No CGO, ACID, embedded | Another dependency, limited query capability |
| **In-memory + JSON backup** | Fast reads, simple serialization | Stale on crash, dual source of truth |

**Decision**: **JSON file** in the config directory, matching the existing `state.json` pattern.

**Rationale**:
1. The credential store is tiny (typically 1-5 passkeys for a personal tool).
2. JSON files match the existing config/state persistence pattern exactly.
3. No new dependencies needed.
4. File locking is already solved in the codebase (`gofrs/flock`).
5. SQLite was deliberately removed -- adding it back for auth would be inconsistent.

**Storage Layout**:
```
~/.stapler-squad/
  workspaces/<hash>/
    auth/
      credentials.json   # WebAuthn credentials (0600 permissions)
      sessions.json      # Active session tokens (0600 permissions)
      tls/
        cert.pem         # Auto-generated TLS certificate
        key.pem          # TLS private key (0600 permissions)
```

**Consequences**:
- Must handle concurrent reads/writes with file locking.
- Must handle corrupt files gracefully (re-enrollment, not crash).
- Session token cleanup (expiry) runs on a timer or at startup.

---

### ADR-004: Go WebAuthn Library Selection

**Status**: Proposed

**Context**: A Go library is needed to implement WebAuthn server-side ceremonies.

| Library | Stars | Maintenance | CGO | Notes |
|---|---|---|---|---|
| **go-webauthn/webauthn** | 1.3k+ | Active, regular releases | No | De facto standard, used by Gitea, Harbor |
| **duo-labs/webauthn** | Archived | Deprecated, merged into go-webauthn | No | Predecessor to go-webauthn |
| **fxamacker/webauthn** | ~200 | Low activity | No | Less adopted |
| **Custom implementation** | N/A | Full control | No | High risk, spec is complex |

**Decision**: **go-webauthn/webauthn** (github.com/go-webauthn/webauthn).

**Rationale**:
1. De facto standard Go WebAuthn library.
2. Active maintenance with regular releases.
3. No CGO dependency -- pure Go.
4. Well-documented with examples for registration and authentication flows.
5. Used by major projects (Gitea, Harbor) validating its production readiness.

**Consequences**:
- Add `github.com/go-webauthn/webauthn` to go.mod.
- Must implement the `webauthn.User` interface for credential storage.
- Must manage WebAuthn session data (challenge storage) between ceremony steps.

---

### ADR-005: QR Code Enrollment Approach

**Status**: Proposed

**Context**: Users need to register a passkey from their phone by scanning a QR code displayed in the web UI or terminal.

| Approach | Pros | Cons |
|---|---|---|
| **QR encodes registration URL** | Simple, works with any QR scanner, phone opens browser | Requires phone to reach server over network |
| **QR encodes challenge directly** | Could work offline | Non-standard, requires custom phone app |
| **WebAuthn cross-device flow (hybrid)** | Native platform support, BLE-based | Complex, requires FIDO2 platform authenticator coordination |

**Decision**: **QR code encodes the registration URL** (e.g., `https://<server-ip>:8543/auth/register`).

**Rationale**:
1. Simplest approach that directly matches the user's request ("scan a QR code on my phone").
2. Works with every phone's built-in QR scanner -- no special app needed.
3. The phone's browser handles the WebAuthn ceremony natively.
4. The URL includes a one-time setup token to prevent unauthorized registration.

**Flow**:
```
1. User visits stapler-squad from local browser (or sees QR in terminal)
2. UI/terminal shows QR code containing: https://<host>:8543/auth/setup?token=<one-time-token>
3. User scans QR code with phone camera
4. Phone browser opens the URL
5. Page triggers navigator.credentials.create() (WebAuthn registration)
6. User confirms with biometric/PIN on phone
7. Credential sent to server, stored, user authenticated
8. Future visits: phone browser triggers navigator.credentials.get()
```

**Consequences**:
- Need a Go QR code library (e.g., `skip2/go-qrcode`).
- The one-time setup token must be cryptographically random and expire after use.
- The phone must be able to reach the server over the network.
- The QR code URL must use the correct hostname/IP that the phone can resolve.

---

## Epic-Level Analysis

### User Value
**Primary**: Securely access stapler-squad from any device (phone, tablet, laptop) when running on a remote/headless machine.

**Secondary**:
- Monitor AI agent sessions from phone while away from desk.
- Approve permission requests from phone notification (future: push notifications).
- Share stapler-squad access with a trusted colleague (register their device).

### Success Metrics
1. User can authenticate from a non-local IP address within 30 seconds of scanning QR code.
2. Passkey works on second and subsequent visits without re-enrollment.
3. Localhost access continues to work with zero configuration changes (backward compat).
4. Server starts with `--remote-access` flag in under 3 seconds (including TLS cert generation).

### Scope Boundary
**In Scope**: Authentication, authorization, TLS, remote network binding, passkey management.
**Out of Scope**: Multi-user/RBAC, push notifications, OAuth/SSO integration, tunnel service.

---

## Story Breakdown

### Story 1: Network Exposure and TLS Foundation
**Goal**: Allow stapler-squad to listen on non-localhost addresses with auto-TLS.
**Value**: Enables remote access -- the prerequisite for everything else.
**Dependencies**: None.
**INVEST**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

### Story 2: WebAuthn Backend (Registration and Authentication)
**Goal**: Implement passkey registration and authentication server-side.
**Value**: Core security mechanism for remote access.
**Dependencies**: Story 1 (needs HTTPS for WebAuthn).
**INVEST**: Independent (from frontend), Negotiable, Valuable, Estimable, Small, Testable.

### Story 3: QR Code Enrollment Flow
**Goal**: Generate and display QR codes for device enrollment.
**Value**: The specific UX the user requested.
**Dependencies**: Story 2 (needs registration endpoint).
**INVEST**: Independent (UI concern), Negotiable, Valuable, Estimable, Small, Testable.

### Story 4: Frontend Auth UI
**Goal**: Login page, registration page, auth state management in React.
**Value**: User-facing authentication experience.
**Dependencies**: Story 2 (backend endpoints), Story 3 (QR display).
**INVEST**: Independent (frontend-only), Negotiable, Valuable, Estimable, Small, Testable.

### Story 5: Auth Middleware and Session Management
**Goal**: Protect existing routes, manage session cookies, localhost bypass.
**Value**: Actually enforces authentication on all endpoints.
**Dependencies**: Story 2 (auth verification), Story 4 (frontend redirects).
**INVEST**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## Atomic Task Decomposition

### Story 1: Network Exposure and TLS Foundation

#### Task 1.1: Add listen address configuration
**Duration**: 2 hours
**Files**: `config/config.go`, `main.go`

**Implementation**:
- Add `ListenAddress string` to `Config` struct (default: `localhost:8543`).
- Add `--listen <addr>` flag to rootCmd.
- Add `--remote-access` flag as shorthand for `--listen 0.0.0.0:8543`.
- Wire flag value into `server.NewServer(address)` call.
- Log warning when binding to non-localhost.

**Acceptance Criteria**:
- `./stapler-squad` binds to localhost:8543 (unchanged).
- `./stapler-squad --listen 0.0.0.0:8543` binds to all interfaces.
- `./stapler-squad --remote-access` binds to 0.0.0.0:8543.
- Warning logged when non-localhost binding.

---

#### Task 1.2: Auto-generate self-signed TLS certificate
**Duration**: 3 hours
**Files**: `server/tls.go` (new), `server/tls_test.go` (new)

**Implementation**:
```go
// server/tls.go
package server

// GenerateOrLoadTLSCert checks for existing cert/key at the given path.
// If not found, generates a self-signed certificate valid for the given
// hostnames/IPs and writes cert.pem + key.pem.
// Returns tls.Certificate suitable for use with http.Server.
func GenerateOrLoadTLSCert(certDir string, hosts []string) (tls.Certificate, error)
```
- Use `crypto/x509`, `crypto/ecdsa`, `crypto/elliptic` (P-256).
- Generate cert valid for: localhost, 127.0.0.1, ::1, plus any IPs/hostnames from listen address.
- Cert validity: 1 year, auto-regenerate if expired.
- Store in `<config-dir>/auth/tls/cert.pem` and `key.pem` with 0600 permissions.

**Acceptance Criteria**:
- First start with `--remote-access` generates cert.pem and key.pem.
- Second start reuses existing cert.
- Expired cert triggers regeneration.
- key.pem has 0600 permissions.

---

#### Task 1.3: Enable HTTPS when remote access is active
**Duration**: 2 hours
**Files**: `server/server.go`, `main.go`

**Implementation**:
- Modify `Server.Start()` to use `ListenAndServeTLS()` when TLS cert is available.
- Add `tlsCert *tls.Certificate` field to `Server` struct.
- Add `NewServerWithTLS(addr string, cert tls.Certificate) *Server` constructor.
- In `main.go`, when `--remote-access` or non-localhost listen address detected, generate/load TLS cert and use TLS server.
- Print both `http://` and `https://` URLs at startup as appropriate.

**Acceptance Criteria**:
- `--remote-access` starts HTTPS server.
- Default localhost mode remains HTTP (no TLS overhead for local dev).
- Browser shows self-signed cert warning (expected).

---

#### Task 1.4: Update CORS middleware for remote origins
**Duration**: 2 hours
**Files**: `server/middleware/cors.go`

**Implementation**:
- When remote access is enabled, CORS must allow the server's actual origin (not just localhost:3000).
- Add a `CORSWithDynamicOrigin` middleware that reflects the request's Origin header if it matches the server's configured listen address.
- Alternatively, when remote access is active, allow any origin but validate via auth token/cookie (defense in depth).

**Acceptance Criteria**:
- CORS allows requests from `https://<server-ip>:8543`.
- Preflight OPTIONS requests succeed from remote origins.
- Existing localhost CORS behavior unchanged.

---

### Story 2: WebAuthn Backend (Registration and Authentication)

#### Task 2.1: Implement credential store
**Duration**: 3 hours
**Files**: `server/auth/store.go` (new), `server/auth/store_test.go` (new)

**Implementation**:
```go
// server/auth/store.go
package auth

// CredentialStore manages WebAuthn credentials on disk.
type CredentialStore struct {
    path string
    mu   sync.RWMutex
}

type StoredUser struct {
    ID          []byte                        `json:"id"`
    Name        string                        `json:"name"`
    DisplayName string                        `json:"display_name"`
    Credentials []webauthn.Credential         `json:"credentials"`
    CreatedAt   time.Time                     `json:"created_at"`
}

func NewCredentialStore(configDir string) (*CredentialStore, error)
func (s *CredentialStore) GetUser() (*StoredUser, error)
func (s *CredentialStore) SaveUser(user *StoredUser) error
func (s *CredentialStore) AddCredential(cred webauthn.Credential) error
func (s *CredentialStore) RemoveCredential(credID []byte) error
func (s *CredentialStore) HasCredentials() bool
```
- File stored at `<config-dir>/auth/credentials.json` with 0600 permissions.
- Uses `gofrs/flock` for file locking (existing dependency).
- Implements `webauthn.User` interface on `StoredUser`.
- Handles corrupt file gracefully (log error, treat as empty).

**Acceptance Criteria**:
- Credentials persist across server restarts.
- File has 0600 permissions.
- Corrupt JSON file results in empty store (not crash).
- Thread-safe concurrent access.

---

#### Task 2.2: Implement session token manager
**Duration**: 2 hours
**Files**: `server/auth/session.go` (new), `server/auth/session_test.go` (new)

**Implementation**:
```go
// server/auth/session.go
package auth

// SessionManager handles auth session tokens (cookies).
type SessionManager struct {
    store    map[string]*Session  // token -> session
    mu       sync.RWMutex
    filePath string
}

type Session struct {
    Token     string    `json:"token"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
    UserAgent string    `json:"user_agent"`
    RemoteIP  string    `json:"remote_ip"`
}

func NewSessionManager(configDir string) *SessionManager
func (m *SessionManager) Create(r *http.Request) (*Session, error)
func (m *SessionManager) Validate(token string) (*Session, bool)
func (m *SessionManager) Revoke(token string)
func (m *SessionManager) CleanExpired()
```
- Token: 32 bytes crypto/rand, base64url encoded.
- Session lifetime: 30 days (configurable).
- Persisted to `<config-dir>/auth/sessions.json` on change.
- Cleanup runs on startup and every hour.

**Acceptance Criteria**:
- Session tokens are cryptographically random.
- Expired sessions are cleaned up.
- Sessions survive server restart.
- Tokens are 256-bit entropy minimum.

---

#### Task 2.3: Implement WebAuthn registration endpoint
**Duration**: 3 hours
**Files**: `server/auth/webauthn.go` (new), `server/auth/handlers.go` (new)

**Implementation**:
```go
// server/auth/webauthn.go
package auth

// WebAuthnService wraps go-webauthn/webauthn for registration and authentication.
type WebAuthnService struct {
    webAuthn *webauthn.WebAuthn
    store    *CredentialStore
    sessions *SessionManager
    // challengeStore holds in-progress ceremony data (in-memory, short-lived)
    challengeStore map[string]*webauthn.SessionData
    challengeMu    sync.Mutex
}

func NewWebAuthnService(rpID, rpOrigin string, store *CredentialStore, sessions *SessionManager) (*WebAuthnService, error)
```

**Endpoints** (registered as plain HTTP handlers):
- `POST /auth/register/begin` - Returns WebAuthn creation options (challenge).
- `POST /auth/register/finish` - Validates credential and stores it.

**Flow**:
1. Client calls `/auth/register/begin`.
2. Server creates challenge via `webAuthn.BeginRegistration()`.
3. Server stores session data in `challengeStore` (keyed by random ID, expires in 5 min).
4. Client performs `navigator.credentials.create()`.
5. Client sends attestation to `/auth/register/finish`.
6. Server calls `webAuthn.FinishRegistration()`, stores credential.

**Acceptance Criteria**:
- Registration ceremony completes successfully with platform authenticator.
- Credential stored in credentials.json after registration.
- Challenge is single-use (cannot replay).
- Challenge expires after 5 minutes.

---

#### Task 2.4: Implement WebAuthn authentication endpoint
**Duration**: 2 hours
**Files**: `server/auth/handlers.go` (modify)

**Endpoints**:
- `POST /auth/login/begin` - Returns WebAuthn assertion options.
- `POST /auth/login/finish` - Validates assertion, creates session.

**Flow**:
1. Client calls `/auth/login/begin`.
2. Server creates challenge via `webAuthn.BeginLogin()`.
3. Client performs `navigator.credentials.get()`.
4. Client sends assertion to `/auth/login/finish`.
5. Server calls `webAuthn.FinishLogin()`.
6. Server creates session token, sets httpOnly cookie.

**Acceptance Criteria**:
- Authentication ceremony completes with registered passkey.
- Session cookie set with httpOnly, Secure, SameSite=Strict.
- Sign count validated (clone detection).

---

#### Task 2.5: Implement setup token for first-time registration
**Duration**: 2 hours
**Files**: `server/auth/setup.go` (new)

**Implementation**:
```go
// server/auth/setup.go
package auth

// SetupTokenManager generates and validates one-time setup tokens
// for bootstrapping the first passkey registration.
type SetupTokenManager struct {
    token     string
    createdAt time.Time
    used      bool
    mu        sync.Mutex
}

func NewSetupTokenManager() *SetupTokenManager
func (m *SetupTokenManager) Generate() string  // Returns token, also prints to console
func (m *SetupTokenManager) Validate(token string) bool  // Single-use
func (m *SetupTokenManager) IsExpired() bool  // 15 minute expiry
```

**Bootstrap Flow**:
- When server starts with `--remote-access` and no credentials exist:
  1. Generate one-time setup token.
  2. Print to console: "Setup URL: https://<host>:8543/auth/setup?token=<token>"
  3. Also display as QR code in terminal (using go-qrcode with terminal rendering).
- The `/auth/setup` page validates the token before allowing registration.
- After first credential is registered, setup tokens are no longer generated.

**Acceptance Criteria**:
- Setup token printed to console on first start.
- Token is single-use.
- Token expires after 15 minutes.
- Cannot register without valid setup token when no credentials exist.
- After first passkey registered, setup endpoint returns 403.

---

### Story 3: QR Code Enrollment Flow

#### Task 3.1: Add QR code generation
**Duration**: 2 hours
**Files**: `server/auth/qrcode.go` (new)

**Implementation**:
```go
// server/auth/qrcode.go
package auth

// GenerateQRCode creates a QR code PNG for the given URL.
func GenerateQRCode(url string, size int) ([]byte, error)

// GenerateTerminalQR creates a QR code rendered in Unicode block characters
// for display in the terminal at startup.
func GenerateTerminalQR(url string) string
```
- Use `github.com/skip2/go-qrcode` for PNG generation.
- Use Unicode block characters (upper/lower half blocks) for terminal QR rendering.
- PNG served at `/auth/setup/qr` endpoint for web UI display.

**Acceptance Criteria**:
- QR code renders in terminal at startup.
- QR code scannable by phone camera from terminal.
- PNG endpoint returns valid QR code image.

---

#### Task 3.2: Setup page with QR code display
**Duration**: 2 hours
**Files**: `server/auth/handlers.go` (modify)

**Implementation**:
- `GET /auth/setup` - Serves a minimal HTML page (not React, to avoid auth chicken-and-egg) with:
  - QR code image from `/auth/setup/qr`.
  - Manual URL display for copy-paste.
  - Instructions text.
  - Auto-redirect after successful registration.
- `GET /auth/setup/qr` - Returns QR code PNG.
- `GET /auth/status` - Returns JSON `{ "hasCredentials": bool, "authenticated": bool }` for frontend state.

**Acceptance Criteria**:
- Setup page displays QR code and instructions.
- Page accessible without authentication (by design).
- Page returns 403 if credentials already registered (unless setup token valid for adding more).
- Scanning QR code on phone opens registration page in phone browser.

---

### Story 4: Frontend Auth UI

#### Task 4.1: Create login page component
**Duration**: 3 hours
**Files**: `web-app/src/app/auth/login/page.tsx` (new), `web-app/src/app/auth/login/login.module.css` (new)

**Implementation**:
- React page at `/auth/login` route.
- On mount, check `/auth/status` for auth state.
- If no credentials registered, redirect to `/auth/setup`.
- If credentials exist, auto-trigger `navigator.credentials.get()`.
- Show "Authenticating..." spinner during ceremony.
- On success, redirect to `/` (dashboard).
- On failure, show error with retry button.
- Accessible: ARIA labels, keyboard navigation, focus management.

**Acceptance Criteria**:
- Login page triggers WebAuthn automatically.
- Error state shows retry button.
- Redirect to dashboard on success.
- Redirect to setup if no credentials exist.

---

#### Task 4.2: Create auth context and route guard
**Duration**: 3 hours
**Files**: `web-app/src/lib/contexts/AuthContext.tsx` (new), `web-app/src/app/layout.tsx` (modify)

**Implementation**:
```typescript
// web-app/src/lib/contexts/AuthContext.tsx
interface AuthState {
  authenticated: boolean;
  loading: boolean;
  hasCredentials: boolean;
  checkAuth: () => Promise<void>;
}

export function AuthProvider({ children }: { children: React.ReactNode })
export function useAuth(): AuthState
export function RequireAuth({ children }: { children: React.ReactNode })
```
- `AuthProvider` wraps the app, checks `/auth/status` on mount.
- `RequireAuth` component redirects to `/auth/login` if not authenticated.
- Wrap main content in `RequireAuth` in `layout.tsx`.
- Localhost detection: if `window.location.hostname` is `localhost` or `127.0.0.1`, skip auth check.

**Acceptance Criteria**:
- Unauthenticated users redirected to login page.
- Authenticated users see dashboard normally.
- Localhost users bypass auth entirely.
- Auth state persists across page navigation (cookie-based).

---

#### Task 4.3: Add passkey management to settings
**Duration**: 2 hours
**Files**: `web-app/src/app/settings/page.tsx` (new or modify), `web-app/src/components/auth/PasskeyList.tsx` (new)

**Implementation**:
- List registered passkeys with: device name (from authenticator), registration date.
- "Register New Device" button triggers registration ceremony.
- "Revoke" button on each passkey with confirmation dialog.
- Calls new endpoints: `GET /auth/credentials`, `DELETE /auth/credentials/<id>`.

**Acceptance Criteria**:
- Passkeys listed with metadata.
- Can register additional passkeys.
- Can revoke existing passkeys.
- Cannot revoke last passkey without warning.

---

### Story 5: Auth Middleware and Session Management

#### Task 5.1: Implement auth middleware
**Duration**: 3 hours
**Files**: `server/middleware/auth.go` (new), `server/middleware/auth_test.go` (new)

**Implementation**:
```go
// server/middleware/auth.go
package middleware

// Auth creates authentication middleware that:
// 1. Allows localhost requests without auth.
// 2. Allows requests with valid session cookie.
// 3. Allows requests to /auth/* endpoints (login, register, setup).
// 4. Allows requests to /health endpoint.
// 5. Redirects all other requests to /auth/login.
func Auth(sessionManager *auth.SessionManager) func(http.Handler) http.Handler
```

**Public paths** (no auth required):
- `/auth/*` - All auth endpoints
- `/health` - Health check
- Requests from 127.0.0.1 or ::1

**Implementation Details**:
- Read session cookie named `cs_session` (cs = stapler-squad).
- Validate token against SessionManager.
- Set `X-Auth-User` header for downstream handlers.
- Return 401 for API requests, 302 redirect for page requests.

**Acceptance Criteria**:
- Localhost requests always pass.
- Auth endpoints always accessible.
- API requests return 401 when unauthenticated.
- Page requests redirect to /auth/login when unauthenticated.
- Valid session cookie allows request through.

---

#### Task 5.2: Wire auth middleware into server startup
**Duration**: 2 hours
**Files**: `server/server.go`, `main.go`

**Implementation**:
- Only enable auth middleware when `--remote-access` or non-localhost listen address.
- Middleware order: otelhttp -> logging -> auth -> CORS -> handler.
- Initialize `CredentialStore`, `SessionManager`, `WebAuthnService` in server setup.
- Register auth HTTP handlers (`/auth/*`).
- Determine rpID and rpOrigin from listen address.

**CRITICAL**: rpID configuration (see Known Issues below).

**Acceptance Criteria**:
- Auth middleware active when remote access enabled.
- Auth middleware absent when localhost-only (default).
- Auth handlers registered at /auth/* paths.
- rpID correctly derived from server configuration.

---

#### Task 5.3: Add bearer token fallback for API access
**Duration**: 2 hours
**Files**: `server/middleware/auth.go` (modify), `config/config.go` (modify)

**Implementation**:
- `--auth-token <token>` flag generates and prints a bearer token.
- Alternatively, auto-generate and print if `--remote-access` is used.
- Auth middleware checks `Authorization: Bearer <token>` header as alternative to cookie.
- Token stored in `<config-dir>/auth/api-token.json` for persistence.
- Useful for: curl access, API scripting, environments without WebAuthn.

**Acceptance Criteria**:
- Bearer token accepted by auth middleware.
- Token printed to console at startup.
- Token persists across restarts.
- Token can be regenerated with `--regenerate-token` flag.

---

## Known Issues and Proactive Bug Identification

### BUG-PASSKEY-001: WebAuthn rpID and Origin Binding [SEVERITY: Critical]

**Description**: WebAuthn requires that the Relying Party ID (rpID) matches the domain the user accesses the site from. For a self-hosted tool accessed via IP address, this creates a fundamental challenge:
- rpID for `https://192.168.1.100:8543` would be `192.168.1.100`.
- rpID for `https://myserver.local:8543` would be `myserver.local`.
- If the user accesses via different hostnames/IPs, the passkey will not work.
- WebAuthn spec says rpID must be a valid domain (not IP) for some authenticators.

**Mitigation**:
- Auto-detect the hostname/IP from the listen address and first request.
- Store rpID in config so it remains stable.
- Print clear warning if rpID changes between restarts.
- Document that users should access via a consistent hostname.
- Consider using `rpID: "localhost"` with a FIDO2-compliant setup for IP-based access (some authenticators support this).

**Files Likely Affected**: `server/auth/webauthn.go`, `server/server.go`, `config/config.go`

**Prevention Strategy**:
- Add `--rp-id <hostname>` flag for explicit override.
- Add `auth.rp_id` config field.
- Default to first non-loopback hostname detected.
- Log rpID at startup for debugging.
- Add integration test that verifies rpID consistency.

---

### BUG-PASSKEY-002: Self-Signed TLS Certificate Trust [SEVERITY: High]

**Description**: Browsers show security warnings for self-signed certificates. On phones, this is especially painful:
- Safari on iOS may refuse to proceed past the warning for WebAuthn.
- Chrome on Android shows a red warning page.
- The user must manually trust the certificate before passkey registration works.

**Mitigation**:
- Print clear instructions at startup: "You will need to trust the self-signed certificate on your phone."
- Provide a `/auth/cert` endpoint that serves the CA certificate for easy import.
- Document how to import the CA cert on iOS and Android.
- Consider generating a CA cert + server cert pair so the user imports the CA once.
- Long-term: support Let's Encrypt via ACME for users with public DNS.

**Files Likely Affected**: `server/tls.go`, docs

**Prevention Strategy**:
- Generate a mini-CA at first startup, then sign server certs with it.
- Serve CA cert at `/auth/ca.pem` for easy download.
- Include mDNS hostname in cert SANs for `.local` domain access.

---

### BUG-PASSKEY-003: Bootstrap Race Condition [SEVERITY: High]

**Description**: When no credentials are registered, the setup page is accessible without auth. Multiple simultaneous requests could create a race condition where two devices register concurrently.

**Mitigation**:
- Use a mutex around the "first registration" flow.
- Setup token is single-use: once consumed, no new registrations without auth.
- After first credential registered, all subsequent registrations require an existing authenticated session.
- Log all registration attempts with IP and timestamp.

**Files Likely Affected**: `server/auth/setup.go`, `server/auth/handlers.go`

**Prevention Strategy**:
- Atomic compare-and-swap on setup token consumption.
- Integration test with concurrent registration attempts.

---

### BUG-PASSKEY-004: CORS with Credentials for WebAuthn [SEVERITY: High]

**Description**: WebAuthn API calls require `credentials: 'include'` in fetch requests for cookies. CORS must allow this with specific origin (not wildcard). The current CORS middleware uses a permissive origin policy that may conflict.

**Mitigation**:
- When remote access is enabled, switch to `CORSWithOrigins` using the server's actual origin.
- Ensure `Access-Control-Allow-Credentials: true` is set.
- Ensure `Access-Control-Allow-Origin` is the specific origin (not `*`).
- Test with actual cross-origin requests from phone.

**Files Likely Affected**: `server/middleware/cors.go`, `server/server.go`

**Prevention Strategy**:
- Unit test CORS headers for WebAuthn endpoints.
- Integration test from actual different origin.

---

### BUG-PASSKEY-005: Session Cookie Security [SEVERITY: Medium]

**Description**: Session cookies must be configured correctly for security. Misconfiguration could lead to session fixation, CSRF, or cookie theft.

**Mitigation**:
- Cookie attributes: `httpOnly=true, Secure=true, SameSite=Strict, Path=/`.
- Cookie name: `cs_session` (avoid generic names like `session`).
- Regenerate session token after authentication (prevent fixation).
- Set `__Host-` prefix if possible (requires Secure + Path=/ + no Domain).

**Files Likely Affected**: `server/auth/session.go`, `server/auth/handlers.go`

**Prevention Strategy**:
- Unit test cookie attributes.
- Security checklist in code review.

---

### BUG-PASSKEY-006: Passkey Store Corruption Recovery [SEVERITY: Medium]

**Description**: If `credentials.json` becomes corrupted (disk full, power loss during write), users could be locked out of their own system.

**Mitigation**:
- Atomic writes: write to `.tmp` file, then rename (already pattern in codebase).
- Keep one backup: `credentials.json.bak` updated on each successful write.
- On corruption detection, attempt to restore from backup.
- If both corrupt, log error and enter setup mode (allow re-enrollment from localhost).
- Localhost access always bypasses auth, so users can always recover locally.

**Files Likely Affected**: `server/auth/store.go`

**Prevention Strategy**:
- Atomic write with rename.
- Backup file rotation.
- Corruption detection on startup with graceful degradation.

---

### BUG-PASSKEY-007: WebSocket Authentication [SEVERITY: Medium]

**Description**: WebSocket connections (used for terminal streaming) establish via HTTP upgrade. The auth cookie must be validated during the upgrade handshake, not just on the initial page load.

**Mitigation**:
- Auth middleware runs before WebSocket upgrade handler.
- Validate session cookie in the WebSocket handshake handler.
- Close WebSocket if session expires during a long-lived connection.
- Add periodic session validation for long-lived WebSocket connections.

**Files Likely Affected**: `server/services/ws_handler.go`, `server/middleware/auth.go`

**Prevention Strategy**:
- Add auth check to WebSocket handler's `CheckOrigin` function.
- Integration test: connect WebSocket with expired cookie.

---

## Dependency Visualization

```
Story 1: Network Exposure & TLS
 |
 |  Task 1.1: Listen address config ────────────────────────┐
 |  Task 1.2: Auto-generate TLS cert ──────────┐            |
 |  Task 1.3: Enable HTTPS ────────────────────[1.1 + 1.2]  |
 |  Task 1.4: Update CORS ────────────────────────────[1.1]  |
 |                                                            |
 v                                                            |
Story 2: WebAuthn Backend                                     |
 |                                                            |
 |  Task 2.1: Credential store ──────────────────────────────(standalone)
 |  Task 2.2: Session token manager ─────────────────────────(standalone)
 |  Task 2.3: Registration endpoint ─────────[2.1 + 1.3]     |
 |  Task 2.4: Authentication endpoint ───────[2.1 + 2.2]     |
 |  Task 2.5: Setup token manager ───────────(standalone)     |
 |                                                            |
 v                                                            |
Story 3: QR Code Enrollment                                   |
 |                                                            |
 |  Task 3.1: QR code generation ────────────(standalone)     |
 |  Task 3.2: Setup page with QR ───────────[2.3 + 2.5 + 3.1]
 |                                                            |
 v                                                            |
Story 4: Frontend Auth UI                                     |
 |                                                            |
 |  Task 4.1: Login page ───────────────────[2.4]            |
 |  Task 4.2: Auth context & route guard ───[4.1]            |
 |  Task 4.3: Passkey management settings ──[4.2]            |
 |                                                            |
 v                                                            |
Story 5: Auth Middleware                                      |
 |                                                            |
 |  Task 5.1: Auth middleware ──────────────[2.2]             |
 |  Task 5.2: Wire into server ────────────[5.1 + 2.3 + 2.4]|
 |  Task 5.3: Bearer token fallback ───────[5.1]             |


Critical Path: 1.1 → 1.2 → 1.3 → 2.3 → 3.2 → 4.1 → 4.2 → 5.2

Parallelizable:
  - Task 2.1, 2.2, 2.5, 3.1 can all start immediately (no deps)
  - Task 1.1 and 1.2 can run in parallel
  - Task 4.3 and 5.3 can run in parallel after their deps
```

---

## Context Preparation Guides

### Story 1: Network Exposure and TLS Foundation

**Files to Load**:
1. `main.go` - Server address wiring, CLI flags (lines 40-153)
2. `config/config.go` - Config struct, LoadConfig (lines 164-361)
3. `server/server.go` - Server struct, Start(), NewServer() (lines 24-45, 338-376)
4. `server/middleware/cors.go` - Current CORS implementation (all)

**Key Understanding**:
- `address` is hardcoded to `localhost:8543` in main.go:146
- Server uses `ListenAndServe()` (no TLS) in server.go:363
- CORS currently allows any origin in the default middleware

---

### Story 2: WebAuthn Backend

**Files to Load**:
1. `go.mod` - Verify no conflicting crypto dependencies
2. `config/config.go` - GetConfigDir() for credential storage path
3. `server/server.go` - Where to register new HTTP handlers
4. `server/middleware/cors.go` - CORS must support credentials

**External Reference**:
- `go-webauthn/webauthn` documentation: https://github.com/go-webauthn/webauthn
- WebAuthn spec: rpID, origin, attestation types

---

### Story 3: QR Code Enrollment

**Files to Load**:
1. `server/auth/handlers.go` (from Task 2.3) - Registration endpoints to link from QR
2. `server/auth/setup.go` (from Task 2.5) - Setup token validation
3. `main.go` - Console output for terminal QR display

**External Reference**:
- `skip2/go-qrcode` documentation

---

### Story 4: Frontend Auth UI

**Files to Load**:
1. `web-app/src/app/layout.tsx` - Where to add AuthProvider
2. `web-app/src/lib/routes.ts` - Add auth routes
3. `web-app/src/lib/config.ts` - API base URL (must work with HTTPS)
4. `web-app/src/lib/contexts/NotificationContext.tsx` - Pattern for context providers

**Key Understanding**:
- App uses Next.js with App Router
- Existing context providers wrap the app in layout.tsx
- API calls use `window.location.origin + '/api'` (adapts automatically to HTTPS)

---

### Story 5: Auth Middleware

**Files to Load**:
1. `server/server.go` - Middleware chain in Start() method (line 342-347)
2. `server/middleware/logging.go` - Pattern for middleware implementation
3. `server/middleware/cors.go` - Middleware ordering reference
4. `server/services/ws_handler.go` - WebSocket handler that needs auth

**Key Understanding**:
- Middleware chain: `otelhttp -> logging -> CORS -> handler`
- Auth should go between logging and CORS: `otelhttp -> logging -> auth -> CORS -> handler`
- WebSocket handler at `/api/session.v1.SessionService/StreamTerminal` needs auth too

---

## New Dependencies

| Package | Purpose | License |
|---|---|---|
| `github.com/go-webauthn/webauthn` | WebAuthn server implementation | BSD-3 |
| `github.com/skip2/go-qrcode` | QR code generation (PNG + terminal) | MIT |

Both are pure Go (no CGO) and have permissive licenses.

---

## Implementation Order

**Phase 1 (MVP - Remote Access Works)**: Stories 1 + 2 + 5 (Tasks 5.1, 5.2)
- Server listens on 0.0.0.0 with TLS
- WebAuthn registration and login work
- Auth middleware protects routes
- Setup token printed to console for first registration

**Phase 2 (QR Code UX)**: Story 3
- QR code displayed in terminal and web setup page
- Smooth enrollment flow from phone

**Phase 3 (Frontend Polish)**: Story 4
- Login page with auto-trigger
- Auth context and route guard
- Passkey management in settings

**Phase 4 (Hardening)**: Story 5 (Task 5.3) + all known issues
- Bearer token fallback
- Rate limiting
- Certificate management improvements

---

## Testing Strategy

### Unit Tests
- `server/auth/store_test.go` - Credential CRUD, corruption handling, file permissions
- `server/auth/session_test.go` - Token generation, expiry, cleanup
- `server/auth/setup_test.go` - Setup token single-use, expiry
- `server/middleware/auth_test.go` - Localhost bypass, cookie validation, public paths
- `server/tls_test.go` - Cert generation, reload, expiry detection

### Integration Tests
- WebAuthn registration ceremony (mock authenticator)
- WebAuthn authentication ceremony (mock authenticator)
- Full flow: setup token -> register -> logout -> login -> access protected route
- Concurrent registration race condition
- WebSocket auth during upgrade

### Manual Testing
- Scan QR code on iPhone, register passkey with Face ID
- Scan QR code on Android, register passkey with fingerprint
- Access from laptop browser with passkey
- Verify localhost access unchanged (no auth prompt)
