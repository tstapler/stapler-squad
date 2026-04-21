# ADR-002: Inline QR PNG as Base64 Data URI in Generate Response

**Status**: Accepted
**Date**: 2026-04-21
**Feature**: Web Device Registration

---

## Context

The `POST /auth/invite/generate` endpoint must deliver a QR code image to the browser so the user can scan it with a new device. Three delivery mechanisms were considered:

1. **Inline data URI**: Base64-encode the PNG bytes and embed them directly in the JSON response as `"qr_png_data_url": "data:image/png;base64,..."`.
2. **Separate PNG endpoint**: Return a `"qr_url"` field pointing to a server endpoint (e.g., `GET /auth/invite/{id}/qr.png`) that serves the image. The browser makes a second request.
3. **SVG string**: Return an SVG string encoding of the QR code instead of a PNG.

The existing QR generation primitive is `GenerateQRPNG(url string) ([]byte, error)` in `server/auth/qrcode.go`. It returns raw PNG bytes using `github.com/skip2/go-qrcode` at 256×256 pixels.

---

## Decision

Return the QR PNG as a base64 data URI embedded directly in the `POST /auth/invite/generate` JSON response. No separate QR image endpoint is introduced.

The response field is `"qr_png_data_url"`. Its value is `"data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)`.

The frontend assigns this string directly to an `<img src={data.qr_png_data_url} ...>` element. No fetch, no `<Image>` component, no blob URL.

---

## Rationale

**Single round-trip.** The invite generation already does the expensive work: generating the token, computing the URLs, and calling `GenerateQRPNG`. Bundling the PNG into the same response saves a second HTTP round-trip with no additional server-side cost. The user sees the QR the instant the generate button responds.

**Stateless delivery.** A separate `/auth/invite/{id}/qr.png` endpoint would need to either re-generate the PNG on each request (wasted compute) or cache it server-side (state to manage, potential stale-data bugs). Inline delivery is inherently stateless.

**PNG size is negligible.** A 256×256 QR PNG for a ~100-character URL compresses to approximately 1–3 KB. Base64-encoding adds ~33%, yielding ~1.3–4 KB. In the context of a JSON response that also includes a registration URL and expiry metadata, this is not a meaningful payload increase.

**No separate auth check required.** A separate `GET /auth/invite/{id}/qr.png` endpoint would require its own authentication gate and invite-ID lookup. Omitting it reduces the attack surface.

**Reuses `GenerateQRPNG` unchanged.** The existing function signature `GenerateQRPNG(url string) ([]byte, error)` is a perfect fit. No interface changes are needed.

---

## Consequences

**Accepted costs:**
- The JSON response for `POST /auth/invite/generate` is larger than a URL-only response by ~1.3–4 KB. Acceptable.
- The data URI cannot be cached by the browser's image cache. Acceptable because each invite is unique and short-lived.

**Not accepted:**
- A separate QR PNG endpoint. It adds complexity (extra route, extra auth check, cache vs. re-generate decision) with no benefit in this single-page modal context.

---

## Alternatives Rejected

**Separate `GET /auth/invite/{id}/qr.png` endpoint**: Rejected. Extra round-trip, extra route to secure, extra state management, no UX or security benefit.

**SVG string**: Rejected. `github.com/skip2/go-qrcode` produces PNG natively. Generating SVG would require either a new dependency or a significant re-implementation of the QR generation logic. PNG at 256×256 is scannable by all mobile camera apps.
