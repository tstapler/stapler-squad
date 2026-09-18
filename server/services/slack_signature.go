package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// slackSignatureReplayWindow is the maximum age of X-Slack-Request-Timestamp
// Slack's own docs recommend tolerating before treating a request as a
// possible replay of a previously-captured valid request+signature pair.
// See research/pitfalls.md §5 item 3.
const slackSignatureReplayWindow = 300 * time.Second

// errSlackSignatureStale is returned when the request timestamp falls
// outside slackSignatureReplayWindow. Kept distinct from
// errSlackSignatureMismatch so callers/logs can tell "expired" (replay
// window) apart from "forged" (bad signature) — plan.md Story 2.1.1 AC3.
var errSlackSignatureStale = errors.New("slack signature verification failed: timestamp outside replay window")

// errSlackSignatureMismatch is returned for any other verification failure:
// missing/malformed headers or a signature that does not match the computed
// HMAC.
var errSlackSignatureMismatch = errors.New("slack signature verification failed: signature mismatch")

// verifySlackSignature validates an inbound Slack request per Slack's v0
// signing scheme: HMAC-SHA256 over "v0:{timestamp}:{raw_body}" using the
// app's signing secret, hex-encoded with a "v0=" prefix, compared against
// the X-Slack-Signature header via hmac.Equal — never == or bytes.Equal,
// which are not constant-time and are a textbook timing side-channel
// (research/pitfalls.md §5 item 1).
//
// rawBody must be the exact, unmodified bytes Slack sent. Callers must read
// it via a single io.ReadAll(r.Body) before any form/JSON parsing touches
// the request — verifying against a re-encoded or partially-consumed body
// would check different bytes than what Slack actually signed (§5 item 2).
//
// Verification fails closed: any error in this function (missing headers,
// unparsable timestamp, stale timestamp, or mismatched signature) is a
// rejection. There is no "verification unavailable, allow by default" path
// (§5 item 6).
func verifySlackSignature(secret string, headers http.Header, rawBody []byte) error {
	tsHeader := headers.Get("X-Slack-Request-Timestamp")
	sigHeader := headers.Get("X-Slack-Signature")
	if tsHeader == "" || sigHeader == "" {
		return errSlackSignatureMismatch
	}

	ts, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil {
		return errSlackSignatureMismatch
	}

	age := time.Now().Unix() - ts
	if age < 0 {
		age = -age
	}
	if time.Duration(age)*time.Second > slackSignatureReplayWindow {
		return errSlackSignatureStale
	}

	base := "v0:" + tsHeader + ":" + string(rawBody)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(base))
	computed := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(computed), []byte(sigHeader)) {
		return errSlackSignatureMismatch
	}
	return nil
}
