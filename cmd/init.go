package cmd

import (
	"claude-squad/cmd/commands"
)

// InitializeCommands sets up all standard commands in the registry
func InitializeCommands(registry *CommandRegistry) error {
	// Initialize contexts first
	if err := InitializeContexts(registry); err != nil {
		return err
	}

	// Register session management commands
	registry.Register(&Command{
		ID:          "session.new",
		Name:        "New Session",
		Description: "Create a new session",
		Category:    CategorySession,
		Handler:     commands.NewSessionCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("n")

	registry.Register(&Command{
		ID:          "session.kill",
		Name:        "Kill Session",
		Description: "Kill (delete) the selected session",
		Category:    CategorySession,
		Handler:     commands.KillSessionCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("D")

	registry.Register(&Command{
		ID:          "session.attach",
		Name:        "Attach Session",
		Description: "Attach to the selected session",
		Category:    CategorySession,
		Handler:     commands.AttachSessionCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("enter")

	registry.Register(&Command{
		ID:          "session.checkout",
		Name:        "Checkout Session",
		Description: "Checkout: commit changes and pause session",
		Category:    CategorySession,
		Handler:     commands.CheckoutCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("c")

	registry.Register(&Command{
		ID:          "session.resume",
		Name:        "Resume Session",
		Description: "Resume a paused session",
		Category:    CategorySession,
		Handler:     commands.ResumeCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("r")

	registry.Register(&Command{
		ID:          "session.claude_settings",
		Name:        "Claude Settings",
		Description: "Configure Claude Code session settings",
		Category:    CategorySession,
		Handler:     commands.ClaudeSettingsCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("C")

	// Register git integration commands
	registry.Register(&Command{
		ID:          "git.status",
		Name:        "Git Status",
		Description: "Open git status interface (fugitive-style)",
		Category:    CategoryGit,
		Handler:     commands.GitStatusCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("g")

	registry.Register(&Command{
		ID:          "git.legacy_submit",
		Name:        "Push Branch (Legacy)",
		Description: "Push branch (legacy - use 'g' for git workflow)",
		Category:    CategoryLegacy,
		Handler:     commands.LegacySubmitCommand,
		Contexts:    []ContextID{ContextList},
		Deprecated: &DeprecationInfo{
			Message:     "Use git status workflow instead",
			Alternative: "git.status",
		},
	}).BindKey("P")

	// Git status context commands (fugitive-style)
	registry.Register(&Command{
		ID:          "git.stage",
		Name:        "Stage File",
		Description: "Stage the selected file for commit",
		Category:    CategoryGit,
		Handler:     commands.StageFileCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKey("s")

	registry.Register(&Command{
		ID:          "git.unstage",
		Name:        "Unstage File",
		Description: "Unstage the selected file",
		Category:    CategoryGit,
		Handler:     commands.UnstageFileCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKey("u")

	registry.Register(&Command{
		ID:          "git.toggle",
		Name:        "Toggle File Stage",
		Description: "Toggle staging status of the selected file",
		Category:    CategoryGit,
		Handler:     commands.ToggleFileCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKey("-")

	registry.Register(&Command{
		ID:          "git.unstage_all",
		Name:        "Unstage All",
		Description: "Unstage all staged files",
		Category:    CategoryGit,
		Handler:     commands.UnstageAllCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKey("U")

	registry.Register(&Command{
		ID:          "git.commit",
		Name:        "Commit",
		Description: "Create a new commit",
		Category:    CategoryGit,
		Handler:     commands.CommitCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKeys("cc")

	registry.Register(&Command{
		ID:          "git.commit_amend",
		Name:        "Amend Commit",
		Description: "Amend the last commit",
		Category:    CategoryGit,
		Handler:     commands.CommitAmendCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKeys("ca")

	registry.Register(&Command{
		ID:          "git.diff",
		Name:        "Show Diff",
		Description: "Show diff for the selected file",
		Category:    CategoryGit,
		Handler:     commands.ShowDiffCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKeys("dd")

	registry.Register(&Command{
		ID:          "git.push",
		Name:        "Push",
		Description: "Push changes to remote",
		Category:    CategoryGit,
		Handler:     commands.PushCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKey("p")

	registry.Register(&Command{
		ID:          "git.pull",
		Name:        "Pull",
		Description: "Pull changes from remote",
		Category:    CategoryGit,
		Handler:     commands.PullCommand,
		Contexts:    []ContextID{ContextGitStatus},
	}).BindKey("P")

	// Navigation commands
	registry.Register(&Command{
		ID:          "nav.up",
		Name:        "Navigate Up",
		Description: "Navigate up (Vim j/k keys supported)",
		Category:    CategoryNavigation,
		Handler:     commands.UpCommand,
		Contexts:    []ContextID{ContextList, ContextGitStatus},
	}).BindKeys("up", "k")

	registry.Register(&Command{
		ID:          "nav.down",
		Name:        "Navigate Down",
		Description: "Navigate down (Vim j/k keys supported)",
		Category:    CategoryNavigation,
		Handler:     commands.DownCommand,
		Contexts:    []ContextID{ContextList, ContextGitStatus},
	}).BindKeys("down", "j")

	registry.Register(&Command{
		ID:          "nav.left",
		Name:        "Navigate Left",
		Description: "Collapse selected category (Vim h/l navigation)",
		Category:    CategoryNavigation,
		Handler:     commands.LeftCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKeys("left", "h")

	registry.Register(&Command{
		ID:          "nav.right",
		Name:        "Navigate Right",
		Description: "Expand selected category (Vim h/l navigation)",
		Category:    CategoryNavigation,
		Handler:     commands.RightCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKeys("right", "l")

	registry.Register(&Command{
		ID:          "nav.page_up",
		Name:        "Page Up",
		Description: "Scroll up (Vim Ctrl+u supported)",
		Category:    CategoryNavigation,
		Handler:     commands.PageUpCommand,
		Contexts:    []ContextID{ContextList, ContextGitStatus},
	}).BindKeys("pgup", "ctrl+u")

	registry.Register(&Command{
		ID:          "nav.page_down",
		Name:        "Page Down",
		Description: "Scroll down (Vim Ctrl+d supported)",
		Category:    CategoryNavigation,
		Handler:     commands.PageDownCommand,
		Contexts:    []ContextID{ContextList, ContextGitStatus},
	}).BindKeys("pgdown", "ctrl+d")

	registry.Register(&Command{
		ID:          "nav.search",
		Name:        "Search",
		Description: "Search sessions by title (Vim-style search)",
		Category:    CategoryNavigation,
		Handler:     commands.SearchCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("/")

	// Organization commands
	registry.Register(&Command{
		ID:          "org.filter_paused",
		Name:        "Filter Paused",
		Description: "Toggle visibility of paused sessions",
		Category:    CategoryOrganization,
		Handler:     commands.FilterPausedCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("f")

	registry.Register(&Command{
		ID:          "org.clear_filters",
		Name:        "Clear Filters",
		Description: "Clear all filters and search",
		Category:    CategoryOrganization,
		Handler:     commands.ClearFiltersCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("C")

	registry.Register(&Command{
		ID:          "org.toggle_group",
		Name:        "Toggle Group",
		Description: "Toggle expand/collapse category",
		Category:    CategoryOrganization,
		Handler:     commands.ToggleGroupCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("space")

	// System commands (available in most contexts)
	registry.Register(&Command{
		ID:          "sys.help",
		Name:        "Help",
		Description: "Show help screen",
		Category:    CategorySystem,
		Handler:     commands.HelpCommand,
		Contexts:    []ContextID{ContextList, ContextGitStatus},
	}).BindKey("?")

	registry.Register(&Command{
		ID:          "sys.quit",
		Name:        "Quit",
		Description: "Quit the application",
		Category:    CategorySystem,
		Handler:     commands.QuitCommand,
		Contexts:    []ContextID{ContextList, ContextGitStatus, ContextHelp},
	}).BindKeys("q", "ctrl+c")

	registry.Register(&Command{
		ID:          "sys.escape",
		Name:        "Cancel/Exit",
		Description: "Cancel/exit current mode",
		Category:    CategorySystem,
		Handler:     commands.EscapeCommand,
		Contexts:    []ContextID{ContextList, ContextGitStatus, ContextHelp, ContextPrompt, ContextSearch, ContextConfirm},
	}).BindKey("esc")

	registry.Register(&Command{
		ID:          "sys.tab",
		Name:        "Switch Tab",
		Description: "Switch between preview and diff tabs",
		Category:    CategorySystem,
		Handler:     commands.TabCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey("tab")

	registry.Register(&Command{
		ID:          "sys.command_mode",
		Name:        "Command Mode",
		Description: "Enter vim-style command mode",
		Category:    CategorySystem,
		Handler:     commands.CommandModeCommand,
		Contexts:    []ContextID{ContextList},
	}).BindKey(":")

	// Confirmation dialog specific commands
	registry.Register(&Command{
		ID:          "sys.confirm",
		Name:        "Confirm/Accept",
		Description: "Accept/confirm the current action",
		Category:    CategorySystem,
		Handler:     commands.ConfirmCommand,
		Contexts:    []ContextID{ContextConfirm},
	}).BindKeys("enter", "y")

	return nil
}

// GetGlobalRegistry returns the initialized global registry
func GetGlobalRegistry() *CommandRegistry {
	registry := GetCommandRegistry()

	// Initialize once
	if len(registry.GetAllCommands()) == 0 {
		if err := InitializeCommands(registry); err != nil {
			panic("Failed to initialize commands: " + err.Error())
		}
	}

	return registry
}
