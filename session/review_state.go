package session

import (
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"strings"
	"time"

	"github.com/spaolacci/murmur3"
)

// ReviewState holds all timestamps and state related to the review queue and terminal activity
// tracking for a session. It is embedded in Instance so all field accesses remain unchanged.
//
// Fields are protected by Instance.stateMutex; do not lock ReviewState independently.
// Methods on ReviewState are intentionally non-locking — callers must hold stateMutex
// if concurrent access is possible.
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
// All access is either within the session package (under stateMutex) or through
// Instance methods that acquire stateMutex.
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
}

// ---- Package-level helpers -----------------------------------------------

// computeContentSignature computes a MurmurHash3 64-bit hash of terminal content.
// This signature is used to detect actual content changes vs app restarts with unchanged content.
// MurmurHash3 is significantly faster than SHA256 and perfect for non-cryptographic checksums.
// Returns a hex-encoded string representation of the hash (16 characters for 64-bit hash).
func computeContentSignature(content string) string {
	hash := murmur3.Sum64([]byte(content))
	return fmt.Sprintf("%016x", hash)
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
// If LastMeaningfulOutput is zero, returns the duration since the given createdAt time.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) TimeSinceLastMeaningfulOutput(createdAt time.Time) time.Duration {
	if rs.LastMeaningfulOutput.IsZero() {
		return time.Since(createdAt)
	}
	return time.Since(rs.LastMeaningfulOutput)
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
// Returns false when LastMeaningfulOutput is zero: if no output has ever been recorded,
// the acknowledgment cannot logically be "after" output, so the session is not snoozed.
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) IsAcknowledgedAfterOutput() bool {
	if rs.LastMeaningfulOutput.IsZero() {
		return false
	}
	return !rs.LastAcknowledged.IsZero() && rs.LastAcknowledged.After(rs.LastMeaningfulOutput)
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
// Caller must hold Instance.stateMutex.
func (rs *ReviewState) UpdateTimestamps(rawContent, filteredContent string, shouldUpdateMeaningful bool, sessionTitle string) {
	now := time.Now()

	// Always update LastTerminalUpdate for any non-blank raw output.
	if len(strings.TrimSpace(rawContent)) > 0 {
		rs.LastTerminalUpdate = now
	}

	if shouldUpdateMeaningful {
		signature := computeContentSignature(filteredContent)
		if signature != rs.LastOutputSignature {
			rs.LastMeaningfulOutput = now
			rs.LastOutputSignature = signature
			log.LogForSession(sessionTitle, "debug", "Updated LastMeaningfulOutput timestamp")
		} else {
			log.LogForSession(sessionTitle, "debug", "Skipped LastMeaningfulOutput update (content unchanged since last update)")
		}
	} else {
		log.LogForSession(sessionTitle, "debug", "NOT updating LastMeaningfulOutput - content classified as non-meaningful (banners only)")
	}
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
	hash := murmur3.Sum64([]byte(promptContext))
	return fmt.Sprintf("%016x", hash)
}

// DetectAndTrackPrompt detects whether the current status represents a new user-facing prompt
// and records it. Returns true only when a NEW prompt is detected (signature changed or first).
// Caller must hold Instance.stateMutex when writing prompt fields.
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
		log.InfoLog.Printf("[Prompt] New prompt detected for '%s': signature=%s...",
			sessionTitle, truncateString(promptSignature, 8))
	}
	return isNewPrompt
}
