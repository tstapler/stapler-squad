package commands

import (
	"github.com/tstapler/stapler-squad/cmd/interfaces"
	tea "github.com/charmbracelet/bubbletea"
)

// OrganizationHandlers contains handlers for organization/filtering commands
type OrganizationHandlers struct {
	OnFilterPaused        func() (tea.Model, tea.Cmd)
	OnClearFilters        func() (tea.Model, tea.Cmd)
	OnToggleGroup         func() (tea.Model, tea.Cmd)
	OnCycleGroupingMode   func() (tea.Model, tea.Cmd)
	OnCycleSortMode       func() (tea.Model, tea.Cmd)
	OnToggleSortDirection func() (tea.Model, tea.Cmd)
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

// CycleGroupingModeCommand cycles through different grouping strategies
func CycleGroupingModeCommand(ctx *interfaces.CommandContext) error {
	if organizationHandlers.OnCycleGroupingMode != nil {
		model, teaCmd := organizationHandlers.OnCycleGroupingMode()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// CycleSortModeCommand cycles through different sort modes
// Sequence: LastActivity → CreationDate → TitleAZ → Repository → Branch → Status
func CycleSortModeCommand(ctx *interfaces.CommandContext) error {
	if organizationHandlers.OnCycleSortMode != nil {
		model, teaCmd := organizationHandlers.OnCycleSortMode()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ToggleSortDirectionCommand toggles between ascending and descending sort order
func ToggleSortDirectionCommand(ctx *interfaces.CommandContext) error {
	if organizationHandlers.OnToggleSortDirection != nil {
		model, teaCmd := organizationHandlers.OnToggleSortDirection()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
