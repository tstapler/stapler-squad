//go:build !darwin

package dirwatch

// NewFDFree reports false off darwin: inotify/ReadDirectoryChangesW cost no fd
// per watched file, so callers keep using fsnotify.
func NewFDFree(...Option) (Watcher, bool) { return nil, false }
