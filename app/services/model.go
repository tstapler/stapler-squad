package services

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Model represents a framework-agnostic view model
// This abstraction allows services to return models without coupling to BubbleTea
type Model interface {
	// Unwrap returns the underlying framework model
	// For BubbleTea, this returns tea.Model
	Unwrap() interface{}
}

// BubbleTeaModel wraps a BubbleTea model for framework compatibility
type BubbleTeaModel struct {
	model tea.Model
}

func (b BubbleTeaModel) Unwrap() interface{} {
	return b.model
}

// NewModel creates a new framework-agnostic model from a BubbleTea model
func NewModel(model tea.Model) Model {
	if model == nil {
		return nil
	}
	return BubbleTeaModel{model: model}
}

// ToTeaModel converts a Model to a BubbleTea model
// This is used by the adapter layer when interfacing with BubbleTea
func ToTeaModel(model Model) tea.Model {
	if model == nil {
		return nil
	}

	result := model.Unwrap()
	if result == nil {
		return nil
	}

	if teaModel, ok := result.(tea.Model); ok {
		return teaModel
	}

	return nil
}

// UpdateResult represents the result of a model update
// This is a framework-agnostic version of BubbleTea's (tea.Model, tea.Cmd) tuple
type UpdateResult struct {
	Model   Model
	Command Command
}

// NewUpdateResult creates an UpdateResult from BubbleTea components
func NewUpdateResult(model tea.Model, cmd tea.Cmd) UpdateResult {
	return UpdateResult{
		Model:   NewModel(model),
		Command: NewCommand(cmd),
	}
}

// ToTeaUpdate converts an UpdateResult to BubbleTea components
func ToTeaUpdate(result UpdateResult) (tea.Model, tea.Cmd) {
	return ToTeaModel(result.Model), ToTeaCmd(result.Command)
}

// WithModel returns a new UpdateResult with the given model
func (r UpdateResult) WithModel(model Model) UpdateResult {
	return UpdateResult{
		Model:   model,
		Command: r.Command,
	}
}

// WithCommand returns a new UpdateResult with the given command
func (r UpdateResult) WithCommand(cmd Command) UpdateResult {
	return UpdateResult{
		Model:   r.Model,
		Command: cmd,
	}
}

// WithNoCommand returns a new UpdateResult with no command
func (r UpdateResult) WithNoCommand() UpdateResult {
	return UpdateResult{
		Model:   r.Model,
		Command: NoOpCommand{},
	}
}
