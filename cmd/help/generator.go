package help

import (
	"fmt"
	"sort"
	"strings"

	"claude-squad/cmd/interfaces"
	"github.com/charmbracelet/lipgloss"
)

// Generator creates help content from the command registry
type Generator struct {
	registry *cmd.CommandRegistry

	// Styles for formatting help content
	titleStyle  lipgloss.Style
	headerStyle lipgloss.Style
	keyStyle    lipgloss.Style
	descStyle   lipgloss.Style
	sepStyle    lipgloss.Style
	warnStyle   lipgloss.Style
}

// NewGenerator creates a new help generator
func NewGenerator(registry *cmd.CommandRegistry) *Generator {
	return &Generator{
		registry:    registry,
		titleStyle:  lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("#7D56F4")),
		headerStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#36CFC9")),
		keyStyle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFCC00")),
		descStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		sepStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("#3C3C3C")),
		warnStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")),
	}
}

// GenerateContextHelp creates a comprehensive help screen for a context
func (g *Generator) GenerateContextHelp(contextID cmd.ContextID) string {
	commands := g.registry.GetCommandsForContext(contextID)
	if len(commands) == 0 {
		return g.titleStyle.Render("No commands available in this context")
	}

	// Get context information
	context, exists := g.registry.GetContext(contextID)
	contextName := string(contextID)
	if exists {
		contextName = context.Name
	}

	var content strings.Builder
	content.WriteString(g.titleStyle.Render(fmt.Sprintf("%s Commands", contextName)))
	content.WriteString("\n\n")

	if exists && context.Description != "" {
		content.WriteString(g.descStyle.Render(context.Description))
		content.WriteString("\n\n")
	}

	// Group commands by category
	categories := g.groupCommandsByCategory(commands)

	// Sort categories by priority
	sortedCategories := make([]cmd.Category, 0, len(categories))
	for category := range categories {
		if !cmd.IsHiddenCategory(category) {
			sortedCategories = append(sortedCategories, category)
		}
	}
	sort.Slice(sortedCategories, func(i, j int) bool {
		return cmd.GetCategoryPriority(sortedCategories[i]) < cmd.GetCategoryPriority(sortedCategories[j])
	})

	// Generate content for each category
	for i, category := range sortedCategories {
		if i > 0 {
			content.WriteString("\n")
		}
		content.WriteString(g.formatCategory(category, categories[category], contextID))
	}

	return content.String()
}

// GenerateStatusLine creates the bottom status line showing key commands
func (g *Generator) GenerateStatusLine(contextID cmd.ContextID) string {
	commands := g.registry.GetCommandsForContext(contextID)

	// Filter to primary commands (non-deprecated, important categories)
	primary := g.filterPrimaryCommands(commands)

	// Sort by category priority
	sort.Slice(primary, func(i, j int) bool {
		return cmd.GetCategoryPriority(primary[i].Category) < cmd.GetCategoryPriority(primary[j].Category)
	})

	var parts []string
	for _, command := range primary {
		keys := g.registry.GetKeysForCommand(command.ID)
		if len(keys) > 0 {
			// Use the first (primary) key binding
			key := keys[0]
			desc := g.truncateDescription(command.Description, 15)

			var part string
			if command.Deprecated != nil {
				part = g.warnStyle.Render(fmt.Sprintf("%s %s", key, desc))
			} else {
				part = fmt.Sprintf("%s %s", g.keyStyle.Render(key), g.descStyle.Render(desc))
			}
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, g.sepStyle.Render(" • "))
}

// GenerateQuickHelp creates a compact help display for overlays
func (g *Generator) GenerateQuickHelp(contextID cmd.ContextID, maxItems int) []string {
	commands := g.registry.GetCommandsForContext(contextID)
	primary := g.filterPrimaryCommands(commands)

	// Limit number of items
	if len(primary) > maxItems {
		primary = primary[:maxItems]
	}

	var items []string
	for _, command := range primary {
		keys := g.registry.GetKeysForCommand(command.ID)
		if len(keys) > 0 {
			key := keys[0]
			desc := g.truncateDescription(command.Description, 25)

			if command.Deprecated != nil {
				items = append(items, g.warnStyle.Render(fmt.Sprintf("%s - %s (deprecated)", key, desc)))
			} else {
				items = append(items, fmt.Sprintf("%s - %s", key, desc))
			}
		}
	}

	return items
}

// ValidateRegistry checks for issues in the command registry
func (g *Generator) ValidateRegistry() []string {
	var issues []string

	// Check for keybinding conflicts
	conflicts := g.registry.DetectConflicts()
	for _, conflict := range conflicts {
		issues = append(issues, fmt.Sprintf("Key conflict in %s: '%s' bound to %v",
			conflict.Context, conflict.Key, conflict.Commands))
	}

	// Check for commands without keys
	allCommands := g.registry.GetAllCommands()
	for id, command := range allCommands {
		keys := g.registry.GetKeysForCommand(id)
		if len(keys) == 0 {
			issues = append(issues, fmt.Sprintf("Command %s has no key bindings", id))
		}
	}

	// Check for deprecated commands without alternatives
	for id, command := range allCommands {
		if command.Deprecated != nil && command.Deprecated.Alternative == "" {
			issues = append(issues, fmt.Sprintf("Deprecated command %s has no alternative specified", id))
		}
	}

	return issues
}

// groupCommandsByCategory groups commands by their category
func (g *Generator) groupCommandsByCategory(commands []*cmd.Command) map[cmd.Category][]*cmd.Command {
	groups := make(map[cmd.Category][]*cmd.Command)

	for _, command := range commands {
		category := command.Category
		if category == "" {
			category = "Other"
		}
		groups[category] = append(groups[category], command)
	}

	// Sort commands within each category by name
	for category := range groups {
		sort.Slice(groups[category], func(i, j int) bool {
			return groups[category][i].Name < groups[category][j].Name
		})
	}

	return groups
}

// formatCategory creates formatted output for a command category
func (g *Generator) formatCategory(category cmd.Category, commands []*cmd.Command, contextID cmd.ContextID) string {
	var content strings.Builder

	content.WriteString(g.headerStyle.Render(string(category) + ":"))
	content.WriteString("\n")

	for _, command := range commands {
		keys := g.registry.GetKeysForCommand(command.ID)
		if len(keys) == 0 {
			continue
		}

		// Format key bindings (show primary key, mention if there are alternatives)
		keyText := keys[0]
		if len(keys) > 1 {
			keyText += fmt.Sprintf(" (%s)", strings.Join(keys[1:], ", "))
		}

		// Calculate padding for alignment
		padding := strings.Repeat(" ", max(0, 12-len(keyText)))

		// Format the line
		line := fmt.Sprintf("  %s%s - %s",
			g.keyStyle.Render(keyText),
			padding,
			g.descStyle.Render(command.Description))

		// Add deprecation warning if needed
		if command.Deprecated != nil {
			line += g.warnStyle.Render(" (deprecated)")
			if command.Deprecated.Alternative != "" {
				if altCmd, exists := g.registry.GetCommand(command.Deprecated.Alternative); exists {
					altKeys := g.registry.GetKeysForCommand(command.Deprecated.Alternative)
					if len(altKeys) > 0 {
						line += g.warnStyle.Render(fmt.Sprintf(" - use '%s' instead", altKeys[0]))
					}
				}
			}
		}

		content.WriteString(line)
		content.WriteString("\n")
	}

	return content.String()
}

// filterPrimaryCommands filters to commands that should appear in status lines
func (g *Generator) filterPrimaryCommands(commands []*cmd.Command) []*cmd.Command {
	var primary []*cmd.Command

	for _, command := range commands {
		// Skip special/hidden categories
		if cmd.IsHiddenCategory(command.Category) {
			continue
		}

		// Include deprecated commands but limit them
		if command.Deprecated != nil {
			// Only include deprecated commands from certain categories
			if command.Category == cmd.CategoryLegacy {
				primary = append(primary, command)
			}
			continue
		}

		primary = append(primary, command)
	}

	return primary
}

// truncateDescription truncates a description to fit in the status line
func (g *Generator) truncateDescription(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}

	// Try to break at word boundary
	if maxLen > 3 {
		for i := maxLen - 3; i > maxLen/2; i-- {
			if desc[i] == ' ' {
				return desc[:i] + "..."
			}
		}
	}

	return desc[:maxLen-3] + "..."
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
