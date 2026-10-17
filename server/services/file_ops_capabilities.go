package services

import (
	"context"
	"sync"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const (
	filesProtocolVersion  = 1
	maxWriteFullBodyBytes = 1536 * 1024 // 1.5 MiB WriteFile body cap (ADR-031)
)

// flagEpoch turns the four files flags into a counter that rises whenever their
// combination changes between two observations.
type flagEpoch struct {
	mu    sync.Mutex
	mask  uint8
	epoch uint64
}

func (e *flagEpoch) observe(mask uint8) uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.epoch == 0 || mask != e.mask {
		e.epoch++
		e.mask = mask
	}
	return e.epoch
}

func (o *FileOps) flagMask(ctx context.Context) uint8 {
	var mask uint8
	for i, on := range []bool{
		o.policy.Flags.Editing(ctx), o.policy.Flags.RangeRead(ctx),
		o.policy.Flags.DeltaWrite(ctx), o.policy.Flags.EditLarge(ctx),
	} {
		if on {
			mask |= 1 << i
		}
	}
	return mask
}

// servedOps lists what this server serves right now. Range and delta ops join
// with their handlers (Epics 1.6 and 2.1); WRITE_FULL follows files:editing.
func (o *FileOps) servedOps(ctx context.Context) []sessionv1.FileOp {
	ops := []sessionv1.FileOp{sessionv1.FileOp_FILE_OP_STAT, sessionv1.FileOp_FILE_OP_READ_ALL}
	if o.policy.Flags.Editing(ctx) {
		ops = append(ops, sessionv1.FileOp_FILE_OP_WRITE_FULL)
	}
	return ops
}

// Capabilities reports what a client may rely on. Both listeners serve the same
// unary wire today; per-listener differences arrive with PATCH (Task 2.1.5).
func (o *FileOps) Capabilities(ctx context.Context) *sessionv1.FileTransportCapabilities {
	return &sessionv1.FileTransportCapabilities{
		ProtocolVersion: filesProtocolVersion,
		Ops:             o.servedOps(ctx),
		Wires:           []sessionv1.FileWire{sessionv1.FileWire_FILE_WIRE_CONNECT_UNARY},
		Limits: &sessionv1.FileTransportLimits{
			MaxWriteFullBytes: maxWriteFullBodyBytes,
			HashCapBytes:      truncateSize,
		},
		CapsEpoch: o.epoch.observe(o.flagMask(ctx)),
	}
}

// Require refuses an op the server does not advertise (capability honesty).
func (o *FileOps) Require(ctx context.Context, op sessionv1.FileOp) error {
	if op == sessionv1.FileOp_FILE_OP_STAT || op == sessionv1.FileOp_FILE_OP_READ_ALL {
		return nil // always served; skip the flag reads on the hot path
	}
	for _, served := range o.servedOps(ctx) {
		if served == op {
			return nil
		}
	}
	return connect.NewError(connect.CodeFailedPrecondition, errOpNotServed(op))
}

type errOpNotServed sessionv1.FileOp

func (e errOpNotServed) Error() string {
	return "file operation not served: " + sessionv1.FileOp(e).String()
}
