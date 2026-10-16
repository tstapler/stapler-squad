package repostate

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/sys/cpu"
)

const (
	segShift = 12
	segRows  = 1 << segShift
	segMask  = segRows - 1

	sideShift      = 12
	sideChunkWords = 1 << sideShift
	sideMask       = sideChunkWords - 1

	// Ids stay below 1<<30 so the top bits of a parent word can carry the
	// side-arena flag and the noParent sentinel.
	maxRows               = 1 << 30
	defaultMaxRows        = 1 << 20
	noParent       uint32 = 0xFFFFFFFF
	sideFlag       uint32 = 0x80000000
)

// RowState is the published lifecycle state of one commit row (ADR-009 item 2).
type RowState uint32

// Row states. Pending is the zero value, so a fresh segment is all Pending.
const (
	Pending RowState = iota
	Filling
	Filled
	Truncated
)

func (s RowState) String() string {
	switch s {
	case Pending:
		return "Pending"
	case Filling:
		return "Filling"
	case Filled:
		return "Filled"
	case Truncated:
		return "Truncated"
	}
	return fmt.Sprintf("RowState(%d)", uint32(s))
}

const (
	stateBits = 2
	stampMask = StampOrd(1<<(32-stateBits) - 1)
)

// StampOrd is the ordinal of the shallow stamp a row was observed under (30 bits).
type StampOrd uint32

// packState encodes a 2-bit state and a 30-bit stamp ordinal into one word.
func packState(s RowState, ord StampOrd) uint32 { return uint32(ord)<<stateBits | uint32(s) }

func unpackState(w uint32) (RowState, StampOrd) {
	return RowState(w & (1<<stateBits - 1)), StampOrd(w >> stateBits)
}

// segment is a fixed block of rows. It holds no pointers (the GC skips it)
// and is never moved or resized once installed in the directory.
type segment struct {
	hash    [segRows]plumbing.Hash
	parents [segRows][2]uint32
	state   [segRows]atomic.Uint32
}

// sideChunk is a block of the octopus side arena: for a commit with n >= 3
// parents, one word n-1 followed by parents 1..n-1.
type sideChunk [sideChunkWords]uint32

const (
	segmentBytes   = int64(unsafe.Sizeof(segment{}))
	sideChunkBytes = int64(unsafe.Sizeof(sideChunk{}))
)

// padded isolates a hot atomic on its own cache lines (ADR-009 caveats).
type padded[T any] struct {
	_ cpu.CacheLinePad
	V T
	_ cpu.CacheLinePad
}
