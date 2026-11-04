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
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Outcome labels the counter (hit, for a server-side cache, joins when one is added). The set is closed so the label stays low cardinality.
type Outcome string

const (
	// OutcomeMiss means the full body was served (no validator, or it did not match).
	OutcomeMiss Outcome = "miss"
	// OutcomeNotModified means the client's validator matched and the body was elided.
	OutcomeNotModified Outcome = "not_modified"

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

type tallyKey struct {
	rpc string
	out Outcome
}

// tally mirrors the OTel counter so tests can read it without a metric reader.
var tally sync.Map // tallyKey -> *atomic.Int64

// readsCounter resolves the instrument on every use (the SDK returns the same one per meter, so
// this is a map lookup): a counter cached across telemetry.Initialize could stay bound to the
// pre-init no-op meter and silently drop every sample.
func readsCounter() metric.Int64Counter {
	c, err := telemetry.GetMeter().Int64Counter(metricName,
		metric.WithDescription("Conditional read outcomes by RPC: not_modified (body elided) or miss (full body)"))
	if err != nil {
		log.Error("conditional: failed to register "+metricName, "err", err)
		return nil
	}
	return c
}

// record counts one outcome. rpc must be a fixed method name, never client text.
func record(rpc string, out Outcome) {
	n, _ := tally.LoadOrStore(tallyKey{rpc, out}, new(atomic.Int64))
	n.(*atomic.Int64).Add(1)
	if c := readsCounter(); c != nil {
		c.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("rpc", rpc), attribute.String("outcome", string(out))))
	}
}

// Count returns the in-process tally for rpc and outcome (tests and diagnostics).
func Count(rpc string, out Outcome) int64 {
	if n, ok := tally.Load(tallyKey{rpc, out}); ok {
		return n.(*atomic.Int64).Load()
	}
	return 0
}

// ResetCountsForTest clears the in-process tally.
func ResetCountsForTest() {
	tally.Range(func(k, _ any) bool { tally.Delete(k); return true })
}
