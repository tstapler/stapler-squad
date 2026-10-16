package terminalguard

// HeldLease stands in for session.HeldLease: its one defining file is this one.
type HeldLease struct{ owner string }

// NewLease is the sanctioned constructor, in the defining file.
func NewLease() *HeldLease { return &HeldLease{owner: "x"} }
