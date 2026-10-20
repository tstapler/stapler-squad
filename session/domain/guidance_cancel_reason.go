package domain

// CancelReason says why a GuidanceRequest was cancelled. It is stored as a
// plain string in cancel_reason; the legacy free-text reasons stay readable.
type CancelReason string

const (
	// CancelReasonSessionEnded: the asking session was archived, stopped or deleted.
	CancelReasonSessionEnded CancelReason = "session_ended"
	// CancelReasonAnsweredInTerminal: the dialog was answered in the session itself.
	CancelReasonAnsweredInTerminal CancelReason = "answered_in_terminal"
	// CancelReasonDialogGone: the dialog is no longer on screen.
	CancelReasonDialogGone CancelReason = "dialog_gone"
	// CancelReasonDialogReplaced: the session is now asking something else.
	CancelReasonDialogReplaced CancelReason = "dialog_replaced"
)

// String returns the stored form, for CancelGuidanceRequest's reason argument.
func (r CancelReason) String() string { return string(r) }
