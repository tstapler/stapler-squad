# BUG-018: Gob Encoding Dominates Session Persistence Memory [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-04-24
**Impact**: Session persistence layer allocates 35MB (20% of heap) via gob deserialization. Grows with session count, degrading memory efficiency over time.

## Problem Description

The session persistence layer uses `encoding/gob` for serializing/deserializing session state. Heap profiling shows `encoding/gob.decString` is the second-largest allocator at 35MB (20% of 175MB heap), with `reflect.unsafe_NewArray` (used heavily by gob's reflection-based codec) consuming another 45MB (26%). Together, gob-related allocations account for nearly half the process heap.

Gob's reflection-driven approach allocates a new string/slice for every field it decodes and cannot reuse memory across calls. This is avoidable with a format that supports zero-copy or pooled deserialization (e.g., protobuf with pre-allocated structs, or JSON with a decoder that reuses buffers).

## Reproduction Steps

1. Run stapler-squad with `--profile`
2. Capture heap profile: `curl -s --output heap.prof http://localhost:6060/debug/pprof/heap`
3. Inspect: `go tool pprof -top heap.prof`
4. Expected: session persistence allocations are a small fraction of heap
5. Actual: `encoding/gob.decString` and `reflect.unsafe_NewArray` together are ~46% of heap

## Root Cause

`encoding/gob` uses reflection to encode/decode structs, allocating a new value for every string and slice field. There is no way to pre-allocate or reuse buffers with gob's API. The allocation pressure is proportional to the number of sessions loaded on startup and on each save/load cycle.

## Files Likely Affected

- `session/storage.go` — primary gob encode/decode site for session state
- `config/` — any additional gob-persisted config structs

## Fix Approach

Replace gob with protobuf (already a project dependency) or JSON for the on-disk session state format. Protobuf's generated code avoids most reflection and supports struct reuse. A migration step is needed to convert existing `sessions.json`/state files to the new format on first load.

Alternatively, introduce a `sync.Pool` of gob decoders reading from a `bytes.Buffer` to reduce per-call allocations while keeping gob as the format.

## Verification

After fix: heap profile shows `encoding/gob.*` and `reflect.unsafe_NewArray` (from gob paths) no longer in the top allocators. Total heap should drop by ~40-50MB under the same session load.

## Related Tasks

- BUG-019: compress/flate writers not pooled (related allocation pressure)
