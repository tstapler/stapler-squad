package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"runtime"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	entErrorEvent "github.com/tstapler/stapler-squad/session/ent/errorevent"
)

// errorEventMaxAge bounds how long ErrorEvent rows accumulate. Nothing else
// ever deletes them -- the same unbounded-growth shape as service.log and
// debug-snapshot-*.json before their retention was added (see
// pruneOldSnapshots in debug_snapshot.go), just less visually alarming since
// it's SQLite rows rather than a file size.
const errorEventMaxAge = 30 * 24 * time.Hour

// Normalization mirrors scripts/log-group.sh's sed pipeline (same order:
// uuid, path, inline object, number) so an error whose message embeds
// per-instance detail (a session ID, a byte count) still fingerprints the
// same as every other occurrence of the same underlying error, instead of
// defeating dedup by hashing that detail along with the message.
var (
	fingerprintUUIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	fingerprintPathPattern = regexp.MustCompile(`(/[A-Za-z0-9_.-]+){2,}`)
	fingerprintObjPattern  = regexp.MustCompile(`\{[^{}]*\}`)
	fingerprintNumPattern  = regexp.MustCompile(`[0-9]+`)
)

// normalizeErrorMessage strips dynamic substrings from msg before
// fingerprinting. The raw msg is still what gets stored/displayed --
// only the fingerprint input is normalized.
func normalizeErrorMessage(msg string) string {
	msg = fingerprintUUIDPattern.ReplaceAllString(msg, "<uuid>")
	msg = fingerprintPathPattern.ReplaceAllString(msg, "<path>")
	msg = fingerprintObjPattern.ReplaceAllString(msg, "<obj>")
	msg = fingerprintNumPattern.ReplaceAllString(msg, "<n>")
	return msg
}

// ErrorRegistry deduplicates RPC errors and persists them to SQLite.
// Each unique (message, procedure) pair is stored once and occurrence_count
// is incremented on every subsequent hit.
type ErrorRegistry struct {
	entClient *ent.Client
	enabled   bool
}

// NewErrorRegistry creates an ErrorRegistry backed by entClient.
// Pass enabled=false (or a nil entClient) to make every call a no-op.
func NewErrorRegistry(entClient *ent.Client, enabled bool) *ErrorRegistry {
	return &ErrorRegistry{entClient: entClient, enabled: enabled}
}

// Record deduplicates the error by fingerprint and upserts into SQLite.
// Silently drops the event when disabled or when the client is nil.
// Implements the ErrorRecorder interface consumed by the interceptor.
func (r *ErrorRegistry) Record(ctx context.Context, errVal error, procedure string) {
	if !r.enabled || r.entClient == nil {
		return
	}
	msg := errVal.Error()
	fingerprint := fingerprintFor(normalizeErrorMessage(msg), procedure)
	now := time.Now()
	buf := make([]byte, 16384)
	n := runtime.Stack(buf, false)
	stackTrace := string(buf[:n])

	if err := r.entClient.ErrorEvent.Create().
		SetFingerprint(fingerprint).
		SetErrorType("rpc_error").
		SetMessage(msg).
		SetStackTrace(stackTrace).
		SetRPCProcedure(procedure).
		SetOccurrenceCount(1).
		SetFirstSeen(now).
		SetLastSeen(now).
		OnConflictColumns(entErrorEvent.FieldFingerprint).
		AddOccurrenceCount(1).
		SetLastSeen(now).
		Exec(ctx); err != nil {
		log.Error("ErrorRegistry.Record: failed to persist error event", "err", err)
		return
	}

	r.pruneOld(ctx)
}

// pruneOld deletes ErrorEvent rows not seen in errorEventMaxAge. Best-effort,
// mirroring pruneOldSnapshots in debug_snapshot.go.
func (r *ErrorRegistry) pruneOld(ctx context.Context) {
	cutoff := time.Now().Add(-errorEventMaxAge)
	if _, err := r.entClient.ErrorEvent.Delete().
		Where(entErrorEvent.LastSeenLT(cutoff)).
		Exec(ctx); err != nil {
		log.Warn("ErrorRegistry.pruneOld: failed to prune stale error events", "err", err)
	}
}

// List returns error events ordered by last_seen desc.
// When includeAcknowledged is false only unacknowledged events are returned.
func (r *ErrorRegistry) List(ctx context.Context, includeAcknowledged bool) ([]*ent.ErrorEvent, error) {
	if !r.enabled || r.entClient == nil {
		return nil, nil
	}
	q := r.entClient.ErrorEvent.Query().Order(ent.Desc(entErrorEvent.FieldLastSeen))
	if !includeAcknowledged {
		q = q.Where(entErrorEvent.Acknowledged(false))
	}
	return q.All(ctx)
}

// Acknowledge marks a single error event as acknowledged.
func (r *ErrorRegistry) Acknowledge(ctx context.Context, fingerprint string) error {
	if !r.enabled || r.entClient == nil {
		return nil
	}
	now := time.Now()
	_, err := r.entClient.ErrorEvent.Update().
		Where(entErrorEvent.FingerprintEQ(fingerprint)).
		SetAcknowledged(true).
		SetAcknowledgedAt(now).
		Save(ctx)
	return err
}

func fingerprintFor(msg, procedure string) string {
	h := sha256.Sum256([]byte(msg + "|" + procedure))
	return fmt.Sprintf("%x", h)
}
