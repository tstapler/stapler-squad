# Stack Research: Frontend File Upload UX

## 1. base64+JSON vs. multipart/form-data

### Current state

`OmnibarCreationPanel` → `image_upload_handler.go` uses base64+JSON today.
`TerminalOutput.tsx` → `session_image_upload_handler.go` **already uses multipart/form-data** with `FormData` on the frontend and `r.ParseMultipartForm` on the backend. This pattern is proven and working.

### Tradeoffs

| Factor | base64+JSON | multipart/form-data |
|---|---|---|
| Wire size overhead | ~33% larger (every 3 bytes → 4 base64 chars) | Negligible (boundary lines only, ~100 bytes per part) |
| Browser memory | FileReader buffers entire file as a string in JS heap **before** sending; a 20 MB file → ~27 MB string in memory | `FormData` streams file bytes directly from the `File` object; no extra in-memory copy |
| Backend complexity | Simple `json.Decoder` | `r.ParseMultipartForm` — also simple in Go stdlib; already used in this codebase |
| MIME type trust | `contentType` field is caller-controlled; no sniffing | Server can sniff magic bytes from the stream; same trust issue on `Content-Type` part header |
| Multiple files | Requires sequential `fetch` per file or batched JSON array | One `FormData` can hold `N` files (multiple `formData.append("file", f)` calls) or N separate requests |
| Streaming | Not possible; entire file must be read before sending | `fetch` with `FormData` streams the file body; Go's `io.Copy` streams to disk |

### Recommendation: switch to multipart/form-data

The 33% overhead is significant at the 20 MB limit (20 MB file → 27 MB on the wire, and 27 MB string in the JS heap from `FileReader`). More importantly, the codebase **already has a working multipart upload path** (`session_image_upload_handler.go` + `TerminalOutput.tsx`). Switching `OmnibarCreationPanel` to the same pattern makes the code consistent and removes the memory spike from `fileToBase64`.

The pre-session path differs from the session-aware path in one way: it uses `$TMPDIR/stapler-paste/` with no session ID. That is easy to preserve — just omit the `session_id` field or route to the existing handler rewritten for multipart. Alternatively, expand `image_upload_handler.go` to accept `multipart/form-data` in place of JSON.

### Multiple-file gotchas with the current base64 approach

- **Memory spike**: for N files, `fileToBase64` runs sequentially but each creates a full base64 string in JS heap before the `fetch` call. Three 20 MB files → three sequential 27 MB strings (GC can reclaim after each fetch, but peak is one 27 MB allocation).
- **Sequential serialization**: the current loop uploads files one at a time. With multipart, files can be sent in a single request or in parallel `fetch` calls using `FormData` (one per file), reducing total round-trip time.
- **`FileReader.readAsDataURL` with binary files**: works for any MIME type (not just images), but the data-URL prefix stripping (`split(",")[1]`) will still work. The bigger issue is that non-image files can be large (archives, videos), making the memory problem worse.
- **Size limit mismatch**: `maxImageBytes = 20 MB` is applied via `http.MaxBytesReader` on the **encoded** base64 body. A 20 MB file encodes to ~27 MB, meaning the backend currently rejects files larger than ~15 MB despite the nominal 20 MB limit. Multipart avoids this discrepancy because the body size and file size are nearly identical.

---

## 2. React libraries for file list display (icons by type)

### Options surveyed

**`lucide-react` (already installed, v1.14.0)**
The project already imports `lucide-react` in `BottomNav.tsx`, `DrawerNav.tsx`, `PaneHeader.tsx`, `PaneTilingContainer.tsx`, and `SessionActionsOverflow.tsx`. Lucide v1.14.0 includes file-type icons suitable for all required categories:

| Category | Lucide icon |
|---|---|
| Image | `ImageIcon` |
| Code / text | `FileCode` |
| Document (PDF, doc) | `FileText` |
| Archive (zip, tar) | `FileArchive` |
| Generic / other | `File` |

This is sufficient for the 5 visual categories in FR-4 without adding any new dependencies.

**`react-dropzone`** (most popular: ~10M weekly downloads)
Handles drag-and-drop and file-input in one API, with TypeScript support. **Out of scope** per requirements (drag-and-drop is explicitly excluded). Would add ~8 KB gzipped for functionality we don't need.

**`filepond` / `react-filepond`**
Full-featured upload widget with progress bars, chunking, plugins. Overkill for this use case; adds ~40 KB gzipped; progress bars are out of scope.

**`uppy`**
Full upload suite; 100+ KB. Out of scope.

### Recommendation: DIY with lucide-react icons

The file list component is simple: map each `AttachedFile` to a row with an icon, filename, optional thumbnail, and a remove button. Given:
- `lucide-react` is already a dependency with appropriate icons
- No drag-and-drop required
- No upload progress required
- The vanilla-extract CSS architecture means a small `.css.ts` file covers all styling

A custom `FileList` component (< 80 lines of TSX + a `FileList.css.ts`) is preferable to adding any library. The icon selection logic is a simple `switch` on MIME type prefix:

```ts
function iconForMimeType(mimeType: string): LucideIcon {
  if (mimeType.startsWith("image/")) return ImageIcon;
  if (mimeType.startsWith("text/") || /\/(javascript|typescript|json|xml|yaml)/.test(mimeType)) return FileCode;
  if (mimeType === "application/pdf" || mimeType.includes("word") || mimeType.includes("document")) return FileText;
  if (mimeType.includes("zip") || mimeType.includes("tar") || mimeType.includes("gzip") || mimeType.includes("archive")) return FileArchive;
  return File;
}
```

This is ~10 lines, needs no dependency, and covers all categories from FR-4.

---

## 3. MIME-type-to-extension mapping in Go

### Options

**Go stdlib `mime.ExtensionsByType`**
Returns a `[]string` of extensions for a MIME type. Relies on `/etc/mime.types` (absent on Alpine/scratch containers) and the OS MIME database. On Linux it typically returns multiple extensions (e.g. `["jpeg", "jpg"]` for `image/jpeg`), and the result is unordered. On minimal containers the list may be empty. Already used in `file_service.go` for content-type detection (not extension derivation).

**Hardcoded map (current pattern in `image_upload_handler.go`)**
`extensionFor()` already uses a `switch` on content type. This is the correct pattern for a controlled allowlist. It is zero-dependency, deterministic, and immune to OS configuration.

**`gabriel-vasile/mimetype`** (github.com/gabriel-vasile/mimetype)
Magic-byte content detection library with a comprehensive extension table. ~300 KB compiled-in MIME database. Best suited when you need to *detect* MIME type from bytes (not map MIME→extension). For extension derivation from a known MIME type, it provides `mimetype.Detect(bytes).Extension()` — accurate but requires reading file bytes again. Not in `go.mod` and adds a non-trivial dependency for what is a simple mapping task.

**`mime.ExtensionsByType` + hardcoded fallback**
A hybrid: try `mime.ExtensionsByType` first (OS-aware, covers unusual types), fall back to a hardcoded map for common types. Used in `file_service.go`'s `videoMIMEOverrides` pattern. The `file_service.go` comment explicitly notes: "mime.TypeByExtension reads /etc/mime.types which may be absent on minimal Linux installs."

### Recommendation: hardcoded map with `.bin` fallback, no new library

For this feature, the mapping direction is MIME type → extension (for naming the saved file). The requirements are:
1. Derive a safe extension from the MIME type when known
2. Fall back to the original file extension (if the client provides a filename with extension)
3. Fall back to `.bin`

The correct implementation is:

```go
// extensionForMIME returns a safe file extension for the given MIME type.
// Falls back to ext (from original filename) then ".bin".
func extensionForMIME(contentType, originalExt string) string {
    // Strip parameters ("text/plain; charset=utf-8" → "text/plain")
    ct := strings.ToLower(strings.SplitN(strings.TrimSpace(contentType), ";", 2)[0])
    if ext, ok := mimeExtensions[ct]; ok {
        return ext
    }
    // Use original extension if present and safe
    if originalExt != "" && isSafeExtension(originalExt) {
        return originalExt
    }
    return ".bin"
}

var mimeExtensions = map[string]string{
    "image/png":               ".png",
    "image/jpeg":              ".jpg",
    "image/gif":               ".gif",
    "image/webp":              ".webp",
    "image/svg+xml":           ".svg",
    "text/plain":              ".txt",
    "text/html":               ".html",
    "text/css":                ".css",
    "text/javascript":         ".js",
    "application/json":        ".json",
    "application/pdf":         ".pdf",
    "application/zip":         ".zip",
    "application/gzip":        ".gz",
    "application/x-tar":       ".tar",
    "application/octet-stream": ".bin",
    // ... extend as needed
}
```

`gabriel-vasile/mimetype` is overkill here — it solves detection from bytes, not MIME→extension mapping. `mime.ExtensionsByType` is unreliable on minimal Linux installs (per existing `file_service.go` comment). A 30–40 entry hardcoded map covers >95% of real-world file types.

---

## 4. Size limit behaviour with the current base64 approach (multiple files)

### The 33% encoding penalty breaks the size limit

`http.MaxBytesReader(w, r.Body, maxImageBytes)` applies to the **raw HTTP body bytes**, which for base64+JSON are ~33% larger than the actual file. At `maxImageBytes = 20 MB`:

- A 15 MB file → ~20 MB base64 string → passes (barely)
- A 16 MB file → ~21.3 MB base64 string → rejected with `request body too large`

The effective file size limit is ~15 MB, not 20 MB. Multipart avoids this: 20 MB file → ~20 MB + ~200 bytes overhead.

### Sequential upload creates a reliability risk for many files

The current loop in `handleAttachFiles` uploads files sequentially and stops on first failure (`break`). With no count limit (FR-3), a user uploading 10 files has 10 sequential round trips. One network hiccup fails silently for the remaining files (the `break` exits the loop, and the already-successful uploads are retained). The error message is generic ("Upload failed") with no indication of which file failed.

With multipart, each file can still be a separate `fetch` call (for progress tracking and per-file error reporting), but the body is a proper binary stream rather than a base64 string, removing the memory and encoding overhead.

### `FileReader.readAsDataURL` blocks the JS event loop during encoding

For large binary files, `FileReader` is synchronous at the OS level (async API but blocking I/O under the hood in browsers). A 20 MB file takes ~50–200ms to encode to base64 on a mid-range device, during which the UI may stutter. `FormData` with a `File` reference has no encoding step — the browser streams bytes directly from the file handle to the network.

---

## Summary

| Question | Answer |
|---|---|
| Switch to multipart? | **Yes** — already used in the codebase (TerminalOutput), removes 33% wire overhead and JS heap spike, fixes the effective size limit bug |
| File list UI library? | **No new library** — use `lucide-react` (already installed) for icons + DIY component |
| MIME→extension in Go? | **Hardcoded map** with `.bin` fallback — no new dependency, immune to OS MIME database gaps |
| base64 gotchas | 33% overhead makes the effective limit ~15 MB not 20 MB; JS heap spike for large files; sequential loop with silent partial-failure |
