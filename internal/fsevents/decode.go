package fsevents

import "unsafe"

// decodeEvents converts the FSEventStreamCallback C arrays (char **paths,
// const uint32 *flags, const uint64 *ids) into Events. Arguments are
// unsafe.Pointers because the caller owns the C memory only for the callback.
func decodeEvents(n int, paths, flags, ids unsafe.Pointer) []Event {
	if n <= 0 || paths == nil || flags == nil || ids == nil {
		return nil
	}
	ps := unsafe.Slice((**byte)(paths), n)
	fs := unsafe.Slice((*uint32)(flags), n)
	is := unsafe.Slice((*uint64)(ids), n)
	out := make([]Event, n)
	for i := range out {
		out[i] = Event{Path: goString(ps[i]), Flags: EventFlags(fs[i]), EventID: is[i]}
	}
	return out
}

// goString copies a NUL-terminated C string.
func goString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}
