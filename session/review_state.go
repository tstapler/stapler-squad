package session

import (
	"encoding/binary"
	"encoding/hex"
	"unsafe"

	"strings"
	"time"

	"github.com/spaolacci/murmur3"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
)

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

// updateReviewTimestamps updates terminal activity timestamps based on processed content.
//   - rawContent: original captured output, used for the LastTerminalUpdate non-blank check.
//   - filteredContent: rawContent with tmux banners stripped, used for signature computation.
//   - shouldUpdateMeaningful: true when the content carries meaningful signal (not just banners).
//   - sessionTitle: used only for structured debug logging.
//
// Caller must be running inside the actor's serialized command closure (via i.send()/
// sendSyncErr()) AND hold Instance.mu, so the caller can touchLocked in the same hold -
// see UpdateTerminalTimestamps in instance_approval.go, the sole caller.
// Returns true when any field was updated (caller should rebuild the snapshot).
func updateReviewTimestamps(rs *domain.ReviewState, rawContent, filteredContent string, shouldUpdateMeaningful bool, sessionTitle string) bool {
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
			rs.StoreLastMeaningfulOutputNs(now.UnixNano())
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

// computePromptSignature computes a hash of the prompt content using the last 10 lines.
// Returns "" if content is empty.
// Caller may call this without holding any lock.
func computePromptSignature(content string) string {
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

// detectAndTrackPrompt detects whether the current status represents a new user-facing prompt
// and records it. Returns true only when a NEW prompt is detected (signature changed or first).
// Caller must hold Instance.mu when writing prompt fields.
func detectAndTrackPrompt(rs *domain.ReviewState, content string, statusInfo InstanceStatusInfo, sessionTitle string) bool {
	isPromptState := statusInfo.ClaudeStatus == detection.StatusNeedsApproval ||
		statusInfo.ClaudeStatus == detection.StatusInputRequired
	if !isPromptState {
		return false
	}

	promptSignature := computePromptSignature(content)
	isNewPrompt := promptSignature != rs.LastPromptSignature || rs.LastPromptSignature == ""
	if isNewPrompt {
		rs.LastPromptDetected = time.Now()
		rs.LastPromptSignature = promptSignature
		log.Info("new prompt detected", "session", sessionTitle, "signature", truncateString(promptSignature, 8))
	}
	return isNewPrompt
}
