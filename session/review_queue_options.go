package session

// DeterminerOptions are the attention-inbox flags Determine may consult. The zero
// value is "all flags off", so Determine stays a pure function of its arguments.
type DeterminerOptions struct {
	IdleClockOutput bool
	ProseQuestion   bool
}

// RuleOptions are the flags consumed by the poller's suppression rules (Story 1.2.x).
type RuleOptions struct {
	AckBypassFix bool
}

// FlagOptionsSource resolves the current flag values. It is called once per poll
// cycle; a nil source means every flag is off.
type FlagOptionsSource func() (DeterminerOptions, RuleOptions)

// resolve returns zero options for a nil source.
func (s FlagOptionsSource) resolve() (DeterminerOptions, RuleOptions) {
	if s == nil {
		return DeterminerOptions{}, RuleOptions{}
	}
	return s()
}
