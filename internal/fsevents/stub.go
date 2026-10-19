//go:build !darwin

package fsevents

import "errors"

// ErrUnsupported is returned by every Stream operation off darwin.
var ErrUnsupported = errors.New("fsevents: only supported on darwin")

// Stream is a no-op placeholder so cross-platform callers compile.
type Stream struct{}

func NewStream(Config) (*Stream, error) { return nil, ErrUnsupported }
func (*Stream) Start() error            { return ErrUnsupported }
func (*Stream) Stop()                   {}
func (*Stream) FlushSync()              {}

// CurrentEventID returns 0 off darwin.
func CurrentEventID() EventID { return 0 }

// VolumeUUID returns "" off darwin.
func VolumeUUID(string) string { return "" }
