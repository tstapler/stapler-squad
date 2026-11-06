package dtypes

// ProfileDialog is what a QuestionDialogProfile reads off a live capture: the
// question text and the ordered option labels of one on-screen dialog.
type ProfileDialog struct {
	// Profile is the profile name that matched; it is stored as questions_json.profile.
	Profile  string
	Header   string
	Question string
	Options  []string
}

// QuestionDialogProfile is the Strategy for one harness's question dialog. A
// harness is replyable only when it has a profile measured against a real
// capture (Spike S5); every other harness reports read_only with the question
// text. Implementations are stateless and safe for concurrent use.
type QuestionDialogProfile interface {
	// Name is the stable profile id ("claude", "opencode").
	Name() string
	// Extract reads the dialog off capture. It is bottom-anchored: text an agent
	// can forge in its transcript or output must not match.
	Extract(capture string) (ProfileDialog, bool)
	// Verify is the pre-write identity guard: capture shows exactly want and
	// nothing else below it.
	Verify(capture string, want ProfileDialog) bool
	// AnswerKeys is the single write that picks option optionIndex (0-based).
	// ok is false for an option whose keystroke was never measured.
	AnswerKeys(optionIndex int, want ProfileDialog) (keys string, ok bool)
}
