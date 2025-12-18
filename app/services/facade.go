package services

import (
	appsession "claude-squad/app/session"
	appui "claude-squad/app/ui"
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Facade provides a unified interface to all application services
// This reduces the home struct's field count and improves maintainability
type Facade interface {
	// Session management operations
	SessionManagement() SessionManagementService

	// Navigation operations
	Navigation() NavigationService

	// Filtering and search operations
	Filtering() FilteringService

	// UI coordination operations
	UICoordination() UICoordinationService
}

// facade implements Facade
type facade struct {
	mu                sync.RWMutex
	sessionManagement SessionManagementService
	navigation        NavigationService
	filtering         FilteringService
	uiCoordination    UICoordinationService
}

// NewFacade creates a new service facade with all required services
func NewFacade(
	storage *session.Storage,
	sessionController appsession.Controller,
	list *ui.List,
	menu *ui.Menu,
	statusBar *ui.StatusBar,
	errBox *ui.ErrBox,
	uiCoordinator appui.Coordinator,
	errorHandler func(error) tea.Cmd,
	instanceLimit int,
) Facade {
	return &facade{
		sessionManagement: NewSessionManagementService(storage, sessionController, list, errorHandler, instanceLimit),
		navigation:        NewNavigationService(list),
		filtering:         NewFilteringService(list),
		uiCoordination:    NewUICoordinationService(uiCoordinator, menu, statusBar, errBox),
	}
}

// SessionManagement returns the session management service
func (f *facade) SessionManagement() SessionManagementService {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.sessionManagement
}

// Navigation returns the navigation service
func (f *facade) Navigation() NavigationService {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.navigation
}

// Filtering returns the filtering service
func (f *facade) Filtering() FilteringService {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.filtering
}

// UICoordination returns the UI coordination service
func (f *facade) UICoordination() UICoordinationService {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.uiCoordination
}

// ===== Convenience methods that delegate to underlying services =====

// NavigateUp delegates to navigation service
func (f *facade) NavigateUp() error {
	return f.Navigation().NavigateUp()
}

// NavigateDown delegates to navigation service
func (f *facade) NavigateDown() error {
	return f.Navigation().NavigateDown()
}

// StartSearch delegates to filtering service
func (f *facade) StartSearch() error {
	return f.Filtering().StartSearch()
}

// UpdateSearchQuery delegates to filtering service
func (f *facade) UpdateSearchQuery(query string) error {
	return f.Filtering().UpdateSearchQuery(query)
}

// ClearSearch delegates to filtering service
func (f *facade) ClearSearch() error {
	return f.Filtering().ClearSearch()
}

// IsSearchActive delegates to filtering service
func (f *facade) IsSearchActive() bool {
	return f.Filtering().IsSearchActive()
}

// GetSearchQuery delegates to filtering service
func (f *facade) GetSearchQuery() string {
	return f.Filtering().GetSearchQuery()
}

// TogglePausedFilter delegates to filtering service
func (f *facade) TogglePausedFilter() error {
	return f.Filtering().TogglePausedFilter()
}

// IsPausedFilterActive delegates to filtering service
func (f *facade) IsPausedFilterActive() bool {
	return f.Filtering().IsPausedFilterActive()
}

// ShowSessionSetupOverlay delegates to UI coordination service
func (f *facade) ShowSessionSetupOverlay(overlay *overlay.SessionSetupOverlay) error {
	return f.UICoordination().ShowSessionSetupOverlay(overlay)
}

// HideSessionSetupOverlay delegates to UI coordination service
func (f *facade) HideSessionSetupOverlay() error {
	return f.UICoordination().HideSessionSetupOverlay()
}

// GetSessionSetupOverlay delegates to UI coordination service
func (f *facade) GetSessionSetupOverlay() *overlay.SessionSetupOverlay {
	return f.UICoordination().GetSessionSetupOverlay()
}

// ShowError delegates to UI coordination service
func (f *facade) ShowError(err error) tea.Cmd {
	return f.UICoordination().ShowError(err)
}

// ClearError delegates to UI coordination service
func (f *facade) ClearError() error {
	return f.UICoordination().ClearError()
}

// UpdateMenuCommands delegates to UI coordination service
func (f *facade) UpdateMenuCommands(commands map[string]string) error {
	return f.UICoordination().UpdateMenuCommands(commands)
}

// GetMenu delegates to UI coordination service
func (f *facade) GetMenu() *ui.Menu {
	return f.UICoordination().GetMenu()
}

// UpdateStatusBar delegates to UI coordination service
func (f *facade) UpdateStatusBar(message string) error {
	return f.UICoordination().UpdateStatusBar(message)
}

// GetStatusBar delegates to UI coordination service
func (f *facade) GetStatusBar() *ui.StatusBar {
	return f.UICoordination().GetStatusBar()
}

// SetDebounceDelay delegates to navigation service
func (f *facade) SetDebounceDelay(delay time.Duration) {
	f.Navigation().SetDebounceDelay(delay)
}

// GetCurrentIndex delegates to navigation service
func (f *facade) GetCurrentIndex() int {
	return f.Navigation().GetCurrentIndex()
}

// GetVisibleItemsCount delegates to navigation service
func (f *facade) GetVisibleItemsCount() int {
	return f.Navigation().GetVisibleItemsCount()
}

// GetFilterState delegates to filtering service
func (f *facade) GetFilterState() FilterState {
	return f.Filtering().GetFilterState()
}
