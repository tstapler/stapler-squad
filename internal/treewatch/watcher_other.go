//go:build !linux

package treewatch

// Watcher is never constructed off Linux.
type Watcher struct{}

// Start reports ErrUnsupported off Linux.
func Start(Config) (*Watcher, error) { return nil, ErrUnsupported }

// Stop is a no-op.
func (*Watcher) Stop() {}
