package overlay

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FuzzyDirectoryBrowser provides FZF-style directory browsing with high-quality fuzzy search
type FuzzyDirectoryBrowser struct {
	// UI Components
	input   textinput.Model
	spinner spinner.Model

	// State
	title         string
	loader        *DirectoryLoader
	results       []DirectoryInfo
	selectedIndex int
	width         int
	height        int
	focused       bool
	loading       bool
	error         error
	query         string

	// Visual styles
	titleStyle      lipgloss.Style
	inputStyle      lipgloss.Style
	resultStyle     lipgloss.Style
	selectedStyle   lipgloss.Style
	breadcrumbStyle lipgloss.Style
	loadingStyle    lipgloss.Style
	errorStyle      lipgloss.Style

	// Callbacks
	onSelect func(directoryPath string)
	onCancel func()

	// Debouncing
	lastQuery     string
	lastQueryTime time.Time
	debounceDelay time.Duration
}

// NewFuzzyDirectoryBrowser creates a new FZF-style directory browser
func NewFuzzyDirectoryBrowser(title, repoRoot string) *FuzzyDirectoryBrowser {
	// Initialize the text input component
	ti := textinput.New()
	ti.Placeholder = "Type to search directories..."
	ti.Focus()

	// Create spinner for loading state
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return &FuzzyDirectoryBrowser{
		input:         ti,
		spinner:       sp,
		title:         title,
		loader:        NewDirectoryLoader(repoRoot),
		selectedIndex: 0,
		focused:       true,
		debounceDelay: 200 * time.Millisecond, // 200ms debounce

		// Initialize styles
		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1),

		inputStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1),

		resultStyle: lipgloss.NewStyle().
			Padding(0, 2),

		selectedStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 2),

		breadcrumbStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1),

		loadingStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Padding(0, 2),

		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Padding(0, 2),
	}
}

// SetCallbacks sets the selection and cancellation callbacks
func (f *FuzzyDirectoryBrowser) SetCallbacks(onSelect func(string), onCancel func()) {
	f.onSelect = onSelect
	f.onCancel = onCancel
}

// SetSize sets the dimensions of the browser
func (f *FuzzyDirectoryBrowser) SetSize(width, height int) {
	f.width = width
	f.height = height
	f.input.Width = width - 4 // Account for borders and padding
}

// Init initializes the component
func (f *FuzzyDirectoryBrowser) Init() tea.Cmd {
	// Load initial directory listing
	return tea.Batch(
		textinput.Blink,
		f.spinner.Tick,
		f.performSearch(""), // Load initial results
	)
}

// Update handles messages and state updates
func (f *FuzzyDirectoryBrowser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if f.onCancel != nil {
				f.onCancel()
			}
			return f, nil

		case "enter":
			if len(f.results) > 0 && f.selectedIndex >= 0 && f.selectedIndex < len(f.results) {
				selected := f.results[f.selectedIndex]

				// Handle special navigation entries
				if selected.Name == ".." {
					// Navigate to parent directory
					err := f.loader.ChangeDirectory(selected.Path)
					if err == nil {
						return f, f.performSearch(f.query)
					}
				} else if selected.Name == "<Repository Root>" {
					// Navigate to repository root
					err := f.loader.ChangeDirectory(f.loader.repoRoot)
					if err == nil {
						return f, f.performSearch(f.query)
					}
				} else if f.onSelect != nil {
					// Regular directory selection
					f.onSelect(selected.Path)
				}
			}
			return f, nil

		case "up", "ctrl+k":
			if f.selectedIndex > 0 {
				f.selectedIndex--
			}
			return f, nil

		case "down", "ctrl+j":
			if f.selectedIndex < len(f.results)-1 {
				f.selectedIndex++
			}
			return f, nil

		case "ctrl+u": // Page up
			f.selectedIndex = max(0, f.selectedIndex-5)
			return f, nil

		case "ctrl+d": // Page down
			f.selectedIndex = min(len(f.results)-1, f.selectedIndex+5)
			return f, nil

		default:
			// Handle input changes
			f.input, cmd = f.input.Update(msg)
			cmds = append(cmds, cmd)

			// Check if query changed
			newQuery := f.input.Value()
			if newQuery != f.lastQuery {
				f.query = newQuery
				f.lastQuery = newQuery
				f.lastQueryTime = time.Now()
				f.selectedIndex = 0 // Reset selection when query changes

				// Debounced search
				cmds = append(cmds, tea.Tick(f.debounceDelay, func(time.Time) tea.Msg {
					return searchDebounceMsg{query: newQuery, timestamp: f.lastQueryTime}
				}))
			}
		}

	case searchDebounceMsg:
		// Only perform search if this is the latest query
		if msg.timestamp.Equal(f.lastQueryTime) {
			cmds = append(cmds, f.performSearch(msg.query))
		}

	case searchResultMsg:
		f.loading = false
		if msg.err != nil {
			f.error = msg.err
		} else {
			f.error = nil
			f.results = msg.results
			// Ensure selected index is within bounds
			if f.selectedIndex >= len(f.results) {
				f.selectedIndex = max(0, len(f.results)-1)
			}
		}

	case spinner.TickMsg:
		f.spinner, cmd = f.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return f, tea.Batch(cmds...)
}

// View renders the fuzzy directory browser
func (f *FuzzyDirectoryBrowser) View() string {
	var sections []string

	// Title section
	sections = append(sections, f.titleStyle.Render(f.title))

	// Breadcrumb section
	breadcrumbs := f.loader.GetBreadcrumbs()
	breadcrumbText := strings.Join(breadcrumbs, " / ")
	sections = append(sections, f.breadcrumbStyle.Render("📁 "+breadcrumbText))

	// Input section
	sections = append(sections, f.inputStyle.Render(f.input.View()))

	// Results section
	if f.loading {
		sections = append(sections, f.loadingStyle.Render(f.spinner.View()+" Searching directories..."))
	} else if f.error != nil {
		sections = append(sections, f.errorStyle.Render("Error: "+f.error.Error()))
	} else if len(f.results) == 0 {
		sections = append(sections, f.resultStyle.Render("No directories found"))
	} else {
		// Calculate visible window
		maxVisible := f.height - 8 // Reserve space for title, breadcrumb, input, etc.
		if maxVisible < 1 {
			maxVisible = 1
		}

		start := 0
		end := len(f.results)

		// Ensure selected item is visible
		if f.selectedIndex >= maxVisible {
			start = f.selectedIndex - maxVisible + 1
			end = start + maxVisible
		} else {
			end = min(maxVisible, len(f.results))
		}

		if end > len(f.results) {
			end = len(f.results)
		}

		// Render visible results
		var resultLines []string
		for i := start; i < end; i++ {
			result := f.results[i]
			line := result.GetDisplayText()

			if i == f.selectedIndex {
				line = f.selectedStyle.Render(line)
			} else {
				line = f.resultStyle.Render(line)
			}
			resultLines = append(resultLines, line)
		}

		sections = append(sections, strings.Join(resultLines, "\n"))

		// Status line
		statusText := fmt.Sprintf("%d/%d directories", f.selectedIndex+1, len(f.results))
		if len(f.results) != end {
			statusText += fmt.Sprintf(" (showing %d-%d)", start+1, end)
		}
		sections = append(sections, f.breadcrumbStyle.Render(statusText))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// Message types for internal communication
type searchDebounceMsg struct {
	query     string
	timestamp time.Time
}

type searchResultMsg struct {
	results []DirectoryInfo
	err     error
}

// performSearch executes the fuzzy search operation
func (f *FuzzyDirectoryBrowser) performSearch(query string) tea.Cmd {
	f.loading = true
	f.error = nil

	return func() tea.Msg {
		results, err := f.loader.FuzzySearchDirectories(query)
		return searchResultMsg{results: results, err: err}
	}
}

// Helper functions (max and min) are defined in overlay.go