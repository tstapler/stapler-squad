package commands

import (
	"claude-squad/cmd/interfaces"
	tea "github.com/charmbracelet/bubbletea"
)

// OrganizationHandlers contains handlers for organization/filtering commands
type OrganizationHandlers struct {
	OnFilterPaused func() (tea.Model, tea.Cmd)
	OnClearFilters func() (tea.Model, tea.Cmd)
	OnToggleGroup  func() (tea.Model, tea.Cmd)
}

var organizationHandlers = &OrganizationHandlers{}

// SetOrganizationHandlers configures the organization command handlers
func SetOrganizationHandlers(handlers *OrganizationHandlers) {
	organizationHandlers = handlers
}

// FilterPausedCommand toggles visibility of paused sessions
func FilterPausedCommand(ctx *interfaces.CommandContext) error {
	if organizationHandlers.OnFilterPaused != nil {
		model, teaCmd := organizationHandlers.OnFilterPaused()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ClearFiltersCommand clears all filters and search
func ClearFiltersCommand(ctx *interfaces.CommandContext) error {
	if organizationHandlers.OnClearFilters != nil {
		model, teaCmd := organizationHandlers.OnClearFilters()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ToggleGroupCommand toggles expand/collapse of category groups
func ToggleGroupCommand(ctx *interfaces.CommandContext) error {
	if organizationHandlers.OnToggleGroup != nil {
		model, teaCmd := organizationHandlers.OnToggleGroup()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
