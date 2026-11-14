package session

import (
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// backendFactory builds one backend's ProcessManager from the caller's options.
type backendFactory func(opts instance.ProcessManagerOptions) instance.ProcessManager

// backendRegistry is the single list of known transports. A new backend is a new
// file implementing ProcessManager (plus BackendKeyed) and one entry here. It is a
// package-level literal, not init() self-registration, so a bare &Instance{} in
// tests still resolves its tmux default.
var backendRegistry = map[domain.ProcessManagerBackend]backendFactory{
	domain.BackendTmux:   func(o instance.ProcessManagerOptions) instance.ProcessManager { return newTmuxBackendFromOpts(o) },
	domain.BackendNative: func(o instance.ProcessManagerOptions) instance.ProcessManager { return NewNativeProcessManager(o) },
	domain.BackendTymux:  func(o instance.ProcessManagerOptions) instance.ProcessManager { return newTymuxBackendFromOpts(o) },
}

func (*TmuxBackend) BackendKey() domain.ProcessManagerBackend          { return domain.BackendTmux }
func (*TymuxBackend) BackendKey() domain.ProcessManagerBackend         { return domain.BackendTymux }
func (*NativeProcessManager) BackendKey() domain.ProcessManagerBackend { return domain.BackendNative }

// buildBackend constructs the registered backend, or returns ErrUnrecognizedBackend.
func buildBackend(key domain.ProcessManagerBackend, opts instance.ProcessManagerOptions) (instance.ProcessManager, error) {
	factory, ok := backendRegistry[key]
	if !ok {
		return nil, fmt.Errorf("%w: %q", instance.ErrUnrecognizedBackend, key)
	}
	return factory(opts), nil
}
