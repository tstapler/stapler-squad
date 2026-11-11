package session

import "fmt"

// backendFactory builds one backend's ProcessManager from the caller's options.
type backendFactory func(opts ProcessManagerOptions) ProcessManager

// backendRegistry is the single list of known transports. A new backend is a new
// file implementing ProcessManager (plus BackendKeyed) and one entry here. It is a
// package-level literal, not init() self-registration, so a bare &Instance{} in
// tests still resolves its tmux default.
var backendRegistry = map[ProcessManagerBackend]backendFactory{
	BackendTmux:   func(o ProcessManagerOptions) ProcessManager { return newTmuxBackendFromOpts(o) },
	BackendNative: func(o ProcessManagerOptions) ProcessManager { return NewNativeProcessManager(o) },
	BackendTymux:  func(o ProcessManagerOptions) ProcessManager { return newTymuxBackendFromOpts(o) },
}

// BackendKeyed is the optional capability naming the registry key a ProcessManager
// was built for.
type BackendKeyed interface {
	BackendKey() ProcessManagerBackend
}

func (*TmuxBackend) BackendKey() ProcessManagerBackend          { return BackendTmux }
func (*TymuxBackend) BackendKey() ProcessManagerBackend         { return BackendTymux }
func (*NativeProcessManager) BackendKey() ProcessManagerBackend { return BackendNative }

func knownBackend(key ProcessManagerBackend) bool {
	_, ok := backendRegistry[key]
	return ok
}

// buildBackend constructs the registered backend, or returns ErrUnrecognizedBackend.
func buildBackend(key ProcessManagerBackend, opts ProcessManagerOptions) (ProcessManager, error) {
	factory, ok := backendRegistry[key]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnrecognizedBackend, key)
	}
	return factory(opts), nil
}

// processManagerBackendKey returns the backend a ProcessManager actually is, or ""
// when it declares none (nil or a test fake).
func processManagerBackendKey(pm ProcessManager) ProcessManagerBackend {
	if k, ok := capabilityOf[BackendKeyed](pm); ok {
		return k.BackendKey()
	}
	return ""
}
