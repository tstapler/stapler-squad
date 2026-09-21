package clihelp

// ResolvedPath is an absolute path to a regular, executable, non-world-writable
// file. Only the prober constructs one.
type ResolvedPath string

// ProbeStatus is the outcome of probing one command.
type ProbeStatus int

const (
	ProbeStatusUnspecified ProbeStatus = iota
	ProbeStatusFoundParsed
	ProbeStatusFoundNoFlags
	ProbeStatusNotFound
	ProbeStatusTimeout
	ProbeStatusError
	ProbeStatusBusy
	ProbeStatusNeedsConfirm
)

func (s ProbeStatus) String() string {
	switch s {
	case ProbeStatusFoundParsed:
		return "FOUND_PARSED"
	case ProbeStatusFoundNoFlags:
		return "FOUND_NO_FLAGS"
	case ProbeStatusNotFound:
		return "NOT_FOUND"
	case ProbeStatusTimeout:
		return "TIMEOUT"
	case ProbeStatusError:
		return "ERROR"
	case ProbeStatusBusy:
		return "BUSY"
	case ProbeStatusNeedsConfirm:
		return "NEEDS_CONFIRM"
	default:
		return "UNSPECIFIED"
	}
}

// ProbeOpts are per-call switches for Probe.
type ProbeOpts struct {
	ConfirmExecute bool
	ResolveOnly    bool
}

// ProbeResult is the outcome of one Probe call. "Found" is derived from Status
// by the caller (see probeResultToProto), never stored.
type ProbeResult struct {
	Status       ProbeStatus
	ResolvedPath ResolvedPath
	IsWrapper    bool
}
