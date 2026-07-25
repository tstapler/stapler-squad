package session

import (
	"sync/atomic"
)

// noCopy prevents ControllerManager from being copied after first use.
// go vet -copylocks will flag any copy of a type containing noCopy.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

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
//
// Both controller and statusManager use atomic.Pointer for lock-free concurrent
// access. Write operations (Register/Unregister/Set) are expected to be called
// sequentially from the Instance lifecycle path and are not themselves
// concurrency-safe against each other.
// ControllerManager must not be copied after first use (enforced by noCopy).
type ControllerManager struct {
	_             noCopy
	controller    atomic.Pointer[ClaudeController]
	statusManager atomic.Pointer[InstanceStatusManager]
}

// HasController reports whether a ClaudeController has been registered.
func (cm *ControllerManager) HasController() bool {
	return cm.controller.Load() != nil
}

// GetController returns the current ClaudeController (may be nil).
func (cm *ControllerManager) GetController() *ClaudeController {
	return cm.controller.Load()
}

// SetController replaces the controller. Callers are responsible for stopping
// the old controller before calling this.
func (cm *ControllerManager) SetController(c *ClaudeController) {
	cm.controller.Store(c)
}

// stopAndClear atomically swaps the controller to nil, then stops it outside
// any lock. Calling Stop() under a lock was a deadlock risk because Stop()
// acquires its own lifecycle lock.
func (cm *ControllerManager) stopAndClear() {
	old := cm.controller.Swap(nil)
	if old != nil {
		old.Stop() //nolint:errcheck
	}
}

// StopAndClearController stops the controller (if running) and clears the reference.
func (cm *ControllerManager) StopAndClearController() {
	cm.stopAndClear()
}

// GetStatusManager returns the current InstanceStatusManager (may be nil).
func (cm *ControllerManager) GetStatusManager() *InstanceStatusManager {
	return cm.statusManager.Load()
}

// SetStatusManager replaces the status manager.
func (cm *ControllerManager) SetStatusManager(m *InstanceStatusManager) {
	cm.statusManager.Store(m)
}

// RegisterController wires a new controller into the status manager and stores
// it. Any existing controller is stopped first.
func (cm *ControllerManager) RegisterController(title string, controller *ClaudeController) {
	cm.stopAndClear()
	if mgr := cm.statusManager.Load(); mgr != nil {
		mgr.RegisterController(title, controller)
	}
	cm.controller.Store(controller)
}

// UnregisterController stops and clears the controller, and removes it from
// the status manager.
func (cm *ControllerManager) UnregisterController(title string) {
	if mgr := cm.statusManager.Load(); mgr != nil {
		mgr.UnregisterController(title)
	}
	cm.stopAndClear()
}
