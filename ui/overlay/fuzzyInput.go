package overlay

import (
	"claude-squad/ui/debounce"
	"claude-squad/ui/fuzzy"
	"fmt"
	"strings"
	"time"
	
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FuzzyInputOverlay is a reusable component for fuzzy searching with text input
type FuzzyInputOverlay struct {
	// UI Components
	input           textinput.Model
	searcher        *fuzzy.FuzzySearcher
	debouncer       *debounce.Debouncer
	spinner         spinner.Model
	
	// State
	title           string
	placeholder     string
	results         []fuzzy.SearchResult
	selectedIndex   int
	width           int
	height          int
	focused         bool
	loading         bool
	error           error
	
	// Visual styles
	titleStyle      lipgloss.Style
	inputStyle      lipgloss.Style
	resultStyle     lipgloss.Style
	selectedStyle   lipgloss.Style
	highlightStyle  lipgloss.Style
	loadingStyle    lipgloss.Style
	errorStyle      lipgloss.Style
	
	// Callbacks
	onSelect        func(item fuzzy.SearchItem)
	onCancel        func()
}

// NewFuzzyInputOverlay creates a new fuzzy search overlay
func NewFuzzyInputOverlay(title, placeholder string) *FuzzyInputOverlay {
	// Initialize the text input component
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	
	// Create spinner for loading state
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	
	// Create a fuzzy searcher with default config
	fs := fuzzy.NewFuzzySearcher(fuzzy.DefaultConfig())
	
	// Create a debouncer for search input
	db := debounce.New(300 * time.Millisecond)
	
	// Define UI styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		MarginBottom(1)
		
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(30)
		
	resultStyle := lipgloss.NewStyle().
		Padding(0, 1)
		
	selectedStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("#3C3C3C")).
		Foreground(lipgloss.Color("#FFFFFF"))
		
	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FFFF"))
		
	loadingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFF00"))
		
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000"))
	
	return &FuzzyInputOverlay{
		input:         ti,
		searcher:      fs,
		debouncer:     db,
		spinner:       sp,
		title:         title,
		placeholder:   placeholder,
		results:       []fuzzy.SearchResult{},
		selectedIndex: 0,
		width:         50,
		height:        15,
		focused:       true,
		loading:       false,
		error:         nil,
		titleStyle:    titleStyle,
		inputStyle:    inputStyle,
		resultStyle:   resultStyle,
		selectedStyle: selectedStyle,
		highlightStyle: highlightStyle,
		loadingStyle:  loadingStyle,
		errorStyle:    errorStyle,
	}
}

// SetItems sets the items to search
func (f *FuzzyInputOverlay) SetItems(items []fuzzy.SearchItem) {
	f.searcher.SetItems(items)
	f.updateSearch()
}

// SetAsyncLoader sets a function to load items asynchronously
func (f *FuzzyInputOverlay) SetAsyncLoader(loader fuzzy.AsyncLoader) {
	f.searcher.SetAsyncLoader(loader)
}

// SetOnSelect sets the callback function for when an item is selected
func (f *FuzzyInputOverlay) SetOnSelect(callback func(fuzzy.SearchItem)) {
	f.onSelect = callback
}

// SetOnCancel sets the callback function for when the search is cancelled
func (f *FuzzyInputOverlay) SetOnCancel(callback func()) {
	f.onCancel = callback
}

// SetSize sets the size of the overlay
func (f *FuzzyInputOverlay) SetSize(width, height int) {
	f.width = width
	f.height = height
	
	// Adjust input width based on overlay width
	inputWidth := width - 10
	if inputWidth < 30 {
		inputWidth = 30
	}
	f.inputStyle = f.inputStyle.Width(inputWidth)
}

// Focus gives focus to the overlay
func (f *FuzzyInputOverlay) Focus() {
	f.focused = true
	f.input.Focus()
}

// Blur removes focus from the overlay
func (f *FuzzyInputOverlay) Blur() {
	f.focused = false
	f.input.Blur()
}

// GetHeight returns the current height of the rendered component
func (f *FuzzyInputOverlay) GetHeight() int {
	// Calculate based on title + input + results + padding
	resultCount := len(f.results)
	if resultCount > f.height - 5 {
		resultCount = f.height - 5
	}
	
	// Title (1) + input (3) + results + padding (1)
	return 5 + resultCount
}

// updateSearch triggers a debounced search with the current input value
func (f *FuzzyInputOverlay) updateSearch() {
	query := f.input.Value()
	
	// Update loading state
	f.loading = true
	
	// Trigger a debounced search
	f.debouncer.Trigger(func() {
		f.searcher.SetQuery(query, func() {
			// This callback is executed after search is complete
			f.results = f.searcher.GetResults()
			f.loading = f.searcher.IsLoading()
			f.error = f.searcher.GetError()
			
			// Reset selected index when results change
			f.selectedIndex = 0
		})
	})
}

// Update handles keyboard input and other messages
func (f *FuzzyInputOverlay) Update(msg tea.Msg) tea.Cmd {
	// If not focused, ignore all messages
	if !f.focused {
		return nil
	}
	
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Select the current item if available
			if len(f.results) > 0 && f.selectedIndex >= 0 && f.selectedIndex < len(f.results) {
				if f.onSelect != nil {
					f.onSelect(f.results[f.selectedIndex].Item)
				}
				return nil
			}
			return nil
			
		case tea.KeyEsc:
			// Cancel the search
			if f.onCancel != nil {
				f.onCancel()
			}
			return nil
			
		case tea.KeyUp:
			// Navigate up in results
			if f.selectedIndex > 0 {
				f.selectedIndex--
			} else if len(f.results) > 0 {
				// Wrap around to the bottom
				f.selectedIndex = len(f.results) - 1
			}
			return nil
			
		case tea.KeyDown:
			// Navigate down in results
			if f.selectedIndex < len(f.results)-1 {
				f.selectedIndex++
			} else {
				// Wrap around to the top
				f.selectedIndex = 0
			}
			return nil
			
		case tea.KeyTab:
			// Autocomplete with the selected result if available
			if len(f.results) > 0 && f.selectedIndex >= 0 && f.selectedIndex < len(f.results) {
				text := f.results[f.selectedIndex].Item.GetDisplayText()
				f.input.SetValue(text)
				f.updateSearch()
			}
			return nil
		}
	}
	
	// Handle text input updates
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	
	// If input value changed, update search
	f.updateSearch()
	
	return cmd
}

// View renders the overlay
func (f *FuzzyInputOverlay) View() string {
	var sb strings.Builder
	
	// Title
	sb.WriteString(f.titleStyle.Render(f.title))
	sb.WriteString("\n")
	
	// Input field
	sb.WriteString(f.inputStyle.Render(f.input.View()))
	sb.WriteString("\n\n")
	
	// Loading indicator or error
	if f.loading {
		sb.WriteString(f.loadingStyle.Render(fmt.Sprintf("%s Searching...", f.spinner.View())))
		sb.WriteString("\n")
	} else if f.error != nil {
		sb.WriteString(f.errorStyle.Render(fmt.Sprintf("Error: %v", f.error)))
		sb.WriteString("\n")
	}
	
	// Results
	maxResults := f.height - 5
	if len(f.results) > 0 {
		visibleResults := f.results
		if len(visibleResults) > maxResults {
			visibleResults = visibleResults[:maxResults]
		}
		
		for i, result := range visibleResults {
			item := result.Item
			text := item.GetDisplayText()
			matches := result.Matches
			
			// Apply highlighting to matching characters
			var highlighted string
			if len(matches) > 0 {
				var parts []string
				lastIndex := 0
				
				for _, idx := range matches {
					if idx > lastIndex && idx < len(text) {
						// Add text before the match
						if idx > lastIndex {
							parts = append(parts, text[lastIndex:idx])
						}
						
						// Add the highlighted character
						parts = append(parts, f.highlightStyle.Render(string(text[idx])))
						
						lastIndex = idx + 1
					}
				}
				
				// Add remaining text
				if lastIndex < len(text) {
					parts = append(parts, text[lastIndex:])
				}
				
				highlighted = strings.Join(parts, "")
			} else {
				highlighted = text
			}
			
			// Apply selection styling
			if i == f.selectedIndex {
				sb.WriteString(f.selectedStyle.Render(highlighted))
			} else {
				sb.WriteString(f.resultStyle.Render(highlighted))
			}
			sb.WriteString("\n")
		}
		
		// Show count if there are more results than visible
		if len(f.results) > maxResults {
			sb.WriteString(fmt.Sprintf("... and %d more results", len(f.results)-maxResults))
			sb.WriteString("\n")
		}
	} else if f.input.Value() != "" && !f.loading {
		sb.WriteString("No results found")
		sb.WriteString("\n")
	}
	
	return sb.String()
}