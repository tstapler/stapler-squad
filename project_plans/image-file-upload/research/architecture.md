# Backend Architecture: Generalizing from Image-Only to Any-File Upload

## 1. Route: `/api/upload/image` vs `/api/upload/file`

**Recommendation: Rename to `/api/upload/file`, with a backward-compatible alias kept for one release.**

Rationale:
- The requirements doc (NFR-3) explicitly states there are no external consumers and prefers rename for clarity.
- The route name is referenced in exactly two places: `server/server.go` line 439 (registration) and `OmnibarCreationPanel.tsx` line 218 (fetch call). Both are trivially updated.
- The test helper in `image_upload_handler_test.go` uses the path string in `httptest.NewRequest` but tests the handler directly — the path string is cosmetic in unit tests and doesn't affect routing.
- **Migration plan**: Register both `/api/upload/image` (alias, calls same handler) and `/api/upload/file` (canonical) in `server.go` for a single release, then drop the alias. With sole-user context this is optional — a single-commit rename across both files is sufficient.

## 2. Extension Determination Strategy

Go's `mime.ExtensionsByType()` returns a slice of extensions in unspecified order. For well-known types it typically returns multiple options (e.g. `text/html` → `[".htm", ".html"]`; `image/jpeg` → `[".jfif", ".jpe", ".jpeg", ".jpg"]`). Relying on it directly produces non-deterministic output.

**Recommended strategy: layered lookup with deterministic fallbacks.**

```
1. Priority table (static map): common MIME types → preferred extension
   Covers all image types already handled plus common code/doc/archive types.
   This is O(1) and deterministic.

2. Fallback to mime.ExtensionsByType(): sort the returned slice and pick the
   lexicographically last entry. Sorting makes the choice deterministic;
   "last" tends to prefer longer/clearer extensions (.jpeg over .jfif,
   .html over .htm) — acceptable heuristic for temp files.

3. If mime.ExtensionsByType() returns empty: derive from original filename
   extension (sanitized: strip path separators, allow only [a-zA-Z0-9]).
   Never trust the raw client-supplied extension — only use the sanitized form.

4. Final fallback: ".bin"
```

The static priority table should at minimum include:
- `image/png` → `.png`, `image/jpeg` → `.jpg`, `image/gif` → `.gif`, `image/webp` → `.webp`
- `text/plain` → `.txt`, `text/html` → `.html`, `text/css` → `.css`
- `application/json` → `.json`, `application/pdf` → `.pdf`
- `application/zip` → `.zip`, `application/gzip` → `.gz`
- `text/x-python`, `application/x-python-code` → `.py`
- `application/javascript`, `text/javascript` → `.js`

Retaining the existing static switch for image types is correct — it should be promoted to the priority table and extended, not replaced by `mime.ExtensionsByType()` alone.

**Security note**: the extension produced by any path must be sanitized to contain only `[a-zA-Z0-9.]` and not start with a path separator, before being passed to `os.CreateTemp`. The `os.CreateTemp` pattern `paste-*<ext>` already prevents directory traversal as long as `ext` is clean.

## 3. Handling `application/octet-stream` and Unknown MIME Types

`application/octet-stream` is the browser's generic binary fallback — it signals that the browser does not know the type, not that the file has no type. Treating it identically to a truly unknown type is correct.

**Recommendation: do not reject — fall through to the filename-extension fallback.**

Decision flow for `application/octet-stream` (or any unrecognized MIME type):
1. `mime.ExtensionsByType()` returns empty for `application/octet-stream` on most systems.
2. Use the sanitized original filename extension (step 3 from above). This is safe because:
   - The extension is sanitized (path separators stripped, only `[a-zA-Z0-9]` allowed).
   - The file is written with `0o600` permissions and a kernel-unique name via `os.CreateTemp`.
   - The file is referenced only by path in the agent prompt; the extension affects only human readability.
3. If the original filename has no extension or a suspicious one, fall back to `.bin`.

**Never** return HTTP 400 for an unknown MIME type under the new generalized handler. The old `extensionFor()` function returns `""` and the caller rejects — this gate must be removed.

The request struct should also accept an optional `originalFilename` field (see section 5) to enable the filename-extension fallback without requiring the client to re-parse it from the `File` object on every call.

## 4. `cleanOldPasteFiles()` and Non-Image Extensions

The current `cleanOldPasteFiles` function (lines 37–56 of `image_upload_handler.go`) evicts all non-directory files in the paste directory older than `maxPasteFileAge`. It does **not** filter by extension — it uses `os.ReadDir` and checks `entry.IsDir()` plus `info.ModTime()`.

**No changes are needed to `cleanOldPasteFiles()` for non-image file support.**

The function is already extension-agnostic. Any file written to `$TMPDIR/stapler-paste/` will be cleaned up after 24 hours regardless of extension. This is the correct behavior — all uploaded files (images, PDFs, code files, archives) should be treated identically for eviction purposes.

The test in `image_upload_handler_test.go` (`TestCleanOldPasteFiles`) uses `.png` filenames only as cosmetic test data — the function being tested has no extension awareness. The test does not need to be changed either, though adding a non-image extension case would improve coverage signal.

## 5. Original Filename: Store or Discard?

**Recommendation: accept `originalFilename` in the request, use it only for extension fallback, do not store it in the response or on disk.**

Arguments for receiving the original filename:
- Enables reliable extension fallback for `application/octet-stream` without brittle client-side logic.
- The client already has `file.name` available when constructing the request body.
- The agent prompt already references files by absolute server path, not by display name — no semantic change is needed.

Arguments against storing it in the response or in the filename on disk:
- **Security**: embedding user-supplied filenames in disk paths creates path traversal and filename injection risk. `os.CreateTemp` with a fixed prefix + kernel-unique random suffix avoids this entirely.
- **Simplicity**: the agent receives the server-assigned path (`paste-<random>.ext`). Preserving the original name adds no value to the agent.
- **Privacy**: temp files are `0o600`; using a random name avoids leaking the original name in `ls` output or logs if the paste dir is accidentally inspected.

**Proposed request struct change:**

```go
type fileUploadRequest struct {
    Data             string `json:"data"`             // base64-encoded bytes
    ContentType      string `json:"contentType"`      // browser-reported MIME type
    OriginalFilename string `json:"originalFilename"` // optional; used only for ext fallback
}
```

The `OriginalFilename` field is sanitized server-side: only the file extension is extracted (`filepath.Ext`), then validated against `[a-zA-Z0-9]` only. The name portion is discarded.

The response struct stays unchanged: `{ "path": "<absolute server path>" }`.

## Summary of Required Handler Changes

| Change | Details |
|---|---|
| Rename handler struct | `ImageUploadHandler` → `FileUploadHandler` (or keep old name, add alias) |
| Remove MIME rejection gate | Delete `if ext == "" { return 400 }` |
| Generalize `extensionFor()` | Priority table + `mime.ExtensionsByType()` sort + filename fallback + `.bin` |
| Accept `originalFilename` | Add optional field to request struct for ext fallback |
| Route rename | `/api/upload/image` → `/api/upload/file` in `server.go` + frontend fetch |
| Log message update | `[ImageUpload]` → `[FileUpload]` in log calls |
| `cleanOldPasteFiles` | No changes needed |
| `maxImageBytes` constant | Rename to `maxUploadBytes` (cosmetic, keeps same 20 MB value) |
