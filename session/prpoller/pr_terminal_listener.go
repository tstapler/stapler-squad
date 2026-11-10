package prpoller

// PRTerminalListener is told, from the poll core, which PullRequestState rows just
// went terminal. Implementations must return quickly: no git or network work.
type PRTerminalListener interface {
	PRStatesWentTerminal(stateIDs []int)
}

// SetPRTerminalListener wires the listener; nil (the default) disables it.
func (p *PRStatusPoller) SetPRTerminalListener(l PRTerminalListener) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.terminalListener = l
}

func (p *PRStatusPoller) notifyTerminal(stateIDs []int) {
	p.mu.RLock()
	l := p.terminalListener
	p.mu.RUnlock()
	if l != nil && len(stateIDs) > 0 {
		l.PRStatesWentTerminal(stateIDs)
	}
}
