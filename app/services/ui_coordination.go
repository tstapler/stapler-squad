package services

import (
	appui "claude-squad/app/ui"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// UICoordinationService handles UI component coordination and overlay management
type UICoordinationService interface {
	// Overlay management
	ShowSessionSetupOverlay(overlay *overlay.SessionSetupOverlay) error
	HideSessionSetupOverlay() error
	GetSessionSetupOverlay() *overlay.SessionSetupOverlay

	// Error handling
	ShowError(err error) tea.Cmd
	ClearError() error

	// Menu management
	UpdateMenuCommands(commands map[string]string) error
	GetMenu() *ui.Menu

	// Status bar management
	UpdateStatusBar(message string) error
	GetStatusBar() *ui.StatusBar
}

// uiCoordinationService implements UICoordinationService
type uiCoordinationService struct {
	mu            sync.Mutex
	uiCoordinator appui.Coordinator
	menu          *ui.Menu
	statusBar     *ui.StatusBar
	errBox        *ui.ErrBox
}

// NewUICoordinationService creates a new UI coordination service
func NewUICoordinationService(
	coordinator appui.Coordinator,
	menu *ui.Menu,
	statusBar *ui.StatusBar,
	errBox *ui.ErrBox,
) UICoordinationService {
	return &uiCoordinationService{
		uiCoordinator: coordinator,
		menu:          menu,
		statusBar:     statusBar,
		errBox:        errBox,
	}
}

// ShowSessionSetupOverlay displays the session setup overlay
func (u *uiCoordinationService) ShowSessionSetupOverlay(ovr *overlay.SessionSetupOverlay) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Set the component in the registry
	if err := u.uiCoordinator.SetComponent(appui.ComponentSessionSetupOverlay, ovr); err != nil {
		return err
	}

	// Show the overlay
	return u.uiCoordinator.ShowOverlay(appui.ComponentSessionSetupOverlay)
}

// HideSessionSetupOverlay hides the session setup overlay
func (u *uiCoordinationService) HideSessionSetupOverlay() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.uiCoordinator.HideOverlay(appui.ComponentSessionSetupOverlay)
}

// GetSessionSetupOverlay returns the current session setup overlay
func (u *uiCoordinationService) GetSessionSetupOverlay() *overlay.SessionSetupOverlay {
	u.mu.Lock()
	defer u.mu.Unlock()

	component := u.uiCoordinator.GetComponentByType(appui.ComponentSessionSetupOverlay)
	if component == nil {
		return nil
	}

	ovr, ok := component.(*overlay.SessionSetupOverlay)
	if !ok {
		return nil
	}

	return ovr
}

// ShowError displays an error message
func (u *uiCoordinationService) ShowError(err error) tea.Cmd {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.errBox.SetError(err)
	return nil
}

// ClearError clears the current error message
func (u *uiCoordinationService) ClearError() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.errBox.Clear()
	return nil
}

// UpdateMenuCommands updates the available menu commands
func (u *uiCoordinationService) UpdateMenuCommands(commands map[string]string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.menu.SetAvailableCommands(commands)
	return nil
}

// GetMenu returns the menu component
func (u *uiCoordinationService) GetMenu() *ui.Menu {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.menu
}

// UpdateStatusBar updates the status bar message
func (u *uiCoordinationService) UpdateStatusBar(message string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.statusBar.SetInfo(message)
	return nil
}

// GetStatusBar returns the status bar component
func (u *uiCoordinationService) GetStatusBar() *ui.StatusBar {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.statusBar
}
