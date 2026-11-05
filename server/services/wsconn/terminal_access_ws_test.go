package wsconn

import "github.com/tstapler/stapler-squad/server/services"

// testWriter returns a read-write TerminalWriter on the real subprocess seam.
func testWriter() services.TerminalWriter {
	w, err := services.TerminalReadWrite.Writer(nil)
	if err != nil {
		panic(err)
	}
	return w
}
