package terminalguard

// badNoWriter writes with no capability at all (R1).
func badNoWriter(inst *Inst, data []byte) {
	_, _ = inst.WriteToPTY(data)
}

// badIgnoresWriter accepts the capability and never references it (R2).
func badIgnoresWriter(w TerminalWriter, inst *Inst) error {
	_ = 1
	return inst.ResizePTY(80, 24)
}

// badMethodValue passes a primitive as a value without a capability.
func badMethodValue(inst *Inst, run func(func(int, int) error)) {
	run(inst.ResizePTY)
}

// badSeam calls the sender seam directly, not through a writer.
func badSeam(s tmuxInputSender, data []byte) error {
	return s.SendInput("", "t", data)
}

// badLiteral spells a tmux subcommand outside the exempt helpers (check (a2)).
func badLiteral() string { return "send-keys" }

// badConstruct builds the concrete writer outside its defining file (check (a)).
func badConstruct() (TerminalWriter, TerminalWriter, TerminalWriter) {
	var zero paneWriter
	_ = zero
	return &paneWriter{}, new(paneWriter), (*paneWriter)(nil)
}

// badLease builds the lease capability outside its defining file (check (a)).
func badLease() *HeldLease { return &HeldLease{owner: "forged"} }

// badToken builds the steer token outside its defining file in every spelling (check (a)).
func badToken() (steerAuthorization, steerAuthorization, *steerAuthorization, steerAuthorization, steerAuthorization) {
	var zero steerAuthorization
	return steerAuthorization{kind: 3}, zero, new(steerAuthorization), steerAuthorization(rawAuth{}), newSteerAuthorization()
}

// badLink builds the backlog link outside its defining file (check (a)).
func badLink() BacklogReviewLink { return BacklogReviewLink{session: "forged"} }
