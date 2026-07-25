package session

import (
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"unsafe"

	"github.com/spaolacci/murmur3"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"strings"
	"time"
)

// ReviewState holds all timestamps and state related to the review queue and terminal activity
// tracking for a session. It is embedded in Instance so all field accesses remain unchanged.
//
// Fields are NOT protected by Instance.mu. Mutation is serialized through the actor's
// send()/sendSyncErr() closures instead (see UpdateTerminalTimestamps in
// instance_approval.go, which routes through i.send() rather than i.mu.Lock()) - the
// same "no locking, serialize via the actor's own command queue" discipline used by
// transitionToLocked et al. Methods on ReviewState are intentionally non-locking -
// callers must be running inside an actor command closure (or otherwise be the sole
// writer) if concurrent access is possible.
//
// Direct field access via Go embedding promotion (inst.LastMeaningfulOutput etc.) is used by:
//   - session/review_queue_poller.go: reads LastMeaningfulOutput, LastAcknowledged,
//     LastAddedToQueue, ProcessingGraceUntil, LastPromptDetected, LastPromptSignature,
//     LastUserResponse, LastViewed, LastTerminalUpdate, LastOutputSignature
//   - server/dependencies.go: reads LastMeaningfulOutput, LastTerminalUpdate,
//     LastAddedToQueue, LastAcknowledged
//   - server/adapters/instance_adapter.go: reads LastTerminalUpdate, LastMeaningfulOutput
//   - server/review_queue_manager.go: writes LastUserResponse directly
//
// All access is either within the session package (via the actor's serialized
// closures) or through Instance methods that route through i.send()/sendSyncErr().
//
// TODO: Migrate cross-package field accesses (server/) to accessor methods to enable
// future encapsulation of ReviewState as a composed (non-embedded) field.
type ReviewState struct {
	// LastAcknowledged tracks when the user last acknowledged this session in the review queue.
	// Sessions acknowledged after their last update won't appear in the queue until they update again.
	LastAcknowledged time.Time

	// LastAddedToQueue tracks when this session was last added to the review queue.
	// Used to prevent notification spam by enforcing a minimum re-add interval.
	LastAddedToQueue time.Time

	// LastTerminalUpdate is the timestamp of the last output received from the terminal (any output).
	LastTerminalUpdate time.Time

	// LastMeaningfulOutput is the timestamp of the last meaningful output (excludes tmux status banners).
	// Used by the review queue to determine session staleness.
	LastMeaningfulOutput time.Time

	// LastOutputSignature is a hash of the terminal content, used to detect actual changes
	// vs app restarts with unchanged content (prevents false "new activity" notifications).
	LastOutputSignature string

	// LastViewed tracks when the user last interacted with this session
	// (viewing the terminal, attaching via tmux, or viewing session details).
	// Used for smarter review queue notifications (don't notify if just viewed).
	LastViewed time.Time

	// LastPromptDetected is the timestamp when we last detected a prompt requiring user input.
	// Used to distinguish new prompts from the same prompt re-appearing.
	LastPromptDetected time.Time

	// LastPromptSignature is a hash of the prompt content (last 10 lines before cursor).
	// Used to determine if this is the same prompt or a new one.
	LastPromptSignature string

	// LastUserResponse is the timestamp when the user last provided input/interaction.
	// Used to determine if user responded AFTER a prompt was detected.
	LastUserResponse time.Time

	// ProcessingGraceUntil is the deadline for waiting for the session to respond after
	// user interaction. If the session shows no activity by this time, it may be re-added
	// to the review queue.
	ProcessingGraceUntil time.Time

	// lastMeaningfulOutputNs is a lock-free shadow of LastMeaningfulOutput stored as
	// UnixNano. Written under mu (write); read without any lock via atomic ops.
	// Zero means "not yet recorded". Use SyncAtomicTimestamps() after construction.
	lastMeaningfulOutputNs int64

	// lastAcknowledgedNs is a lock-free shadow of LastAcknowledged stored as UnixNano,
	// following the exact same pattern as lastMeaningfulOutputNs. Written under mu
	// (MarkAcknowledged, instance_state.go); read without any lock via atomic ops.
	// Zero means "not yet recorded". Use SyncAtomicTimestamps() after construction.
	lastAcknowledgedNs int64
}

// SyncAtomicTimestamps initialises atomic shadow fields from their time.Time counterparts.
// Must be called once after constructing ReviewState from persisted or restored data so that
// lock-free readers see the correct initial value immediately.
func (rs *ReviewState) SyncAtomicTimestamps() {
	if !rs.LastMeaningfulOutput.IsZero() {
		atomic.StoreInt64(&rs.lastMeaningfulOutputNs, rs.LastMeaningfulOutput.UnixNano())
	}
	if !rs.LastAcknowledged.IsZero() {
		atomic.StoreInt64(&rs.lastAcknowledgedNs, rs.LastAcknowledged.UnixNano())
	}
}

// loadLastMeaningfulOutputNs returns the nanosecond timestamp without acquiring any lock.
// Returns 0 when no meaningful output has been recorded yet.
func (rs *ReviewState) loadLastMeaningfulOutputNs() int64 {
	return atomic.LoadInt64(&rs.lastMeaningfulOutputNs)
}

// loadLastAcknowledgedNs returns the nanosecond timestamp without acquiring any lock.
// Returns 0 when the session has not yet been acknowledged.
func (rs *ReviewState) loadLastAcknowledgedNs() int64 {
	return atomic.LoadInt64(&rs.lastAcknowledgedNs)
}

// ---- Package-level helpers -----------------------------------------------

// computeContentSignature computes a MurmurHash3 64-bit hash of terminal content.
// This signature is used to detect actual content changes vs app restarts with unchanged content.
// MurmurHash3 is significantly faster than SHA256 and perfect for non-cryptographic checksums.
// Returns a 16-character hex string. Uses unsafe.StringData to avoid a []byte copy.
func computeContentSignature(content string) string {
	var b []byte
	if len(content) > 0 {
		b = unsafe.Slice(unsafe.StringData(content), len(content))
	}
	hash := murmur3.Sum64(b)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], hash)
	return hex.EncodeToString(buf[:])
}

// truncateString truncates s to maxLen characters. Used for log messages.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// ---- ReviewState methods --------------------------------------------------

// TimeSinceLastMeaningfulOutput returns how long ago meaningful terminal output was received.
// If no meaningful output has been recorded yet, returns the duration since the given
// createdAt time.
//
// Lock-free: reads the atomic shadow (lastMeaningfulOutputNs) via loadLastMeaningfulOutputNs()
// rather than the plain LastMeaningfulOutput field, so this method is safe to call from any
// goroutine — including outside the actor's serialized command closures (e.g.
// HibernationSweeper.sweep(), which runs on its own independent background goroutine).
func (rs *ReviewState) TimeSinceLastMeaningfulOutput(createdAt time.Time) time.Duration {
	ns := rs.loadLastMeaningfulOutputNs()
	if ns == 0 {
		return time.Since(createdAt)
	}
	return time.Since(time.Unix(0, ns))
}

// TimeSinceLastTerminalUpdate returns how long ago any terminal output was received.
// If LastTerminalUpdate is zero, returns the duration since the given createdAt time.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) TimeSinceLastTerminalUpdate(createdAt time.Time) time.Duration {
	if rs.LastTerminalUpdate.IsZero() {
		return time.Since(createdAt)
	}
	return time.Since(rs.LastTerminalUpdate)
}

// IsAcknowledgedAfterOutput returns true if the user acknowledged this session more recently
// than the last meaningful terminal output — meaning no new output has occurred since the
// user last dismissed the session from the review queue.
// Returns false when no meaningful output has been recorded yet: the acknowledgment cannot
// logically be "after" output that never happened, so the session is not snoozed.
//
// Lock-free: reads both atomic shadows (lastMeaningfulOutputNs, lastAcknowledgedNs) rather
// than the plain LastMeaningfulOutput/LastAcknowledged fields, so this method is safe to call
// from any goroutine — including outside the actor's serialized command closures (e.g.
// review_queue_determiner.go's Determine(), called directly on the live *Instance from
// ReviewQueuePoller's own independent background goroutine).
func (rs *ReviewState) IsAcknowledgedAfterOutput() bool {
	outputNs := rs.loadLastMeaningfulOutputNs()
	if outputNs == 0 {
		return false
	}
	ackNs := rs.loadLastAcknowledgedNs()
	return ackNs != 0 && ackNs > outputNs
}

// IsInProcessingGracePeriod returns true if the session is within its processing grace window.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) IsInProcessingGracePeriod() bool {
	return !rs.ProcessingGraceUntil.IsZero() && time.Now().Before(rs.ProcessingGraceUntil)
}

// UserRespondedAfterPrompt returns true if the user responded (LastUserResponse) after
// a prompt was detected (LastPromptDetected), indicating the session is no longer waiting.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) UserRespondedAfterPrompt() bool {
	return !rs.LastUserResponse.IsZero() &&
		!rs.LastPromptDetected.IsZero() &&
		rs.LastUserResponse.After(rs.LastPromptDetected)
}

// UpdateTimestamps updates terminal activity timestamps based on processed content.
//   - rawContent: original captured output, used for the LastTerminalUpdate non-blank check.
//   - filteredContent: rawContent with tmux banners stripped, used for signature computation.
//   - shouldUpdateMeaningful: true when the content carries meaningful signal (not just banners).
//   - sessionTitle: used only for structured debug logging.
//
// Caller must be running inside the actor's serialized command closure (via i.send()/
// sendSyncErr()), not holding Instance.mu - see UpdateTerminalTimestamps in
// instance_approval.go, the sole caller.
// Returns true when any field was updated (caller should rebuild the snapshot).
func (rs *ReviewState) UpdateTimestamps(rawContent, filteredContent string, shouldUpdateMeaningful bool, sessionTitle string) bool {
	now := time.Now()
	changed := false

	// Always update LastTerminalUpdate for any non-blank raw output.
	if len(strings.TrimSpace(rawContent)) > 0 {
		rs.LastTerminalUpdate = now
		changed = true
	}

	if shouldUpdateMeaningful {
		signature := computeContentSignature(filteredContent)
		if signature != rs.LastOutputSignature {
			rs.LastMeaningfulOutput = now
			atomic.StoreInt64(&rs.lastMeaningfulOutputNs, now.UnixNano())
			rs.LastOutputSignature = signature
			changed = true
			log.ForSession(sessionTitle).Debug("Updated LastMeaningfulOutput timestamp")
		} else {
			log.ForSession(sessionTitle).Debug("Skipped LastMeaningfulOutput update (content unchanged since last update)")
		}
	} else {
		log.ForSession(sessionTitle).Debug("NOT updating LastMeaningfulOutput - content classified as non-meaningful (banners only)")
	}
	return changed
}

// ComputePromptSignature computes a hash of the prompt content using the last 10 lines.
// Returns "" if content is empty.
// Caller may call this without holding any lock.
func (rs *ReviewState) ComputePromptSignature(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	const contextLines = 10
	startIdx := len(lines) - contextLines
	if startIdx < 0 {
		startIdx = 0
	}
	promptContext := strings.Join(lines[startIdx:], "\n")
	var b []byte
	if len(promptContext) > 0 {
		b = unsafe.Slice(unsafe.StringData(promptContext), len(promptContext))
	}
	hash := murmur3.Sum64(b)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], hash)
	return hex.EncodeToString(buf[:])
}

// DetectAndTrackPrompt detects whether the current status represents a new user-facing prompt
// and records it. Returns true only when a NEW prompt is detected (signature changed or first).
// Caller must hold Instance.mu when writing prompt fields.
func (rs *ReviewState) DetectAndTrackPrompt(content string, statusInfo InstanceStatusInfo, sessionTitle string) bool {
	isPromptState := statusInfo.ClaudeStatus == detection.StatusNeedsApproval ||
		statusInfo.ClaudeStatus == detection.StatusInputRequired
	if !isPromptState {
		return false
	}

	promptSignature := rs.ComputePromptSignature(content)
	isNewPrompt := promptSignature != rs.LastPromptSignature || rs.LastPromptSignature == ""
	if isNewPrompt {
		rs.LastPromptDetected = time.Now()
		rs.LastPromptSignature = promptSignature
		log.Info("new prompt detected", "session", sessionTitle, "signature", truncateString(promptSignature, 8))
	}
	return isNewPrompt
}
