package ui

import (
	"strings"

	"claude-squad/session"

	"github.com/charmbracelet/lipgloss"
)

var keyStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#4A4545",
	Dark:  "#C0C0C0",
})

var descStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#5A5454",
	Dark:  "#B8B8B8",
})

var sepStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#C0BCBC",
	Dark:  "#707070",
})

var actionGroupStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))

var separator = " • "
var verticalSeparator = " │ "

var menuStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("205"))

// MenuState represents different states the menu can be in
type MenuState int

const (
	StateDefault MenuState = iota
	StateEmpty
	StateNewInstance
	StatePrompt
	StateCreatingInstance
	StateAdvancedNew
	StateSearch
)

// MenuOption represents a single menu item
type MenuOption struct {
	Key         string
	Description string
	Highlighted bool
}

type Menu struct {
	options       []MenuOption
	height, width int
	state         MenuState
	instance      *session.Instance
	isInDiffTab   bool

	// keyDown is the key which is pressed. Empty string means no key pressed.
	keyDown string
}

// Static menu options - these will be replaced by dynamic command registry lookups
var defaultMenuCommands = []string{"session.new", "nav.search", "org.filter_paused", "sys.help", "sys.quit"}
var newInstanceMenuCommands = []string{} // Enter key for submission
var promptMenuCommands = []string{}      // Enter key for submission
var searchMenuCommands = []string{}      // Enter key for submission

func NewMenu() *Menu {
	return &Menu{
		options:     []MenuOption{}, // Will be populated by updateOptions
		state:       StateEmpty,
		isInDiffTab: false,
		keyDown:     "",
	}
}

func (m *Menu) Keydown(key string) {
	m.keyDown = key
}

func (m *Menu) ClearKeydown() {
	m.keyDown = ""
}

// SetAvailableCommands updates menu options from available commands
// availableCommands is a map of key -> description from the command bridge
func (m *Menu) SetAvailableCommands(availableCommands map[string]string) {
	// Create menu options from the available commands
	var options []MenuOption

	// For now, we'll use a simplified approach - just show the most common commands
	// This will be improved when we have proper command priority/grouping
	commonKeys := []string{"n", "D", "enter", "g", "c", "r", "/", "f", "G", "S", "tab", "?", "q"}

	for _, key := range commonKeys {
		if desc, exists := availableCommands[key]; exists {
			options = append(options, MenuOption{
				Key:         key,
				Description: desc,
				Highlighted: key == m.keyDown,
			})
		}
	}

	m.options = options
}

// SetState updates the menu state and options accordingly
func (m *Menu) SetState(state MenuState) {
	m.state = state
	m.updateOptions()
}

// GetState returns the current menu state
func (m *Menu) GetState() MenuState {
	return m.state
}

// SetInstance updates the current instance and refreshes menu options
func (m *Menu) SetInstance(instance *session.Instance) {
	m.instance = instance
	// Only change the state if we're not in a special state
	if m.state != StateNewInstance && m.state != StatePrompt && m.state != StateAdvancedNew {
		if m.instance != nil {
			m.state = StateDefault
		} else {
			m.state = StateEmpty
		}
	}
	m.updateOptions()
}

// SetInDiffTab updates whether we're currently in the diff tab
func (m *Menu) SetInDiffTab(inDiffTab bool) {
	m.isInDiffTab = inDiffTab
	m.updateOptions()
}

// updateOptions updates the menu options based on current state and instance
// This is now a stub - the menu will be populated via SetAvailableCommands from the command bridge
func (m *Menu) updateOptions() {
	// The command bridge will populate menu options via SetAvailableCommands
	// This method is kept for compatibility but logic has moved to bridge-based system
	switch m.state {
	case StateCreatingInstance, StateAdvancedNew:
		// Clear menu during special states
		m.options = []MenuOption{}
	default:
		// Menu will be populated by command bridge calling SetAvailableCommands
	}
}

// SetSize sets the width of the window. The menu will be centered horizontally within this width.
func (m *Menu) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *Menu) String() string {
	if len(m.options) == 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, "")
	}

	// Calculate available width (leave some padding)
	availableWidth := m.width - 4
	if availableWidth < 20 {
		availableWidth = 20
	}

	// Build menu items with length checking
	var menuItems []string
	currentLineLength := 0
	var currentLine strings.Builder
	actionKeys := []string{"enter", "D", "g", "c", "r"}

	for _, option := range m.options {
		var (
			localActionStyle = actionGroupStyle
			localKeyStyle    = keyStyle
			localDescStyle   = descStyle
		)

		// Highlight if this key is pressed
		if m.keyDown == option.Key {
			localActionStyle = localActionStyle.Underline(true)
			localKeyStyle = localKeyStyle.Underline(true)
			localDescStyle = localDescStyle.Underline(true)
		}

		// Check if this is an action key
		isActionKey := false
		for _, actionKey := range actionKeys {
			if option.Key == actionKey {
				isActionKey = true
				break
			}
		}

		// Build the item text
		var itemText string
		if isActionKey {
			itemText = localActionStyle.Render(option.Key) + " " + localActionStyle.Render(option.Description)
		} else {
			itemText = localKeyStyle.Render(option.Key) + " " + localDescStyle.Render(option.Description)
		}

		// Calculate the length without ANSI codes for width checking
		itemLength := len(option.Key) + 1 + len(option.Description)
		separatorLength := 3 // " • "

		// Check if adding this item would exceed the line length
		needNewLine := false
		if currentLineLength > 0 { // Not the first item on the line
			if currentLineLength+separatorLength+itemLength > availableWidth {
				needNewLine = true
			}
		} else { // First item on line
			if itemLength > availableWidth {
				// Truncate long descriptions
				maxDescLength := availableWidth - len(option.Key) - 1 - 3 // key + space + "..."
				if maxDescLength < 10 {
					maxDescLength = 10
				}
				if len(option.Description) > maxDescLength {
					truncatedDesc := option.Description[:maxDescLength-3] + "..."
					if isActionKey {
						itemText = localActionStyle.Render(option.Key) + " " + localActionStyle.Render(truncatedDesc)
					} else {
						itemText = localKeyStyle.Render(option.Key) + " " + localDescStyle.Render(truncatedDesc)
					}
					itemLength = len(option.Key) + 1 + len(truncatedDesc)
				}
			}
		}

		if needNewLine {
			// Add current line to items and start a new line
			menuItems = append(menuItems, currentLine.String())
			currentLine.Reset()
			currentLineLength = 0
		}

		// Add separator if not the first item on the line
		if currentLineLength > 0 {
			currentLine.WriteString(sepStyle.Render(separator))
			currentLineLength += separatorLength
		}

		// Add the item
		currentLine.WriteString(itemText)
		currentLineLength += itemLength
	}

	// Add the last line if it has content
	if currentLine.Len() > 0 {
		menuItems = append(menuItems, currentLine.String())
	}

	// Join lines and render
	menuText := strings.Join(menuItems, "\n")
	centeredMenuText := menuStyle.Render(menuText)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, centeredMenuText)
}

// GetOptions returns the current menu options for testing
func (m *Menu) GetOptions() []MenuOption {
	return m.options
}
