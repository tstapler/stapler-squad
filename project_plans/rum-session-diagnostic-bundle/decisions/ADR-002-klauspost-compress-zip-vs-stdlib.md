# ADR-002: Use klauspost/compress/zip for ZIP Assembly, Not stdlib archive/zip

**Date**: 2026-10-16
**Status**: Accepted
**Feature**: RUM Session Diagnostic Bundle

---

## Context

The bundle assembler needs to create a ZIP archive in memory containing `manifest.json` and per-section files (scrollback, analytics, context history, artifacts). Two options are available:

1. **`stdlib archive/zip`** — part of the Go standard library; no import required.
2. **`github.com/klauspost/compress/zip`** — a drop-in replacement with SIMD-accelerated deflate; already a direct dependency in `go.mod` (`github.com/klauspost/compress v1.19.1`).

## Decision

Use **`github.com/klauspost/compress/zip`**, imported as `kzip "github.com/klauspost/compress/zip"` to match the existing `kgzip` alias convention in `server/middleware/gzip.go`.

## Rationale

`github.com/klauspost/compress v1.19.1` is already a **direct** dependency — confirmed in `go.mod`. The `zip` sub-package is part of the same module; using it adds zero new transitive dependencies and requires no `go get` or security vetting.

The performance benefit is meaningful for the scrollback section: `klauspost/compress/zip` uses SIMD-accelerated deflate (approximately 20–40% faster compression on x86-64 than `stdlib archive/zip`). For a 5-second assembly budget with potentially 500 KB of scrollback, faster compression reduces latency headroom consumed.

The API is a drop-in replacement (same `zip.NewWriter`, `w.CreateHeader`, `w.Close` surface). If a future refactor needs to switch back to stdlib, the change is a one-line import swap.

`stdlib archive/zip` was not chosen only because a faster, already-available alternative exists at no additional cost. There is no correctness or compatibility advantage to stdlib here.

## Consequences

- Import alias `kzip "github.com/klauspost/compress/zip"` in `server/services/session_bundle_service.go`.
- No new `go.mod` or `go.sum` entries required.
- `go build ./...` validates the import resolves correctly; no runtime behavior differences from stdlib.
