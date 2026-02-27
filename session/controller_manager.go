package session

// ControllerManager owns the ClaudeController and InstanceStatusManager
// references that were previously bare fields on Instance.
//
// Instance keeps thin wrapper methods (with lifecycle guards) that delegate
// here. ControllerManager itself has no knowledge of Instance lifecycle; it
// only manages the controller and status-manager references.
//
// Note: claudeSession is intentionally NOT included here because it is a
// rich data object with complex lifecycle management (persistence, re-attachment,
// session selection) that is tightly coupled to Instance business logic.
// It remains a direct field on Instance for now.
type ControllerManager struct {
	controller    *ClaudeController
	statusManager *InstanceStatusManager
}

// HasController reports whether a ClaudeController has been registered.
func (cm *ControllerManager) HasController() bool {
	return cm.controller != nil
}

// GetController returns the current ClaudeController (may be nil).
func (cm *ControllerManager) GetController() *ClaudeController {
	return cm.controller
}

// SetController replaces the controller. Callers are responsible for stopping
// the old controller before calling this.
func (cm *ControllerManager) SetController(c *ClaudeController) {
	cm.controller = c
}

// StopAndClearController stops the controller (if running) and clears the reference.
func (cm *ControllerManager) StopAndClearController() {
	if cm.controller != nil {
		cm.controller.Stop()
		cm.controller = nil
	}
}

// GetStatusManager returns the current InstanceStatusManager (may be nil).
func (cm *ControllerManager) GetStatusManager() *InstanceStatusManager {
	return cm.statusManager
}

// SetStatusManager replaces the status manager.
func (cm *ControllerManager) SetStatusManager(m *InstanceStatusManager) {
	cm.statusManager = m
}

// RegisterController wires a new controller into the status manager and stores
// it. Any existing controller is stopped first.
func (cm *ControllerManager) RegisterController(title string, controller *ClaudeController) {
	cm.StopAndClearController()
	if cm.statusManager != nil {
		cm.statusManager.RegisterController(title, controller)
	}
	cm.controller = controller
}

// UnregisterController stops and clears the controller, and removes it from
// the status manager.
func (cm *ControllerManager) UnregisterController(title string) {
	if cm.statusManager != nil {
		cm.statusManager.UnregisterController(title)
	}
	cm.StopAndClearController()
}
