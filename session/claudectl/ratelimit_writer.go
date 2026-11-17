package claudectl

// RateLimitWriter is implemented by an InstanceContext whose rate-limit
// recovery input must be serialized against other pane writers (the owning
// Instance takes its terminal write lease). A context without it is written
// to directly: there is nothing to serialize against.
type RateLimitWriter interface {
	WriteRateLimitRecovery(data []byte) (int, error)
}

// recoveryAccessor is the ratelimit.SessionAccessor the controller hands to
// ratelimit.NewManager.
type recoveryAccessor struct {
	InstanceContext
}

func (a recoveryAccessor) WriteToPTY(data []byte) (int, error) {
	if w, ok := a.InstanceContext.(RateLimitWriter); ok {
		return w.WriteRateLimitRecovery(data)
	}
	return a.InstanceContext.WriteToPTY(data)
}
