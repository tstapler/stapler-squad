package keys

import (
	"github.com/charmbracelet/bubbles/key"
)

type KeyName int

const (
	KeyUp KeyName = iota
	KeyDown
	KeyEnter
	KeyNew
	KeyKill
	KeyQuit
	KeyReview
	KeyPush
	KeySubmit

	KeyTab        // Tab is a special keybinding for switching between panes.
	KeySubmitName // SubmitName is a special keybinding for submitting the name of a new instance.

	KeyCheckout
	KeyResume
	KeyPrompt // New key for entering a prompt
	KeyHelp   // Key for showing help screen
	KeyEsc    // Escape key for cancelling operations

	// Diff keybindings
	KeyShiftUp
	KeyShiftDown
	
	// Session organization keybindings
	KeySearch      // Search for sessions
	KeyRight       // Expand category
	KeyLeft        // Collapse category
	KeyToggleGroup // Toggle expand/collapse category
	KeyFilterPaused // Toggle visibility of paused sessions
	KeyClearFilters // Clear all filters and search
	KeyGit          // Enter git mode (:G command)
)

// GlobalKeyStringsMap is a global, immutable map string to keybinding.
var GlobalKeyStringsMap = map[string]KeyName{
	"up":         KeyUp,
	"k":          KeyUp,
	"down":       KeyDown,
	"j":          KeyDown,
	"shift+up":   KeyShiftUp,
	"shift+down": KeyShiftDown,
	"ctrl+u":     KeyShiftUp,
	"ctrl+d":     KeyShiftDown,
	"N":          KeyPrompt,
	":":          KeyPrompt,
	"enter":      KeyEnter,
	"n":          KeyNew,
	"D":          KeyKill,
	"q":          KeyQuit,
	"tab":        KeyTab,
	"c":          KeyCheckout,
	"r":          KeyResume,
	"P":          KeySubmit,
	"?":          KeyHelp,
	"right":      KeyRight,
	"l":          KeyRight,
	"left":       KeyLeft,
	"h":          KeyLeft,
	"s":          KeySearch,
	"/":          KeySearch,
	"space":      KeyToggleGroup,
	"f":          KeyFilterPaused,
	"C":          KeyClearFilters,
	"g":          KeyGit,
	"esc":        KeyEsc,
}

// GlobalkeyBindings is a global, immutable map of KeyName tot keybinding.
var GlobalkeyBindings = map[KeyName]key.Binding{
	KeyUp: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	KeyDown: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	KeyShiftUp: key.NewBinding(
		key.WithKeys("shift+up", "ctrl+u"),
		key.WithHelp("shift+↑/^u", "scroll up"),
	),
	KeyShiftDown: key.NewBinding(
		key.WithKeys("shift+down", "ctrl+d"),
		key.WithHelp("shift+↓/^d", "scroll down"),
	),
	KeyEnter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("↵", "attach"),
	),
	KeyNew: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	KeyKill: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "kill"),
	),
	KeyHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	KeyQuit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	KeySubmit: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "push branch"),
	),
	KeyPrompt: key.NewBinding(
		key.WithKeys("N", ":"),
		key.WithHelp("N/:", "new with prompt"),
	),
	KeyCheckout: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "checkout"),
	),
	KeyTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch tab"),
	),
	KeyResume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume"),
	),
	
	// Session organization bindings
	KeySearch: key.NewBinding(
		key.WithKeys("s", "/"),
		key.WithHelp("s/", "search sessions"),
	),
	KeyRight: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "expand category"),
	),
	KeyLeft: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "collapse category"),
	),
	KeyToggleGroup: key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "toggle category"),
	),
	KeyFilterPaused: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "filter paused"),
	),
	KeyClearFilters: key.NewBinding(
		key.WithKeys("C"),
		key.WithHelp("C", "clear all filters"),
	),
	KeyGit: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "git status"),
	),

	// General keybinding
	KeyEsc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),

	// -- Special keybindings --

	KeySubmitName: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "submit name"),
	),
}
