# ADR-004: CA Cert as Mandatory Step 0 in AddDeviceModal

**Status**: Accepted
**Date**: 2026-04-21
**Feature**: Web Device Registration

---

## Context

Stapler Squad generates a self-signed TLS certificate using a private CA. The CA cert is served at `GET /auth/ca.pem`. All HTTPS endpoints (including the registration URL in the invite QR) require this CA to be trusted by the connecting device.

A new device scanning the invite QR code will attempt to navigate to `https://<host>/login?setup_token=<token>`. If the CA cert has not been imported into the device's trust store, the browser will display a hard TLS error and block the page. The user cannot bypass this error by clicking "Accept risk" on all platforms — iOS Safari in particular provides no bypass option for user-untrusted certificates on initial navigation.

The CLI `print-qr-codes` command (in `main.go`) addresses this by printing two QR codes: one for the CA cert download URL and one for the registration URL, along with explicit instructions to install the cert first. The web invite modal must provide at least equivalent guidance.

---

## Decision

The `AddDeviceModal` presents a two-step UI where Step 0 (CA cert installation) is always visible before Step 1 (scanning the invite QR). Both QR codes are shown simultaneously — one for the CA cert URL and one for the registration URL — with numbered labels and platform-specific installation instructions.

The CA cert QR encodes `https://<host>:<port>/auth/ca.pem`. This URL is included in the `POST /auth/invite/generate` response as `"ca_url"` alongside the registration URL and QR PNG.

A second QR PNG for the CA cert URL is also generated server-side using `GenerateQRPNG` and returned as `"ca_qr_png_data_url"` in the generate response.

Platform-specific instructions are rendered in an expandable/tabbed section within the modal:

**iOS (17+)**
1. Tap the CA cert QR code or tap the "Download CA Cert" button — Safari downloads the file.
2. Go to Settings → General → VPN & Device Management.
3. Tap the downloaded profile → Install.
4. Go to Settings → General → About → Certificate Trust Settings.
5. Enable full trust for the Stapler Squad CA certificate.

**Android (7+)**
1. Tap the CA cert QR code or tap the "Download CA Cert" button.
2. Open Settings → Security & privacy → More security settings → Encryption & credentials → Install a certificate → CA Certificate.
3. Select the downloaded `.pem` file.

**macOS / Windows**
1. Download the CA cert using the button or link below.
2. Double-click the `.pem` file and install it into the System keychain (macOS) or Certificate Manager (Windows), trusting it for SSL.

After following Step 0, the user scans the registration QR (Step 1) on the new device.

---

## Rationale

**100% failure rate without this step.** Research (findings-pitfalls.md, iOS CA cert install query) confirmed that Safari on iOS 17 does NOT auto-trigger a profile install dialog when a `.pem` file is downloaded from a URL. The user must manually navigate to Settings → General → VPN & Device Management. Without explicit step-by-step guidance, no non-technical user will complete device registration.

**Mirrors the CLI behavior.** The existing `print-qr-codes` command already generates two QR codes. The web modal should be at least as capable as the CLI for a comparable operator experience.

**The CA cert URL is not a secret.** The CA cert at `/auth/ca.pem` is a public-key material, not a credential. Serving its URL alongside the invite is not a security issue. A non-CA-trusting browser cannot connect to the server anyway.

**Generates a second QR on the server.** `GenerateQRPNG` is already in the auth package. Generating a second PNG (for the CA cert URL) in the `generateInvite` handler adds one additional `GenerateQRPNG` call, producing another ~1–3 KB PNG. This is acceptable given the two-QR pattern is already the intended design.

**Modal layout.** Both QRs are shown side by side with clear labels ("Step 0 — Install CA Cert" and "Step 1 — Register Device"). The instructions expand below each QR. On mobile, the QRs stack vertically.

---

## Consequences

**Accepted costs:**
- `POST /auth/invite/generate` response includes two QR PNG data URIs and two URLs, increasing payload size by ~2–6 KB. Negligible.
- The modal is more complex than a single-QR design. The additional complexity is justified by the CA bootstrapping requirement.
- Platform-specific instructions require maintenance if Apple or Google change their certificate trust UI in future OS versions.

**Not accepted:**
- Omitting the CA cert step on the assumption that the new device already trusts the CA. This assumption fails for any device that has never accessed the server before — which is precisely the use case for the invite flow.
- Using a Let's Encrypt certificate to avoid the CA bootstrapping problem. This requires a publicly resolvable hostname and is out of scope.

---

## Alternatives Rejected

**Single QR (registration only) with a text link for the CA cert**: Rejected. Research confirms that a bare download link is insufficient on iOS — users must follow explicit step-by-step instructions. A QR code for the CA cert also eliminates the need to type the URL on the new device before HTTPS connectivity is established.

**Assume CA is already trusted**: Rejected. The invite flow's entire purpose is to on-board a device that has never connected to this server. Such a device has not installed the CA cert.
