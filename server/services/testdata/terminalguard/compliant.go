package terminalguard

// goodInput takes the writer and uses it before writing.
func goodInput(w TerminalWriter, inst *Inst, data []byte) {
	if w == nil {
		return
	}
	_, _ = inst.WriteToPTY(data)
}

// goodSeam reaches the sender through the writer.
func goodSeam(w TerminalWriter, data []byte) error {
	return w.Sender().SendInput("", "t", data)
}

type holder struct{ writer TerminalWriter }

// goodField uses a writer field of its receiver.
func (h holder) goodField(inst *Inst) error {
	if h.writer == nil {
		return nil
	}
	return inst.ResizePTY(80, 24)
}

// sendInputToTmux is exempt by name: it holds the tmux subcommand literal.
func sendInputToTmux() []string { return []string{"send-keys", "-t"} }
