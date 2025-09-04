package commands

import (
	"claude-squad/cmd/interfaces"
	tea "github.com/charmbracelet/bubbletea"
)

// GitHandlers contains handlers for git-related commands
type GitHandlers struct {
	OnGitStatus    func() (tea.Model, tea.Cmd)
	OnStageFile    func(filePath string) (tea.Model, tea.Cmd)
	OnUnstageFile  func(filePath string) (tea.Model, tea.Cmd)
	OnToggleFile   func(filePath string) (tea.Model, tea.Cmd)
	OnUnstageAll   func() (tea.Model, tea.Cmd)
	OnCommit       func() (tea.Model, tea.Cmd)
	OnCommitAmend  func() (tea.Model, tea.Cmd)
	OnShowDiff     func(filePath string) (tea.Model, tea.Cmd)
	OnPush         func() (tea.Model, tea.Cmd)
	OnPull         func() (tea.Model, tea.Cmd)
	OnLegacySubmit func() (tea.Model, tea.Cmd) // For backwards compatibility
}

var gitHandlers = &GitHandlers{}

// SetGitHandlers configures the git command handlers
func SetGitHandlers(handlers *GitHandlers) {
	gitHandlers = handlers
}

// GitStatusCommand opens the fugitive-style git status interface
func GitStatusCommand(ctx *interfaces.CommandContext) error {
	if gitHandlers.OnGitStatus != nil {
		model, teaCmd := gitHandlers.OnGitStatus()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// StageFileCommand stages the selected file for commit
func StageFileCommand(ctx *interfaces.CommandContext) error {
	filePath, _ := ctx.Args["filePath"].(string)
	if gitHandlers.OnStageFile != nil {
		model, teaCmd := gitHandlers.OnStageFile(filePath)
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// UnstageFileCommand unstages the selected file
func UnstageFileCommand(ctx *interfaces.CommandContext) error {
	filePath, _ := ctx.Args["filePath"].(string)
	if gitHandlers.OnUnstageFile != nil {
		model, teaCmd := gitHandlers.OnUnstageFile(filePath)
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ToggleFileCommand toggles staging status of the selected file
func ToggleFileCommand(ctx *interfaces.CommandContext) error {
	filePath, _ := ctx.Args["filePath"].(string)
	if gitHandlers.OnToggleFile != nil {
		model, teaCmd := gitHandlers.OnToggleFile(filePath)
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// UnstageAllCommand unstages all staged files
func UnstageAllCommand(ctx *interfaces.CommandContext) error {
	if gitHandlers.OnUnstageAll != nil {
		model, teaCmd := gitHandlers.OnUnstageAll()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// CommitCommand creates a new commit
func CommitCommand(ctx *interfaces.CommandContext) error {
	if gitHandlers.OnCommit != nil {
		model, teaCmd := gitHandlers.OnCommit()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// CommitAmendCommand amends the last commit
func CommitAmendCommand(ctx *interfaces.CommandContext) error {
	if gitHandlers.OnCommitAmend != nil {
		model, teaCmd := gitHandlers.OnCommitAmend()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ShowDiffCommand shows diff for the selected file
func ShowDiffCommand(ctx *interfaces.CommandContext) error {
	filePath, _ := ctx.Args["filePath"].(string)
	if gitHandlers.OnShowDiff != nil {
		model, teaCmd := gitHandlers.OnShowDiff(filePath)
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// PushCommand pushes changes to remote
func PushCommand(ctx *interfaces.CommandContext) error {
	if gitHandlers.OnPush != nil {
		model, teaCmd := gitHandlers.OnPush()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// PullCommand pulls changes from remote
func PullCommand(ctx *interfaces.CommandContext) error {
	if gitHandlers.OnPull != nil {
		model, teaCmd := gitHandlers.OnPull()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// LegacySubmitCommand provides backward compatibility for the old submit workflow
func LegacySubmitCommand(ctx *interfaces.CommandContext) error {
	if gitHandlers.OnLegacySubmit != nil {
		model, teaCmd := gitHandlers.OnLegacySubmit()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
