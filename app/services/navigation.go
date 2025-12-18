package services

import (
	"claude-squad/ui"
	"sync"
	"time"
)

// NavigationService handles list navigation with debouncing for performance
type NavigationService interface {
	// Core navigation operations
	NavigateUp() error
	NavigateDown() error
	NavigateToIndex(idx int) error
	NavigatePageUp() error
	NavigatePageDown() error
	NavigateHome() error
	NavigateEnd() error

	// Query operations
	GetCurrentIndex() int
	GetVisibleItemsCount() int

	// Configuration
	SetDebounceDelay(delay time.Duration)
}

// navigationService implements NavigationService
type navigationService struct {
	mu            sync.Mutex
	list          *ui.List
	responsiveNav *ResponsiveNavigationManager
}

// NewNavigationService creates a new navigation service with default 150ms debounce delay
func NewNavigationService(list *ui.List) NavigationService {
	return &navigationService{
		list:          list,
		responsiveNav: NewResponsiveNavigationManager(150 * time.Millisecond),
	}
}

// NavigateUp moves selection up one item
func (n *navigationService) NavigateUp() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.responsiveNav.ScheduleNavigation(func() error {
		n.list.Up()
		return nil
	})
}

// NavigateDown moves selection down one item
func (n *navigationService) NavigateDown() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.responsiveNav.ScheduleNavigation(func() error {
		n.list.Down()
		return nil
	})
}

// NavigateToIndex moves selection to a specific index
func (n *navigationService) NavigateToIndex(idx int) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Direct index navigation doesn't use debouncing
	n.list.SetSelectedIdx(idx)
	return nil
}

// NavigatePageUp moves selection up one page
func (n *navigationService) NavigatePageUp() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Page up is approximated as 10 up movements
	return n.responsiveNav.ScheduleNavigation(func() error {
		for i := 0; i < 10; i++ {
			n.list.Up()
		}
		return nil
	})
}

// NavigatePageDown moves selection down one page
func (n *navigationService) NavigatePageDown() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Page down is approximated as 10 down movements
	return n.responsiveNav.ScheduleNavigation(func() error {
		for i := 0; i < 10; i++ {
			n.list.Down()
		}
		return nil
	})
}

// NavigateHome moves selection to the first item
func (n *navigationService) NavigateHome() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Direct jump to first item
	n.list.SetSelectedIdx(0)
	return nil
}

// NavigateEnd moves selection to the last item
func (n *navigationService) NavigateEnd() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Direct jump to last item
	count := len(n.list.GetVisibleItems())
	if count > 0 {
		n.list.SetSelectedIdx(count - 1)
	}
	return nil
}

// GetCurrentIndex returns the current selection index
func (n *navigationService) GetCurrentIndex() int {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Get visible items to determine current index
	selected := n.list.GetSelectedInstance()
	if selected == nil {
		return 0
	}

	visibleItems := n.list.GetVisibleItems()
	for idx, item := range visibleItems {
		if item == selected {
			return idx
		}
	}

	return 0
}

// GetVisibleItemsCount returns the number of visible items
func (n *navigationService) GetVisibleItemsCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()

	return len(n.list.GetVisibleItems())
}

// SetDebounceDelay configures the debounce delay for navigation
func (n *navigationService) SetDebounceDelay(delay time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.responsiveNav.SetDebounceDelay(delay)
}

// ResponsiveNavigationManager provides debounced navigation updates
// This prevents expensive operations during rapid key presses
type ResponsiveNavigationManager struct {
	mu            sync.Mutex
	debounceDelay time.Duration
	lastNav       time.Time
}

// NewResponsiveNavigationManager creates a new responsive navigation manager
func NewResponsiveNavigationManager(debounceDelay time.Duration) *ResponsiveNavigationManager {
	return &ResponsiveNavigationManager{
		debounceDelay: debounceDelay,
	}
}

// ScheduleNavigation executes navigation with debouncing
// If called too quickly after the last navigation, it skips the operation
func (r *ResponsiveNavigationManager) ScheduleNavigation(navFn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.lastNav) < r.debounceDelay {
		// Skip expensive operations during rapid navigation
		// Return nil to indicate success (operation was intentionally skipped)
		return nil
	}

	r.lastNav = now
	return navFn()
}

// SetDebounceDelay updates the debounce delay
func (r *ResponsiveNavigationManager) SetDebounceDelay(delay time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.debounceDelay = delay
}

// GetDebounceDelay returns the current debounce delay
func (r *ResponsiveNavigationManager) GetDebounceDelay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.debounceDelay
}
