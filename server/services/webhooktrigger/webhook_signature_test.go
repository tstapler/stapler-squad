package webhooktrigger

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// sign computes the "sha256=<hex>" header value GitHub (and this repo's generic
// webhook scheme) would send for secret/body.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return githubSignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyGitHubSignature_should_ReturnTrue_When_SignatureIsValid(t *testing.T) {
	t.Parallel()
	secret := "s3cr3t"
	body := []byte(`{"repository":{"full_name":"tstapler/stapler-squad"}}`)
	header := sign(secret, body)

	assert.True(t, VerifyGitHubSignature(secret, body, header))
}

func TestVerifyGitHubSignature_should_ReturnFalse_When_SecretIsWrong(t *testing.T) {
	t.Parallel()
	body := []byte(`{"repository":{"full_name":"tstapler/stapler-squad"}}`)
	header := sign("s3cr3t", body)

	assert.False(t, VerifyGitHubSignature("wrong-secret", body, header))
}

func TestVerifyGitHubSignature_should_ReturnFalse_When_BodyDoesNotHashToHeader(t *testing.T) {
	t.Parallel()
	secret := "s3cr3t"
	body := []byte(`{"repository":{"full_name":"tstapler/stapler-squad"}}`)

	assert.False(t, VerifyGitHubSignature(secret, body, "sha256=deadbeef"))
}

func TestVerifyGitHubSignature_should_ReturnFalse_When_PrefixIsMissing(t *testing.T) {
	t.Parallel()
	secret := "s3cr3t"
	body := []byte(`{"a":1}`)
	// Compute the correct digest but omit the "sha256=" prefix GitHub always sends.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	headerWithoutPrefix := hex.EncodeToString(mac.Sum(nil))

	assert.False(t, VerifyGitHubSignature(secret, body, headerWithoutPrefix))
}

func TestVerifyGitHubSignature_should_ReturnFalse_When_HeaderIsMalformedHex(t *testing.T) {
	t.Parallel()
	assert.False(t, VerifyGitHubSignature("s3cr3t", []byte("body"), "sha256=not-hex!!"))
}

func TestVerifyGitHubSignature_should_ReturnFalse_When_HeaderIsEmpty(t *testing.T) {
	t.Parallel()
	assert.False(t, VerifyGitHubSignature("s3cr3t", []byte("body"), ""))
}

func TestVerifyGitHubSignature_should_ReturnFalse_When_SecretIsEmpty(t *testing.T) {
	t.Parallel()
	body := []byte("body")
	header := sign("s3cr3t", body)
	assert.False(t, VerifyGitHubSignature("", body, header))
}

// TestVerifyGitHubSignature_should_ReturnTrue_When_BodyIsEmptyAndSignatureGenuinelyMatches
// is the Task 2.1.1c edge case: an empty body is not special-cased to always fail — if
// the secret+empty-body genuinely hashes to the supplied header, verification succeeds
// exactly like any other body.
func TestVerifyGitHubSignature_should_ReturnTrue_When_BodyIsEmptyAndSignatureGenuinelyMatches(t *testing.T) {
	t.Parallel()
	secret := "s3cr3t"
	body := []byte{}
	header := sign(secret, body)

	assert.True(t, VerifyGitHubSignature(secret, body, header))
}

func TestVerifyGitHubSignature_should_ReturnFalse_When_BodyIsEmptyAndHeaderIsWrong(t *testing.T) {
	t.Parallel()
	assert.False(t, VerifyGitHubSignature("s3cr3t", []byte{}, "sha256=deadbeef"))
}

// VerifyWebhookSecret shares its implementation with VerifyGitHubSignature but is
// exercised independently since it's the generic `webhook` trigger type's entry point.
func TestVerifyWebhookSecret_should_ReturnTrue_When_SignatureIsValid(t *testing.T) {
	t.Parallel()
	secret := "generic-secret"
	body := []byte(`{"event":"issue_created"}`)
	header := sign(secret, body)

	assert.True(t, VerifyWebhookSecret(secret, body, header))
}

func TestVerifyWebhookSecret_should_ReturnFalse_When_SignatureIsInvalid(t *testing.T) {
	t.Parallel()
	body := []byte(`{"event":"issue_created"}`)
	assert.False(t, VerifyWebhookSecret("generic-secret", body, "sha256=deadbeef"))
}

func TestDecryptWorkflowSecret_should_RoundTripAndRejectBadInput(t *testing.T) {
	infra := svctest.NewWebhookTestInfra(t)
	wf := svctest.NewGenericWebhookWorkflow(t, infra, "rt", "s3cr3t-plain", "", "", "")

	got, err := decryptWorkflowSecret(infra.Cfg, wf)
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t-plain", got)

	body := []byte(`{"a":1}`)
	assert.True(t, VerifyWebhookSecret(got, body, sign("s3cr3t-plain", body)))
	assert.False(t, VerifyWebhookSecret(got, body, sign("wrong-secret", body)))

	_, err = decryptWorkflowSecret(nil, wf)
	assert.Error(t, err, "nil config")
	wf.WebhookSecretEncrypted = ""
	_, err = decryptWorkflowSecret(infra.Cfg, wf)
	assert.Error(t, err, "no secret configured")
	wf.WebhookSecretEncrypted = "not-ciphertext"
	_, err = decryptWorkflowSecret(infra.Cfg, wf)
	assert.Error(t, err, "undecryptable ciphertext")
}
