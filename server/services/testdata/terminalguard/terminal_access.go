// Package terminalguard is a fixture for the Story 5.1d guard checks. It mimics the
// names the checks key on (TerminalWriter, paneWriter, the primitive method names and
// the tmuxInputSender seam) and contains both compliant and non-compliant code, so
// the negative controls can assert each violation is found.
package terminalguard

// TerminalWriter mirrors server/services.TerminalWriter.
type TerminalWriter interface{ Sender() tmuxInputSender }

type tmuxInputSender interface {
	SendInput(socket, session string, data []byte) error
}

// paneWriter is defined here, so constructing it in this file is allowed.
type paneWriter struct{ s tmuxInputSender }

func (w *paneWriter) Sender() tmuxInputSender { return w.s }

// NewWriter is the one allowed construction site.
func NewWriter(s tmuxInputSender) TerminalWriter { return &paneWriter{s: s} }

// Inst mirrors *session.Instance's primitive method names.
type Inst struct{}

func (*Inst) WriteToPTY([]byte) (int, error) { return 0, nil }
func (*Inst) ResizePTY(int, int) error       { return nil }
