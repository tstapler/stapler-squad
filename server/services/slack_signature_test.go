package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// computeTestSlackSignature independently reimplements Slack's v0 signing
// scheme using crypto/hmac directly — never verifySlackSignature or any
// helper it calls — so a broken implementation of the function under test
// cannot accidentally self-verify against its own output (plan.md Story
// 2.1.1's explicit test-design requirement).
func computeTestSlackSignature(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func slackTestHeaders(timestamp, signature string) http.Header {
	h := http.Header{}
	h.Set("X-Slack-Request-Timestamp", timestamp)
	h.Set("X-Slack-Signature", signature)
	return h
}

// slackSigTestSecret/slackSigTestBody are the fixed secret/body half of the
// test vector. The timestamp is generated fresh per test (relative to
// time.Now()) since verifySlackSignature enforces a real 5-minute replay
// window against wall-clock time — a fully static historical timestamp
// would make the "Accepts" happy-path test fail from the moment it was
// written. The (secret, timestamp, body) tuple is still never fed through
// verifySlackSignature itself to derive the expected signature; only
// computeTestSlackSignature's independent HMAC call is used for that.
const (
	slackSigTestSecret = "test-signing-secret-9f83a1"
	slackSigTestBody   = `payload=%7B%22actions%22%3A%5B%7B%22action_id%22%3A%22approve%22%2C%22value%22%3A%22appr-123%3Aallow%22%7D%5D%7D`
)

func TestVerifySlackSignature_Accepts_When_ValidSignatureAndFreshTimestamp(t *testing.T) {
	t.Parallel()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeTestSlackSignature(slackSigTestSecret, ts, slackSigTestBody)

	err := verifySlackSignature(slackSigTestSecret, slackTestHeaders(ts, sig), []byte(slackSigTestBody))
	require.NoError(t, err)
}

func TestVerifySlackSignature_Rejects_When_BodyTampered(t *testing.T) {
	t.Parallel()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeTestSlackSignature(slackSigTestSecret, ts, slackSigTestBody)

	tampered := slackSigTestBody + "x" // one byte appended after signing
	err := verifySlackSignature(slackSigTestSecret, slackTestHeaders(ts, sig), []byte(tampered))
	require.Error(t, err)
	assert.ErrorIs(t, err, errSlackSignatureMismatch)
}

func TestVerifySlackSignature_Rejects_When_TimestampStale(t *testing.T) {
	t.Parallel()
	staleTS := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	sig := computeTestSlackSignature(slackSigTestSecret, staleTS, slackSigTestBody)

	err := verifySlackSignature(slackSigTestSecret, slackTestHeaders(staleTS, sig), []byte(slackSigTestBody))
	require.Error(t, err)
	// Must be the distinct staleness/replay error, not a signature-mismatch
	// error — the signature itself is correctly computed for the stale
	// timestamp, so a mismatch error here would mean staleness isn't
	// actually being checked.
	assert.ErrorIs(t, err, errSlackSignatureStale)
	assert.NotErrorIs(t, err, errSlackSignatureMismatch)
}

func TestVerifySlackSignature_Rejects_When_WrongSecret(t *testing.T) {
	t.Parallel()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeTestSlackSignature("secret-one", ts, slackSigTestBody)

	err := verifySlackSignature("secret-two", slackTestHeaders(ts, sig), []byte(slackSigTestBody))
	require.Error(t, err)
	assert.ErrorIs(t, err, errSlackSignatureMismatch)
}
