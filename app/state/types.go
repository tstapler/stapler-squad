package state

// State represents the discrete states of the application's state machine
type State int

const (
	// Default is the standard application state showing the session list
	Default State = iota
	// New is the state when the user is creating a new instance
	New
	// Prompt is the state when the user is entering a prompt
	Prompt
	// Help is the state when a help screen is displayed
	Help
	// Confirm is the state when a confirmation modal is displayed
	Confirm
	// CreatingSession is the state when a session is being created asynchronously
	CreatingSession
	// AdvancedNew is the state when the user is using the advanced session setup
	AdvancedNew
	// Git is the state when the git status overlay is displayed
	Git
	// ClaudeSettings is the state when the Claude settings overlay is displayed
	ClaudeSettings
	// ZFSearch is the state when the ZF fuzzy search overlay is displayed
	ZFSearch
	// TagEditor is the state when the tag editor overlay is displayed
	TagEditor
	// HistoryBrowser is the state when the history browser overlay is displayed
	HistoryBrowser
	// ConfigEditor is the state when the config editor overlay is displayed
	ConfigEditor
	// Rename is the state when the rename overlay is displayed
	Rename
)

// String returns a human-readable string representation of the state
func (s State) String() string {
	switch s {
	case Default:
		return "Default"
	case New:
		return "New"
	case Prompt:
		return "Prompt"
	case Help:
		return "Help"
	case Confirm:
		return "Confirm"
	case CreatingSession:
		return "CreatingSession"
	case AdvancedNew:
		return "AdvancedNew"
	case Git:
		return "Git"
	case ClaudeSettings:
		return "ClaudeSettings"
	case ZFSearch:
		return "ZFSearch"
	case TagEditor:
		return "TagEditor"
	case HistoryBrowser:
		return "HistoryBrowser"
	case ConfigEditor:
		return "ConfigEditor"
	case Rename:
		return "Rename"
	default:
		return "Unknown"
	}
}

// IsValid returns true if the state is a valid state value
func (s State) IsValid() bool {
	return s >= Default && s <= Rename
}

// IsOverlayState returns true if the state represents an overlay/modal state
func (s State) IsOverlayState() bool {
	switch s {
	case New, Prompt, Help, Confirm, AdvancedNew, Git, ClaudeSettings, ZFSearch, TagEditor, HistoryBrowser, ConfigEditor, Rename:
		return true
	default:
		return false
	}
}

// IsAsyncState returns true if the state represents an asynchronous operation
func (s State) IsAsyncState() bool {
	return s == CreatingSession
}