package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/pprof"
)

// maxClientSpansBytes bounds the browser-supplied span payload (the request is
// untrusted input that is copied into the in-memory ZIP).
const maxClientSpansBytes = 5 * 1024 * 1024

// serverProfiles are the process-wide Go runtime profiles included in every
// bundle. Heap and goroutine are cheap snapshots; CPU profiles need a sampling
// window and are deliberately excluded.
var serverProfiles = []string{"heap", "goroutine"}

// captureProfiles returns gzip-compressed pprof snapshots keyed by section
// name, plus runtime.MemStats as JSON. Profiles are process-wide, not
// per-session.
func captureProfiles() (map[string][]byte, error) {
	out := make(map[string][]byte, len(serverProfiles)+1)
	for _, name := range serverProfiles {
		p := pprof.Lookup(name)
		if p == nil {
			continue
		}
		var buf bytes.Buffer
		if err := p.WriteTo(&buf, 0); err != nil {
			return nil, fmt.Errorf("write %s profile: %w", name, err)
		}
		out["profile_"+name] = buf.Bytes()
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	stats, err := json.MarshalIndent(struct {
		Goroutines int              `json:"goroutines"`
		NumCPU     int              `json:"num_cpu"`
		MemStats   runtime.MemStats `json:"mem_stats"`
	}{runtime.NumGoroutine(), runtime.NumCPU(), ms}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal runtime stats: %w", err)
	}
	out["runtime_stats"] = stats
	return out, nil
}

// validateClientSpans reports why the browser span payload can't be bundled,
// or "" if it can.
func validateClientSpans(raw []byte) string {
	switch {
	case len(raw) == 0:
		return "no browser spans recorded for this session"
	case len(raw) > maxClientSpansBytes:
		return fmt.Sprintf("payload exceeds %d bytes", maxClientSpansBytes)
	case !json.Valid(raw):
		return "payload is not valid JSON"
	}
	return ""
}
