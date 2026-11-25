package wsconn

import (
	"github.com/tstapler/stapler-squad/server/services/access"
)

// testWriter returns a read-write TerminalWriter on the real subprocess seam.
func testWriter() access.TerminalWriter {
	w, err := access.TerminalReadWrite.Writer(nil)
	if err != nil {
		panic(err)
	}
	return w
}
