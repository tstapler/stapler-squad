package session

import "sync"

// instance_program_extension.go contains the programExtension contract and
// the runtime extension registry StartController/StopController dispatch
// through (session/instance_controller.go). Split out of that file to keep
// it under the file-size lint threshold -- this registry is a self-contained
// concept with no dependency on ClaudeController's own lifecycle code.

// programExtension lets StartController/StopController dispatch through one
// interface instead of an if-branch per program (mirroring
// instance_tmux.go's programKind sum type). The method set is exported even
// though the interface itself is not, since Go's structural typing means a
// registered extension never needs to name programExtension directly.
// Implementations must be pointer types: UnregisterProgramExtension compares
// values with ==, which panics for a non-pointer type containing a slice,
// map, or func field.
type programExtension interface {
	// Supported reports whether i's current Program/config should route
	// through this extension for a NEW StartController call.
	Supported(i *Instance) bool
	// Running reports whether this extension currently owns live
	// controller-lifecycle state for i, independent of Supported() -- see
	// StopController's Bug 2 fix doc comment for why StopController routes
	// on this instead of re-evaluating Supported().
	Running() bool
	// StartController starts this extension's controller-equivalent
	// lifecycle for i.
	StartController(i *Instance) error
	// StopController stops it. Safe to call even if nothing was ever started.
	StopController(i *Instance)
}

// programExtensions holds custom program extensions registered at runtime
// via RegisterProgramExtension, checked in addition to the built-in
// piExtension every Instance already carries (see controllerExtensions).
// This lets callers (API/UI/MCP) add support for new coding-agent programs
// without modifying this package.
//
// programExtensionsMu guards all reads/writes of programExtensions:
// Register/Unregister are rare, config-time events while controllerExtensions
// is read on every controller lifecycle event, but neither side is hot
// enough to justify atomic.Pointer copy-on-write over a plain RWMutex.
var (
	programExtensionsMu sync.RWMutex
	programExtensions   []programExtension
)

// RegisterProgramExtension registers a new program extension for runtime program
// detection and command building. This allows users to define custom program
// behaviors dynamically via API/UI/MCP without needing to modify source code.
func RegisterProgramExtension(ext programExtension) {
	programExtensionsMu.Lock()
	defer programExtensionsMu.Unlock()
	programExtensions = append(programExtensions, ext)
}

// UnregisterProgramExtension removes a program extension from runtime detection.
func UnregisterProgramExtension(ext programExtension) {
	programExtensionsMu.Lock()
	defer programExtensionsMu.Unlock()
	for i, e := range programExtensions {
		if e == ext {
			programExtensions = append(programExtensions[:i], programExtensions[i+1:]...)
			break
		}
	}
}

// controllerExtensions returns every registered extension plus the built-in
// &i.piExtension, most-recently-registered first with piExtension always
// last as the fallback (pinned by
// TestControllerExtensions_MostRecentlyRegisteredWins). Supported()/Running()
// are deliberately not evaluated here -- StartController and StopController
// are the sole places those predicates are checked, so a
// Running()-but-unsupported extension is still returned and can still be
// stopped.
func (i *Instance) controllerExtensions() []programExtension {
	programExtensionsMu.RLock()
	defer programExtensionsMu.RUnlock()

	extensions := make([]programExtension, 0, len(programExtensions)+1)
	for j := len(programExtensions) - 1; j >= 0; j-- {
		extensions = append(extensions, programExtensions[j])
	}
	return append(extensions, &i.piExtension)
}
