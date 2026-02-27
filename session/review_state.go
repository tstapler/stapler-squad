package session

import "time"

// ReviewState holds all timestamps and state related to the review queue and terminal activity
// tracking for a session. It is embedded in Instance so all field accesses remain unchanged.
//
// Fields are protected by Instance.stateMutex; do not lock ReviewState independently.
// Methods on ReviewState are intentionally non-locking — callers must hold stateMutex
// if concurrent access is possible.
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
// Caller must hold the relevant mutex if concurrent access is possible.
func (rs *ReviewState) IsAcknowledgedAfterOutput() bool {
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
