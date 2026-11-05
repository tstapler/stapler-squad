package services

// testWriter returns a read-write TerminalWriter on the real subprocess seam.
func testWriter() TerminalWriter {
	w, err := TerminalReadWrite.Writer(nil)
	if err != nil {
		panic(err)
	}
	return w
}
