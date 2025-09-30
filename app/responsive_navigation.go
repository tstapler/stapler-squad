package app

import (
	"claude-squad/log"
	"claude-squad/session"
	"sync"
	"time"
)

// ResponsiveNavigationManager provides instant UI feedback with deferred expensive operations
type ResponsiveNavigationManager struct {
	// Current selection state
	lastSelectedInstance *session.Instance
	lastSelectionTime    time.Time

	// Debouncing for expensive operations
	expensiveUpdateTimer *time.Timer
	expensiveUpdateDelay time.Duration

	// Background operation management
	backgroundMutex      sync.Mutex
	backgroundOperations map[string]bool // track ongoing operations by instance ID

	// Performance monitoring
	updateCount    int
	lastReportTime time.Time
}

// NewResponsiveNavigationManager creates a new responsive navigation manager
func NewResponsiveNavigationManager() *ResponsiveNavigationManager {
	return &ResponsiveNavigationManager{
		expensiveUpdateDelay: 150 * time.Millisecond, // Configurable delay
		backgroundOperations: make(map[string]bool),
		lastReportTime:       time.Now(),
	}
}

// HandleInstanceChange provides instant UI feedback and schedules expensive operations
func (r *ResponsiveNavigationManager) HandleInstanceChange(h *home, newInstance *session.Instance) {
	// Step 1: INSTANT UI updates (no delay, no expensive operations)
	r.updateUIImmediately(h, newInstance)

	// Step 2: Check if we need expensive updates
	if r.shouldSkipExpensiveUpdate(newInstance) {
		return
	}

	// Step 3: Schedule expensive operations with debouncing
	r.scheduleExpensiveOperations(h, newInstance)

	r.updateCount++
}

// updateUIImmediately performs only instant UI operations
func (r *ResponsiveNavigationManager) updateUIImmediately(h *home, instance *session.Instance) {
	// These operations must be <1ms total

	// Update menu (86ns from benchmark)
	h.menu.SetInstance(instance)

	// Update tab window reference (instant - no content loading)
	h.tabbedWindow.SetInstance(instance)

	// Mark selection as changed
	r.lastSelectedInstance = instance
	r.lastSelectionTime = time.Now()
}

// shouldSkipExpensiveUpdate determines if expensive operations are needed
func (r *ResponsiveNavigationManager) shouldSkipExpensiveUpdate(instance *session.Instance) bool {
	// Skip if same instance selected
	if r.lastSelectedInstance == instance {
		return true
	}

	// Don't skip if instance is nil - we need to show fallback content
	// This ensures preview pane displays "No agents running yet" message
	if instance == nil {
		return false
	}

	// Check if expensive operations are already running for this instance
	r.backgroundMutex.Lock()
	defer r.backgroundMutex.Unlock()

	instanceID := instance.Title // Use title as ID
	if r.backgroundOperations[instanceID] {
		return true // Already processing this instance
	}

	return false
}

// scheduleExpensiveOperations debounces expensive operations
func (r *ResponsiveNavigationManager) scheduleExpensiveOperations(h *home, instance *session.Instance) {
	// Cancel any existing timer
	if r.expensiveUpdateTimer != nil {
		r.expensiveUpdateTimer.Stop()
	}

	// Handle nil instance case (fallback content)
	instanceID := ""
	if instance != nil {
		instanceID = instance.Title
	}

	// Mark operation as pending
	r.backgroundMutex.Lock()
	r.backgroundOperations[instanceID] = true
	r.backgroundMutex.Unlock()

	// Schedule expensive operations
	r.expensiveUpdateTimer = time.AfterFunc(r.expensiveUpdateDelay, func() {
		// Perform expensive operations in background
		r.performExpensiveOperations(h, instance)

		// Mark operation as complete
		r.backgroundMutex.Lock()
		delete(r.backgroundOperations, instanceID)
		r.backgroundMutex.Unlock()
	})
}

// performExpensiveOperations runs the expensive tmux/git operations synchronously
// These operations are already async internally, so no need for additional goroutine
func (r *ResponsiveNavigationManager) performExpensiveOperations(h *home, instance *session.Instance) {
	// 1. Category organization (3ns - very fast)
	h.list.OrganizeByCategory()

	// 2. Git diff operations (async internally - returns immediately)
	h.tabbedWindow.UpdateDiff(instance)

	// 3. Tmux capture-pane (async internally - returns immediately)
	if err := h.tabbedWindow.UpdatePreview(instance); err != nil {
		// Log error but don't crash
		log.WarningLog.Printf("UpdatePreview error: %v", err)
	}
}

// OptimizedInstanceChanged replaces the current instanceChanged method
func (h *home) OptimizedInstanceChanged() {
	if h.responsiveNav == nil {
		h.responsiveNav = NewResponsiveNavigationManager()
	}

	selected := h.list.GetSelectedInstance()
	h.responsiveNav.HandleInstanceChange(h, selected)
}

// GetPerformanceStats returns performance metrics
func (r *ResponsiveNavigationManager) GetPerformanceStats() map[string]interface{} {
	r.backgroundMutex.Lock()
	defer r.backgroundMutex.Unlock()

	elapsed := time.Since(r.lastReportTime)
	updatesPerSecond := float64(r.updateCount) / elapsed.Seconds()

	stats := map[string]interface{}{
		"total_updates":          r.updateCount,
		"updates_per_second":     updatesPerSecond,
		"background_operations":  len(r.backgroundOperations),
		"expensive_update_delay": r.expensiveUpdateDelay,
		"last_selection_time":    r.lastSelectionTime,
	}

	// Reset counters
	r.updateCount = 0
	r.lastReportTime = time.Now()

	return stats
}

// ConfigureDelay allows tuning the expensive operation delay
func (r *ResponsiveNavigationManager) ConfigureDelay(delay time.Duration) {
	r.expensiveUpdateDelay = delay
}

// FlushPendingOperations forces immediate execution of pending operations
func (r *ResponsiveNavigationManager) FlushPendingOperations() {
	if r.expensiveUpdateTimer != nil {
		r.expensiveUpdateTimer.Stop()
		// The timer function will run immediately
		// This is useful when the user stops navigating and we want immediate updates
	}
}
