package terminalguard

// Fixture units for check (c): callers of the watched surface.

// pinnedWriter is a listed caller.
func pinnedWriter(i *Inst) { _ = i.SendKeys("x") }

// unlistedWriter is a new caller nobody added a row for.
func unlistedWriter(i *Inst) { _ = i.SendKeys("y") }

// pauseLike takes the write lease although it is a lifecycle unit.
func pauseLike(i *Inst) { _, _ = i.TryTerminalWriteLease("pause") }

// streamSite calls the unary constructor, which only unary handlers may.
func streamSite(i *Inst) bool { return AccessForUnary(i) }

// SendKeys and TryTerminalWriteLease mirror the session.Instance members the
// real check watches.
func (*Inst) SendKeys(string) error                           { return nil }
func (*Inst) TryTerminalWriteLease(string) (*HeldLease, bool) { return nil, false }

// AccessForUnary mirrors services.AccessForUnary.
func AccessForUnary(*Inst) bool { return true }
