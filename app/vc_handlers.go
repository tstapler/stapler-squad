package app

import (
	"claude-squad/cmd"
	"claude-squad/log"
	"claude-squad/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// VC Tab Handler Methods
// These methods handle Version Control tab operations

// handleVCStageFile stages the currently selected file in the VC pane
func (h *home) handleVCStageFile() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		log.ErrorLog.Println("VCPane not available")
		return h, nil
	}

	// Get selected instance for working directory
	selected := h.list.GetSelectedInstance()
	if selected == nil {
		return h, nil
	}

	if err := vcPane.StageSelectedFile(selected); err != nil {
		return h, h.handleError(err)
	}

	log.InfoLog.Println("Staged selected file")
	return h, nil
}

// handleVCUnstageFile unstages the currently selected file in the VC pane
func (h *home) handleVCUnstageFile() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		log.ErrorLog.Println("VCPane not available")
		return h, nil
	}

	// Get selected instance for working directory
	selected := h.list.GetSelectedInstance()
	if selected == nil {
		return h, nil
	}

	if err := vcPane.UnstageSelectedFile(selected); err != nil {
		return h, h.handleError(err)
	}

	log.InfoLog.Println("Unstaged selected file")
	return h, nil
}

// handleVCStageAll stages all changed files
func (h *home) handleVCStageAll() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		log.ErrorLog.Println("VCPane not available")
		return h, nil
	}

	// Get selected instance for working directory
	selected := h.list.GetSelectedInstance()
	if selected == nil {
		return h, nil
	}

	if err := vcPane.StageAllFiles(selected); err != nil {
		return h, h.handleError(err)
	}

	log.InfoLog.Println("Staged all files")
	return h, nil
}

// handleVCUnstageAll unstages all staged files
func (h *home) handleVCUnstageAll() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		log.ErrorLog.Println("VCPane not available")
		return h, nil
	}

	// Get selected instance for working directory
	selected := h.list.GetSelectedInstance()
	if selected == nil {
		return h, nil
	}

	if err := vcPane.UnstageAllFiles(selected); err != nil {
		return h, h.handleError(err)
	}

	log.InfoLog.Println("Unstaged all files")
	return h, nil
}

// handleVCOpenTerminal opens an interactive terminal for VCS operations
func (h *home) handleVCOpenTerminal() (tea.Model, tea.Cmd) {
	// Get selected instance
	selected := h.list.GetSelectedInstance()
	if selected == nil {
		return h, nil
	}

	// Get the appropriate command based on VCS type
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		log.ErrorLog.Println("VCPane not available")
		return h, nil
	}

	// Send the interactive command to the session's tmux
	interactiveCmd := vcPane.GetInteractiveCommand(selected)
	if interactiveCmd == "" {
		// Default to shell if no VCS-specific command
		interactiveCmd = "bash"
	}

	log.InfoLog.Printf("Opening interactive VCS terminal with command: %s", interactiveCmd)

	// Send the command to the session
	if err := selected.SendKeys(interactiveCmd + "\n"); err != nil {
		return h, h.handleError(err)
	}

	return h, nil
}

// handleVCToggleHelp toggles help display in the VC pane
func (h *home) handleVCToggleHelp() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		log.ErrorLog.Println("VCPane not available")
		return h, nil
	}

	vcPane.ToggleHelp()
	log.InfoLog.Println("Toggled VC help display")
	return h, nil
}

// handleVCNavigateUp navigates up in the VC file list
func (h *home) handleVCNavigateUp() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		return h, nil
	}

	vcPane.ScrollUp()
	return h, nil
}

// handleVCNavigateDown navigates down in the VC file list
func (h *home) handleVCNavigateDown() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		return h, nil
	}

	vcPane.ScrollDown()
	return h, nil
}

// handleVCCommandPalette opens the command palette for VCS operations
func (h *home) handleVCCommandPalette() (tea.Model, tea.Cmd) {
	vcPane := h.tabbedWindow.GetVCPane()
	if vcPane == nil {
		log.ErrorLog.Println("VCPane not available")
		return h, nil
	}

	// Set up callbacks before showing
	vcPane.SetCommandPaletteCallbacks(
		func(cmd ui.VCCommand) {
			// Execute the selected command
			log.InfoLog.Printf("Executing VC command: %s", cmd.ID)
			h.executeVCCommand(cmd)
		},
		func() {
			// On cancel - just hide the palette
			log.InfoLog.Println("VC command palette cancelled")
		},
	)

	vcPane.ShowCommandPalette()
	log.InfoLog.Println("Opened VC command palette")
	return h, nil
}

// executeVCCommand executes a command from the VC command palette
func (h *home) executeVCCommand(cmd ui.VCCommand) {
	switch cmd.ID {
	case "stage":
		h.handleVCStageFile()
	case "unstage":
		h.handleVCUnstageFile()
	case "stage_all":
		h.handleVCStageAll()
	case "unstage_all":
		h.handleVCUnstageAll()
	case "terminal":
		h.handleVCOpenTerminal()
	case "help":
		h.handleVCToggleHelp()
	default:
		log.InfoLog.Printf("VC command not implemented: %s", cmd.ID)
	}
}

// updateVCContext updates the bridge context when switching to/from VC tab
func (h *home) updateVCContext() {
	if h.tabbedWindow.IsInVCTab() {
		h.bridge.SetContext(cmd.ContextVCTab)
	} else {
		h.bridge.SetContext(cmd.ContextList)
	}
	h.updateMenuFromContext()
}
