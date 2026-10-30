// Package conditional is the server half of app-level conditional reads: a
// polling client sends the validator (ETag) of the body it already holds in the
// request message, and the server answers not_modified instead of re-sending an
// identical payload.
//
// It rides inside the protobuf message rather than HTTP headers because Connect
// unary calls are POSTs (browsers never revalidate them) and many reads travel
// over the batch endpoint and the unary WebSocket bridge, which have no
// per-call headers. Fields are additive, so an old client simply never sends a
// validator and an old server simply never returns one.
//
// Validators are HMAC(per-process secret, rpc, serialized response body):
//   - derived only from server-side state (the body about to be served), never
//     from client input, so a stale 304 cannot hide a change: any change to the
//     body, including an authorization-driven one, yields a different tag;
//   - bound to the RPC name, so one RPC's tag never validates another;
//   - unforgeable and unguessable without the secret, and invalidated by a
//     restart (the next poll just takes the full response).
package conditional

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Outcome labels the counter. The set is closed so the label stays low cardinality.
type Outcome string

const (
	// OutcomeMiss means the full body was served (no validator, or it did not match).
	OutcomeMiss Outcome = "miss"
	// OutcomeNotModified means the client's validator matched and the body was elided.
	OutcomeNotModified Outcome = "not_modified"
	// OutcomeHit means the body came from a server-side cache without recomputation.
	OutcomeHit Outcome = "hit"

	metricName = "conditional_reads_total"
	tagBytes   = 16
)

var (
	secretOnce sync.Once
	secret     [32]byte
)

func processSecret() []byte {
	secretOnce.Do(func() {
		if _, err := rand.Read(secret[:]); err != nil {
			// A predictable key would make validators forgeable; refuse to serve any.
			panic("conditional: crypto/rand unavailable: " + err.Error())
		}
	})
	return secret[:]
}

// Validator is an opaque ETag. The zero value means "none".
type Validator string

// TagMessage returns the validator for body as served by rpc.
func TagMessage(rpc string, body proto.Message) (Validator, error) {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, processSecret())
	mac.Write([]byte(rpc))
	mac.Write([]byte{0})
	mac.Write(raw)
	return Validator("\"" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:tagBytes]) + "\""), nil
}

// Matches reports whether the client's validator equals the current one. An
// empty validator never matches.
func Matches(ifNoneMatch, etag Validator) bool {
	if ifNoneMatch == "" || etag == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(ifNoneMatch), []byte(etag)) == 1
}

// Check tags the full response body and reports whether the caller may elide
// it. The body must be the complete response with its etag/not_modified fields
// still unset. On a marshal failure it returns no validator and false, which
// degrades to serving the full body.
func Check(rpc string, ifNoneMatch Validator, full proto.Message) (etag Validator, notModified bool) {
	etag, err := TagMessage(rpc, full)
	if err != nil {
		log.Warn("conditional: could not tag response; serving full body", "rpc", rpc, "err", err)
		record(rpc, OutcomeMiss)
		return "", false
	}
	if Matches(ifNoneMatch, etag) {
		record(rpc, OutcomeNotModified)
		return etag, true
	}
	record(rpc, OutcomeMiss)
	return etag, false
}

var (
	tallyMu sync.Mutex
	tally   = map[tallyKey]int64{}
)

type tallyKey struct {
	rpc string
	out Outcome
}

// Record counts one read outcome for rpc. rpc must be a fixed method name, never client text.
func Record(rpc string, out Outcome) { record(rpc, out) }

func record(rpc string, out Outcome) {
	tallyMu.Lock()
	tally[tallyKey{rpc, out}]++
	tallyMu.Unlock()
	c, err := telemetry.GetMeter().Int64Counter(metricName,
		metric.WithDescription("Conditional read outcomes by RPC: hit (server cache), not_modified (body elided), miss (full body)"))
	if err != nil {
		return
	}
	c.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("rpc", rpc), attribute.String("outcome", string(out))))
}

// Count returns the in-process tally for rpc and outcome (tests and diagnostics).
func Count(rpc string, out Outcome) int64 {
	tallyMu.Lock()
	defer tallyMu.Unlock()
	return tally[tallyKey{rpc, out}]
}

// ResetCountsForTest clears the in-process tally.
func ResetCountsForTest() {
	tallyMu.Lock()
	tally = map[tallyKey]int64{}
	tallyMu.Unlock()
}
