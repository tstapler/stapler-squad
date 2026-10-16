package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// githubSignaturePrefix is the header-value prefix GitHub (and this repo's generic
// webhook scheme, for consistency) use ahead of the hex-encoded HMAC-SHA256 digest —
// e.g. "sha256=abcdef...".
const githubSignaturePrefix = "sha256="

// VerifyGitHubSignature reports whether sigHeader (the raw X-Hub-Signature-256 header
// value, e.g. "sha256=<hex>") is a valid HMAC-SHA256 signature of body computed with
// secret. Comparison is constant-time via hmac.Equal — never == or bytes.Equal, which
// leak timing information about how many leading bytes matched (webhook-triggers
// Epic 2.1).
func VerifyGitHubSignature(secret string, body []byte, sigHeader string) bool {
	return verifyHMACSHA256Signature(secret, body, sigHeader, githubSignaturePrefix)
}

// VerifyWebhookSecret reports whether sigHeader (e.g. "sha256=<hex>") is a valid
// HMAC-SHA256 signature of body computed with secret. Same scheme as
// VerifyGitHubSignature, exposed under its own name for the generic `webhook` trigger
// type's configurable signature header (X-Webhook-Signature) — kept as a distinct
// function (not an alias) so the two trigger types' verification call sites can diverge
// independently if a future provider needs a different scheme.
func VerifyWebhookSecret(secret string, body []byte, sigHeader string) bool {
	return verifyHMACSHA256Signature(secret, body, sigHeader, githubSignaturePrefix)
}

// verifyHMACSHA256Signature strips prefix from sigHeader, hex-decodes the remainder,
// and compares it against HMAC-SHA256(secret, body) using hmac.Equal. Returns false
// (never panics) for any malformed input: empty secret/header, missing prefix, invalid
// hex, or a decoded digest of the wrong length.
func verifyHMACSHA256Signature(secret string, body []byte, sigHeader, prefix string) bool {
	if secret == "" || sigHeader == "" {
		return false
	}
	if !strings.HasPrefix(sigHeader, prefix) {
		return false
	}

	digest, err := hex.DecodeString(strings.TrimPrefix(sigHeader, prefix))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body) // hash.Hash.Write never returns an error
	expected := mac.Sum(nil)

	return hmac.Equal(digest, expected)
}
