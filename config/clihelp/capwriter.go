package clihelp

import "bytes"

// capWriter keeps the first max bytes and reports the rest as written, so the
// child never blocks on a full pipe. It is assigned to both cmd.Stdout and
// cmd.Stderr; os/exec copies through a single goroutine when the two writers
// are identical, so no mutex is needed (checked under -race).
type capWriter struct {
	buf        bytes.Buffer
	max        int
	truncated  bool
	onOverflow func() // called once, on the first byte beyond max
}

func (w *capWriter) Write(p []byte) (int, error) {
	room := w.max - w.buf.Len()
	if room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}
	if len(p) > max(room, 0) && !w.truncated {
		w.truncated = true
		if w.onOverflow != nil {
			w.onOverflow()
		}
	}
	return len(p), nil
}
