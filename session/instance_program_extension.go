package session

// instance_program_extension.go contains the programExtension contract and
// the runtime extension registry StartController/StopController dispatch
// through (session/instance_controller.go). Split out of that file to keep
// it under the file-size lint threshold -- this registry is a self-contained
// concept with no dependency on ClaudeController's own lifecycle code.

// programExtension is implemented by a per-coding-agent-program controller
// lifecycle manager, so StartController/StopController dispatch through one
// interface instead of growing an if-branch per new program (mirroring
// instance_tmux.go's programKind sum type for command building). Currently
// only *piExtension implements this: the default (Claude/plain) path below
// isn't itself an "extension" -- ClaudeController's lifecycle is separate
// business logic that happens to also be named "claude" (claudeExtension,
// by contrast, only holds resume-session data unrelated to controller
// lifecycle -- see its doc comment in instance_claude.go).
//
// The method set is exported even though the interface type itself is not:
// Go's interface satisfaction is structural, so a program extension
// registered from another package via RegisterProgramExtension only needs
// to implement these exported methods -- it never needs to name
// programExtension itself.
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
var programExtensions []programExtension

// RegisterProgramExtension registers a new program extension for runtime program
// detection and command building. This allows users to define custom program
// behaviors dynamically via API/UI/MCP without needing to modify source code.
func RegisterProgramExtension(ext programExtension) {
	programExtensions = append(programExtensions, ext)
}

// UnregisterProgramExtension removes a program extension from runtime detection.
func UnregisterProgramExtension(ext programExtension) {
	for i, e := range programExtensions {
		if e == ext {
			programExtensions = append(programExtensions[:i], programExtensions[i+1:]...)
			break
		}
	}
}

// controllerExtensions returns the ordered set of program extensions
// StartController/StopController check before falling back to the default
// Claude-controller path below. A future built-in program gets a new
// programExtension implementation appended to this slice instead of a new
// if-branch in either method; a third-party program gets registered at
// runtime via RegisterProgramExtension instead and is checked first.
func (i *Instance) controllerExtensions() []programExtension {
	extensions := []programExtension{&i.piExtension}
	for _, ext := range programExtensions {
		if ext.Supported(i) {
			extensions = append([]programExtension{ext}, extensions...)
		}
	}
	return extensions
}
