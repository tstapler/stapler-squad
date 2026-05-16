# Implementation Plan: Multi-File Upload (Any File Type)

**Feature**: Generalize image-only upload to accept any file type  
**Branch**: `stapler-squad-image-file`  
**Requirements**: `../requirements.md`  
**Research**: `../research/`  
**Date**: 2026-05-16

---

## Overview

Three epics in dependency order:

1. **Epic 1 — Backend: Generalize Upload Handler** — rename handler, expand MIME map, add
   `originalFilename` fallback, fix `MaxBytesReader` base64 overhead bug, add
   `sanitizeExtension()`, update route in `server.go`, update tests.
2. **Epic 2 — Frontend: Multi-File Upload Logic** — rename type, remove 3-file cap, add
   client-side size guard, add deduplication, conditionally create preview URLs, pass
   `originalFilename`, update fetch URL.
3. **Epic 3 — Frontend: File Chip List UI** — replace thumbnail grid with horizontal chip
   row, file-type icon mapping, vanilla-extract styles, accessibility.

Epic 2 depends on Epic 1 (the `/api/upload/file` endpoint must exist before the frontend
calls it). Epic 3 depends on Epic 2 (the `AttachedFile` type must exist before the UI
component uses it). Work within each epic can proceed in story order; tasks within a story
are sequential.

---

## Epic 1: Backend — Generalize Upload Handler

**Goal**: Accept any MIME type, derive a safe extension, fix the 20 MB limit bug, rename
all image-specific identifiers to file-generic ones, register the new route.

**Files touched**:
- `server/services/image_upload_handler.go` (primary)
- `server/services/image_upload_handler_test.go` (tests)
- `server/server.go` (route registration)

---

### Story 1.1: Rename Handler and Constants

**Why**: All identifiers carry "image" branding. Renaming before adding new behavior keeps
each commit's diff focused. The old handler struct and route are unused externally (sole
user, no external consumers per NFR-3), so a direct rename with no alias is sufficient.

#### Task 1.1.1 — Rename struct, constructor, constants, log tags in `image_upload_handler.go`

**File**: `server/services/image_upload_handler.go`

Changes:
- Rename constant `maxImageBytes` → `maxUploadBytes` (value unchanged: `20 * 1024 * 1024`).
- Rename constant `imageFileMode` → `uploadFileMode` (value unchanged: `0o600`).
- Rename type `ImageUploadHandler` → `FileUploadHandler`.
- Rename constructor `NewImageUploadHandler` → `NewFileUploadHandler`.
- Rename request struct `imageUploadRequest` → `fileUploadRequest`.
- Rename response struct `imageUploadResponse` → `fileUploadResponse`.
- Replace all `[ImageUpload]` log tag strings with `[FileUpload]`.

**Specific lines to change** (based on current file):
- Line 16: `maxImageBytes` → `maxUploadBytes`
- Line 17: `imageFileMode` → `uploadFileMode`
- Line 24–33: struct declaration and constructor body
- Lines 58–65: request/response struct declarations
- Lines 73, 104, 114, 119, 123: `maxImageBytes` / `imageFileMode` references and log tags

**Expected test assertions**: No behavior change — existing tests pass after renaming
`newImageUploadHandler` to `newFileUploadHandler` and updating all references in the test
file (see Task 1.1.2).

---

#### Task 1.1.2 — Update `image_upload_handler_test.go` to use renamed identifiers

**File**: `server/services/image_upload_handler_test.go`

Changes:
- Rename `newImageUploadHandler` helper → `newFileUploadHandler`.
- Update all calls: `newImageUploadHandler(t)` → `newFileUploadHandler(t)`.
- Change return type annotation in `newFileUploadHandler` to `*FileUploadHandler`.
- Update `postJSON` helper signature parameter type from `*ImageUploadHandler` to
  `*FileUploadHandler`.
- Replace `imageFileMode` → `uploadFileMode` in `TestHandleUpload_ValidPNG` (line 59).

**Expected test assertions**: `go test ./server/services/...` passes with no failures.

---

#### Task 1.1.3 — Update route registration in `server/server.go`

**File**: `server/server.go`

Changes (lines 438–440 based on grep output):
- Line 438: `services.NewImageUploadHandler(pasteDir)` → `services.NewFileUploadHandler(pasteDir)`.
- Rename local variable `imageHandler` → `fileHandler` (if declared separately).
- Line 439: `"/api/upload/image"` → `"/api/upload/file"`.
- Line 440: Update the log message string: `"Registered image upload handler at /api/upload/image"` →
  `"Registered file upload handler at /api/upload/file"`.

**Expected test assertions**: `go build ./...` succeeds. Integration: a POST to
`/api/upload/file` reaches `FileUploadHandler.HandleUpload`.

---

### Story 1.2: Expand MIME-to-Extension Map

**Why**: The current `extensionFor()` switch handles only 4 image MIME types and returns
`""` for everything else, causing a 400 rejection. The generalized handler needs to derive
a safe extension for 40+ MIME types.

#### Task 1.2.1 — Replace `extensionFor()` with `extensionForMIME()` priority table

**File**: `server/services/image_upload_handler.go`

Replace the `extensionFor()` function (lines 129–144) with `extensionForMIME()` using a
package-level `mimeExtensions` map:

```go
// mimeExtensions is the priority table for MIME type → file extension.
// Keys must be lowercase with no parameters. Values include the leading dot.
// Covers the most common types encountered in developer workflows (40+ entries).
var mimeExtensions = map[string]string{
    // Images
    "image/png":                    ".png",
    "image/jpeg":                   ".jpg",
    "image/jpg":                    ".jpg",
    "image/gif":                    ".gif",
    "image/webp":                   ".webp",
    "image/svg+xml":                ".svg",
    "image/bmp":                    ".bmp",
    "image/tiff":                   ".tiff",
    "image/ico":                    ".ico",
    "image/x-icon":                 ".ico",
    // Text / code
    "text/plain":                   ".txt",
    "text/html":                    ".html",
    "text/css":                     ".css",
    "text/javascript":              ".js",
    "text/typescript":              ".ts",
    "text/x-python":                ".py",
    "text/x-go":                    ".go",
    "text/x-rust":                  ".rs",
    "text/x-c":                     ".c",
    "text/x-c++":                   ".cpp",
    "text/x-java":                  ".java",
    "text/x-ruby":                  ".rb",
    "text/x-sh":                    ".sh",
    "text/xml":                     ".xml",
    "text/csv":                     ".csv",
    "text/markdown":                ".md",
    // Application / code
    "application/json":             ".json",
    "application/javascript":       ".js",
    "application/typescript":       ".ts",
    "application/xml":              ".xml",
    "application/yaml":             ".yaml",
    "application/x-yaml":          ".yaml",
    "application/toml":             ".toml",
    "application/pdf":              ".pdf",
    "application/msword":           ".doc",
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
    "application/vnd.ms-excel":     ".xls",
    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       ".xlsx",
    // Archives
    "application/zip":              ".zip",
    "application/gzip":             ".gz",
    "application/x-gzip":          ".gz",
    "application/x-tar":           ".tar",
    "application/x-bzip2":         ".bz2",
    "application/x-7z-compressed": ".7z",
    "application/x-rar-compressed": ".rar",
    // Generic
    "application/octet-stream":    ".bin",
}

// extensionForMIME returns a safe file extension for the given MIME type.
// Priority: (1) mimeExtensions table, (2) sanitized original filename extension,
// (3) ".bin" fallback.
//
// contentType may contain parameters ("text/plain; charset=utf-8") — they are stripped.
// originalExt is the extension from the client-supplied filename (e.g. ".go"), already
// sanitized by sanitizeExtension before being passed here.
func extensionForMIME(contentType, originalExt string) string {
    ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
    if ext, ok := mimeExtensions[ct]; ok {
        return ext
    }
    if originalExt != "" {
        return originalExt
    }
    return ".bin"
}
```

Remove the old `extensionFor()` function entirely.

**Expected test assertions** (Task 1.2.2 adds these):
- `extensionForMIME("image/png", "")` → `".png"`
- `extensionForMIME("application/json", "")` → `".json"`
- `extensionForMIME("application/zip", "")` → `".zip"`
- `extensionForMIME("text/x-python", "")` → `".py"`
- `extensionForMIME("application/octet-stream", ".go")` → `".bin"` (table wins)
- `extensionForMIME("application/octet-stream", "")` → `".bin"`
- `extensionForMIME("UNKNOWN/type", ".rs")` → `".rs"` (originalExt fallback)
- `extensionForMIME("UNKNOWN/type", "")` → `".bin"` (final fallback)
- `extensionForMIME("image/png; charset=utf-8", "")` → `".png"` (parameter stripping)
- `extensionForMIME("IMAGE/PNG", "")` → `".png"` (case insensitive)

---

#### Task 1.2.2 — Update tests for extension function

**File**: `server/services/image_upload_handler_test.go`

- Replace `TestHandleUpload_ContentTypeExtensions` (currently calls `extensionFor()`):
  - Rename test to `TestExtensionForMIME`.
  - Update all `extensionFor(tc.ct)` calls to `extensionForMIME(tc.ct, "")`.
  - Add new test cases (from assertions above): JSON, ZIP, Python, unknown MIME with
    fallback, content-type with parameters, uppercase MIME.
  - Remove the `unknown_bmp` case that expected `""` — `"image/bmp"` now returns `".bmp"`.
  - Add a case for `"image/bmp"` expecting `".bmp"`.

```go
func TestExtensionForMIME(t *testing.T) {
    cases := []struct {
        name        string
        ct          string
        originalExt string
        want        string
    }{
        {"png", "image/png", "", ".png"},
        {"jpeg", "image/jpeg", "", ".jpg"},
        {"gif", "image/gif", "", ".gif"},
        {"webp", "image/webp", "", ".webp"},
        {"bmp", "image/bmp", "", ".bmp"},
        {"json", "application/json", "", ".json"},
        {"zip", "application/zip", "", ".zip"},
        {"python", "text/x-python", "", ".py"},
        {"octet_stream_no_fallback", "application/octet-stream", "", ".bin"},
        {"octet_stream_table_wins", "application/octet-stream", ".go", ".bin"},
        {"unknown_with_fallback", "application/x-unknown", ".rs", ".rs"},
        {"unknown_no_fallback", "application/x-unknown", "", ".bin"},
        {"params_stripped", "image/png; charset=utf-8", "", ".png"},
        {"uppercase", "IMAGE/PNG", "", ".png"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got := extensionForMIME(tc.ct, tc.originalExt)
            if got != tc.want {
                t.Errorf("extensionForMIME(%q, %q) = %q, want %q", tc.ct, tc.originalExt, got, tc.want)
            }
        })
    }
}
```

---

### Story 1.3: Add `sanitizeExtension()` Helper

**Why**: The original filename extension from the client can contain path separators or
non-alphanumeric characters (`../../etc/passwd`). `os.CreateTemp` does not sanitize its
pattern argument, so a crafted extension can escape the paste directory (see pitfalls
research §2). The session handler has `sanitizeFilename()` for this; the pre-session
handler needs a targeted extension-only variant.

#### Task 1.3.1 — Implement `sanitizeExtension()` in `image_upload_handler.go`

**File**: `server/services/image_upload_handler.go`

Add after the `mimeExtensions` var block:

```go
// sanitizeExtension returns a safe file extension from a client-supplied filename.
// It extracts the extension with filepath.Ext, then strips all characters that are
// not ASCII alphanumeric or '.'. If the result is empty or longer than 10 characters,
// it returns "". The returned string includes the leading dot (e.g. ".go").
//
// This prevents path traversal via crafted extensions such as "../../etc/passwd".
func sanitizeExtension(filename string) string {
    ext := filepath.Ext(filepath.Base(filename)) // strips directory components first
    if ext == "" {
        return ""
    }
    // Allow only [a-zA-Z0-9.] — strip everything else
    var b strings.Builder
    for _, r := range ext {
        if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' {
            b.WriteRune(r)
        }
    }
    safe := strings.ToLower(b.String())
    if len(safe) > 10 || safe == "." {
        return ""
    }
    return safe
}
```

**Expected test assertions** (Task 1.3.2):
- `sanitizeExtension("report.pdf")` → `".pdf"`
- `sanitizeExtension("archive.tar.gz")` → `".gz"`
- `sanitizeExtension("../../etc/passwd")` → `".passwd"` (base strips path, only ext extracted)
- `sanitizeExtension("file")` → `""`
- `sanitizeExtension("file.")` → `""`
- `sanitizeExtension("file.UPPERCASE")` → `".uppercase"`
- `sanitizeExtension("file.very-long-ext-here")` → `""` (>10 chars → blocked)
- `sanitizeExtension("")` → `""`

---

#### Task 1.3.2 — Tests for `sanitizeExtension()`

**File**: `server/services/image_upload_handler_test.go`

Add new test function:

```go
func TestSanitizeExtension(t *testing.T) {
    cases := []struct {
        filename string
        want     string
    }{
        {"report.pdf", ".pdf"},
        {"archive.tar.gz", ".gz"},
        {"../../etc/passwd", ".passwd"},
        {"file", ""},
        {"file.", ""},
        {"file.UPPERCASE", ".uppercase"},
        {"file.very-long-ext-here", ""},
        {"", ""},
        {"has spaces .txt", ".txt"},
        {"../../../.bashrc", ".bashrc"},
    }
    for _, tc := range cases {
        t.Run(tc.filename, func(t *testing.T) {
            got := sanitizeExtension(tc.filename)
            if got != tc.want {
                t.Errorf("sanitizeExtension(%q) = %q, want %q", tc.filename, got, tc.want)
            }
        })
    }
}
```

---

### Story 1.4: Add `originalFilename` Field and Fix `MaxBytesReader` Bug

**Why**: Two correctness fixes: (1) the request struct needs `OriginalFilename` so the
server can derive an extension for `application/octet-stream` files; (2) `MaxBytesReader`
is set to 20 MB but the body is base64-encoded (33% larger), meaning the effective limit
is ~15 MB, not 20 MB (pitfalls §3a).

#### Task 1.4.1 — Add `OriginalFilename` to request struct and wire into handler

**File**: `server/services/image_upload_handler.go`

Changes:
- Add field to `fileUploadRequest`:
  ```go
  type fileUploadRequest struct {
      Data             string `json:"data"`             // base64-encoded file bytes
      ContentType      string `json:"contentType"`      // browser-reported MIME type
      OriginalFilename string `json:"originalFilename"` // optional; used only for ext fallback
  }
  ```
- In `HandleUpload`, after decoding the request, derive the extension:
  ```go
  safeOrigExt := sanitizeExtension(req.OriginalFilename)
  ext := extensionForMIME(req.ContentType, safeOrigExt)
  ```
- Remove the old `ext == ""` rejection gate:
  ```go
  // DELETE this block entirely:
  // if ext == "" {
  //     http.Error(w, "unsupported content type", http.StatusBadRequest)
  //     return
  // }
  ```
- Update the empty-data guard message from "image data is empty" to "file data is empty".
- Update the `os.CreateTemp` error message from "failed to save image" to "failed to save file".
- Update the write error log/message similarly.

**Expected test assertions**: A POST with `contentType: "application/pdf"` returns 200
with a path ending in `.pdf`. A POST with `contentType: "application/x-unknown"` and
`originalFilename: "script.py"` returns 200 with a path ending in `.py`. A POST with
an unknown MIME and no `originalFilename` returns 200 with a path ending in `.bin`.

---

#### Task 1.4.2 — Fix `MaxBytesReader` limit for base64 overhead

**File**: `server/services/image_upload_handler.go`

The 20 MB file limit applies to the decoded file bytes, but the HTTP body contains the
base64-encoded representation (~33% larger). Fix the reader limit:

```go
// Before (line 73):
r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

// After:
// Base64 encoding inflates by 4/3; add 4096 bytes for JSON overhead.
r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes*4/3+4096)
```

`maxUploadBytes` remains `20 * 1024 * 1024`. The reader limit becomes ~26.7 MB, correctly
allowing a 20 MB file through.

**Expected test assertions** (Task 1.4.3): A file of exactly 20 MB returns 200. A file
slightly over 20 MB (decoded bytes) returns 413.

---

#### Task 1.4.3 — Tests for `originalFilename` and size limit

**File**: `server/services/image_upload_handler_test.go`

Add tests:

```go
// TestHandleUpload_NonImageMIMEAccepted verifies that non-image MIME types are accepted.
func TestHandleUpload_NonImageMIMEAccepted(t *testing.T) {
    h, _ := newFileUploadHandler(t)
    cases := []struct {
        ct      string
        wantExt string
    }{
        {"application/json", ".json"},
        {"application/pdf", ".pdf"},
        {"application/zip", ".zip"},
        {"text/x-python", ".py"},
        {"application/octet-stream", ".bin"},
    }
    for _, tc := range cases {
        t.Run(tc.ct, func(t *testing.T) {
            rr := postJSON(t, h, map[string]string{
                "data":        base64.StdEncoding.EncodeToString([]byte("fake")),
                "contentType": tc.ct,
            })
            if rr.Code != http.StatusOK {
                t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
            }
            var resp fileUploadResponse
            _ = json.NewDecoder(rr.Body).Decode(&resp)
            if !strings.HasSuffix(resp.Path, tc.wantExt) {
                t.Errorf("expected path ending in %q, got %q", tc.wantExt, resp.Path)
            }
        })
    }
}

// TestHandleUpload_OriginalFilenameExtFallback verifies extension fallback via filename.
func TestHandleUpload_OriginalFilenameExtFallback(t *testing.T) {
    h, _ := newFileUploadHandler(t)
    rr := postJSON(t, h, map[string]string{
        "data":             base64.StdEncoding.EncodeToString([]byte("fake")),
        "contentType":      "application/x-unknown-type",
        "originalFilename": "my_script.rs",
    })
    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
    }
    var resp fileUploadResponse
    _ = json.NewDecoder(rr.Body).Decode(&resp)
    if !strings.HasSuffix(resp.Path, ".rs") {
        t.Errorf("expected .rs extension, got %q", resp.Path)
    }
}

// TestHandleUpload_PathTraversalInFilename verifies that a crafted originalFilename
// cannot escape the paste directory.
func TestHandleUpload_PathTraversalInFilename(t *testing.T) {
    h, dir := newFileUploadHandler(t)
    rr := postJSON(t, h, map[string]string{
        "data":             base64.StdEncoding.EncodeToString([]byte("fake")),
        "contentType":      "application/x-unknown-type",
        "originalFilename": "../../etc/passwd",
    })
    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
    }
    var resp fileUploadResponse
    _ = json.NewDecoder(rr.Body).Decode(&resp)
    // Path must be inside the paste dir
    if !strings.HasPrefix(resp.Path, dir) {
        t.Errorf("path escaped paste dir: %q (expected prefix %q)", resp.Path, dir)
    }
}
```

Update `postJSON` helper to accept `*FileUploadHandler` (already done in Task 1.1.2).

Update `TestHandleUpload_ValidPNG` to use the updated struct name `fileUploadResponse`.

---

## Epic 2: Frontend — Multi-File Upload Logic

**Goal**: Rename `AttachedImage` → `AttachedFile`, remove the 3-file cap, add client-side
size guard, deduplicate on `name|size|lastModified`, create preview URLs only for images,
pass `originalFilename` in the request body, update the fetch URL to `/api/upload/file`.

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (primary)

---

### Story 2.1: Rename `AttachedImage` → `AttachedFile` and Make `previewUrl` Optional

**Why**: NFR-4 requires the TypeScript type rename. Making `previewUrl` optional is
necessary for non-image files to avoid creating blob URLs for binary files (pitfalls §5).

#### Task 2.1.1 — Update the `AttachedFile` interface

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

Replace lines 123–127:
```ts
// Before:
interface AttachedImage {
  file: File;
  path: string;       // absolute server path returned from upload
  previewUrl: string; // object URL for thumbnail preview
}

// After:
interface AttachedFile {
  file: File;
  path: string;        // absolute server path returned from upload
  previewUrl?: string; // object URL; only set for image/* files
  name: string;        // original filename for display
  size: number;        // file size in bytes for display
}
```

---

#### Task 2.1.2 — Update all state variables and refs to use `AttachedFile`

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

- Line 181: `useState<AttachedImage[]>` → `useState<AttachedFile[]>`
- Line 184: `useRef<AttachedImage[]>` → `useRef<AttachedFile[]>`
- Line 198: `attachedImagesRef.current.forEach((img) => URL.revokeObjectURL(img.previewUrl))`
  → guard for optional:
  ```ts
  attachedImagesRef.current.forEach((f) => {
    if (f.previewUrl) URL.revokeObjectURL(f.previewUrl);
  });
  ```
- Rename state variable `attachedImages` → `attachedFiles` throughout the component
  (replace all 13 occurrences).
- Rename `attachedImagesRef` → `attachedFilesRef`.
- Rename callback `removeImage` → `removeFile`.

---

### Story 2.2: Remove 3-File Cap and Update `accept` Attribute

**Why**: FR-1 requires `accept="*/*"` (or no attribute) and FR-3 removes the hard cap.

#### Task 2.2.1 — Change `accept` attribute and remove cap logic

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

In the hidden file input (around line 579):
```tsx
// Before:
<input
  ref={attachInputRef}
  type="file"
  accept="image/*"
  ...
/>

// After:
<input
  ref={attachInputRef}
  type="file"
  accept="*/*"
  ...
/>
```

In `handleAttachFiles` (around line 206–208), remove the cap slice:
```ts
// Before:
const available = 3 - attachedImages.length;
const toUpload = files.slice(0, available);

// After:
const toUpload = files; // no cap; deduplication applied below (Story 2.3)
```

In the attach button (around line 588–596):
```tsx
// Before:
disabled={isAttaching || attachedImages.length >= 3}
aria-label="Attach image (up to 3)"

// After:
disabled={isAttaching}
aria-label="Attach files"
```

Remove the cap-exceeded message block (lines 597–599):
```tsx
// Delete:
{attachedImages.length >= 3 && (
  <span className={styles.attachLimit}>Max 3 images</span>
)}
```

Update the button label text: `"📎 Attach image"` → `"📎 Attach files"`.

---

### Story 2.3: Client-Side Size Guard and Deduplication

**Why**: Without a size guard, selecting a 100 MB file causes `FileReader.readAsDataURL`
to allocate ~133 MB in the JS heap before any network request (pitfalls §3b). Without
deduplication, selecting the same file twice causes two uploads and two paths injected into
the prompt (pitfalls §6).

#### Task 2.3.1 — Add file size check before FileReader

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

Inside `handleAttachFiles`, before the upload loop, add a pre-filter:

```ts
const MAX_FILE_BYTES = 20 * 1024 * 1024; // 20 MB — must match backend maxUploadBytes

// Size check — reject oversized files before FileReader allocation
const oversized = toUpload.filter(f => f.size > MAX_FILE_BYTES);
if (oversized.length > 0) {
  setAttachError(`${oversized.map(f => f.name).join(", ")}: exceeds 20 MB limit`);
  // Continue with remaining files (don't abort the whole batch)
}
const sizedOk = toUpload.filter(f => f.size <= MAX_FILE_BYTES);
```

Use `sizedOk` as the iteration source instead of `toUpload`.

---

#### Task 2.3.2 — Add deduplication by `name|size|lastModified`

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

After the size check, before the upload loop:

```ts
// Deduplication — skip files already attached (by name+size+lastModified)
const existingKeys = new Set(
  attachedFiles.map(f => `${f.file.name}|${f.file.size}|${f.file.lastModified}`)
);
const deduplicated = sizedOk.filter(
  f => !existingKeys.has(`${f.name}|${f.size}|${f.lastModified}`)
);
```

Use `deduplicated` as the upload source.

---

### Story 2.4: Conditional Preview URL and Updated Fetch Call

**Why**: Creating blob URLs for non-image files wastes browser memory (pitfalls §5).
The fetch URL must point to the new `/api/upload/file` endpoint. `originalFilename` must
be sent for extension fallback on `application/octet-stream` files.

#### Task 2.4.1 — Conditional `previewUrl` creation

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

In `handleAttachFiles`, replace the current unconditional blob URL creation:
```ts
// Before (line 215):
const previewUrl = URL.createObjectURL(file);

// After:
const isImage = file.type.startsWith("image/");
const previewUrl = isImage ? URL.createObjectURL(file) : undefined;
```

When pushing to `results`, populate all `AttachedFile` fields:
```ts
results.push({
  file,
  path: data.path,
  previewUrl,      // undefined for non-images
  name: file.name,
  size: file.size,
});
```

---

#### Task 2.4.2 — Update fetch URL and request body

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

In `handleAttachFiles`, update the fetch call:
```ts
// Before (line 218):
const resp = await fetch(`${uploadBaseUrl}/upload/image`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ data: base64, contentType: file.type }),
});

// After:
const resp = await fetch(`${uploadBaseUrl}/upload/file`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    data: base64,
    contentType: file.type,
    originalFilename: file.name,  // for extension fallback on octet-stream
  }),
});
```

---

#### Task 2.4.3 — Update `removeFile` and prop callback

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

In `removeFile` (renamed from `removeImage`), guard the URL revocation:
```ts
const removeFile = useCallback((index: number) => {
  setAttachedFiles((prev) => {
    const f = prev[index];
    if (f.previewUrl) URL.revokeObjectURL(f.previewUrl);
    return prev.filter((_, i) => i !== index);
  });
}, []);
```

Update the `onAttachedImagesChange` call in the sync `useEffect` (line 192):
```ts
// Before:
onAttachedImagesChange?.(attachedImages.map((img) => img.path));

// After:
onAttachedImagesChange?.(attachedFiles.map((f) => f.path));
```

---

## Epic 3: Frontend — File Chip List UI

**Goal**: Replace the image thumbnail grid with a horizontal chip row. Each chip shows a
file-type icon (from `lucide-react`), truncated filename, and an × remove button. Images
show an inline thumbnail instead of the generic icon. Styles live in a new
`FileChipList.css.ts` (vanilla-extract). Accessibility: `aria-label="Remove <filename>"`
on each × button.

**Files touched**:
- `web-app/src/components/sessions/FileChipList.tsx` (new component)
- `web-app/src/components/sessions/FileChipList.css.ts` (new vanilla-extract styles)
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (import + replace thumbnail grid)

---

### Story 3.1: Create `FileChipList` Component

**Why**: Separating the chip list into its own component keeps `OmnibarCreationPanel.tsx`
focused on form logic and makes the chip UI independently testable.

#### Task 3.1.1 — Create `FileChipList.css.ts` (vanilla-extract styles)

**File**: `web-app/src/components/sessions/FileChipList.css.ts` (new file)

Per the project CSS architecture (`.claude/rules/css-architecture.md`), new component
styles must use vanilla-extract `.css.ts` files with `vars` from the theme contract.

```ts
import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const chipList = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space[2],
  marginTop: vars.space[2],
  maxHeight: "120px",
  overflowY: "auto",
});

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space[1],
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.md,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderDefault}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  maxWidth: "200px",
  overflow: "hidden",
});

export const chipIcon = style({
  flexShrink: 0,
  width: "16px",
  height: "16px",
  objectFit: "cover",
  borderRadius: "2px",
});

export const chipThumbnail = style({
  flexShrink: 0,
  width: "20px",
  height: "20px",
  objectFit: "cover",
  borderRadius: "2px",
});

export const chipName = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  minWidth: 0,
});

export const chipRemove = style({
  flexShrink: 0,
  marginLeft: vars.space[1],
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: "0",
  lineHeight: 1,
  color: vars.color.textSecondary,
  ":hover": {
    color: vars.color.statusDanger,
  },
});
```

If `vars` paths differ from the above (inspect `web-app/src/styles/theme.css.ts`), adjust
token names accordingly. The implementer must cross-reference actual token names before
writing the file.

---

#### Task 3.1.2 — Create `FileChipList.tsx` component

**File**: `web-app/src/components/sessions/FileChipList.tsx` (new file)

```tsx
// +feature: file-chip-list
import React from "react";
import {
  File,
  FileCode,
  FileText,
  FileArchive,
  ImageIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  chipList,
  chip,
  chipIcon,
  chipThumbnail,
  chipName,
  chipRemove,
} from "./FileChipList.css";

export interface AttachedFile {
  file: File;
  path: string;
  previewUrl?: string; // only for image/* files
  name: string;
  size: number;
}

function iconForMimeType(mimeType: string): LucideIcon {
  if (mimeType.startsWith("image/")) return ImageIcon;
  if (
    mimeType.startsWith("text/") ||
    /\/(javascript|typescript|json|xml|yaml|toml)/.test(mimeType) ||
    /x-(python|go|rust|c|c\+\+|java|ruby|sh)/.test(mimeType)
  ) {
    return FileCode;
  }
  if (
    mimeType === "application/pdf" ||
    mimeType.includes("word") ||
    mimeType.includes("document") ||
    mimeType.includes("spreadsheet") ||
    mimeType.includes("presentation")
  ) {
    return FileText;
  }
  if (
    mimeType.includes("zip") ||
    mimeType.includes("tar") ||
    mimeType.includes("gzip") ||
    mimeType.includes("bzip") ||
    mimeType.includes("7z") ||
    mimeType.includes("rar") ||
    mimeType.includes("archive")
  ) {
    return FileArchive;
  }
  return File;
}

interface FileChipListProps {
  files: AttachedFile[];
  onRemove: (index: number) => void;
}

export function FileChipList({ files, onRemove }: FileChipListProps) {
  if (files.length === 0) return null;

  return (
    <div className={chipList} role="list" aria-label="Attached files">
      {files.map((f, i) => {
        const Icon = iconForMimeType(f.file.type);
        const isImage = f.file.type.startsWith("image/") && f.previewUrl;

        return (
          <div key={f.path} className={chip} role="listitem" title={f.name}>
            {isImage ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={f.previewUrl}
                alt=""
                aria-hidden="true"
                className={chipThumbnail}
              />
            ) : (
              <Icon
                size={16}
                aria-hidden="true"
                className={chipIcon}
              />
            )}
            <span className={chipName}>{f.name}</span>
            <button
              type="button"
              className={chipRemove}
              onClick={() => onRemove(i)}
              aria-label={`Remove ${f.name}`}
            >
              ×
            </button>
          </div>
        );
      })}
    </div>
  );
}
```

**Notes**:
- The `AttachedFile` interface is defined here and re-exported. `OmnibarCreationPanel.tsx`
  should import it from this module rather than re-declaring it (or keep the local
  declaration and ensure both are identical — prefer import for single source of truth).
- `role="list"` + `role="listitem"` provides accessible structure.
- `title={f.name}` provides a hover tooltip for truncated filenames.
- `type="button"` on the remove button prevents accidental form submission.

---

### Story 3.2: Replace Thumbnail Grid in `OmnibarCreationPanel.tsx`

**Why**: The current thumbnail grid (lines 605–627) is image-only and hard-coded to the
old `AttachedImage` type. It must be replaced with the new `FileChipList` component.

#### Task 3.2.1 — Import `FileChipList` and replace thumbnail grid

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

Add import at the top of the file (with other component imports):
```ts
import { FileChipList } from "./FileChipList";
```

Replace the thumbnail preview block (lines 605–627):
```tsx
// Before:
{/* Thumbnail previews */}
{attachedImages.length > 0 && (
  <div className={styles.thumbnailRow}>
    {attachedImages.map((img, i) => (
      <div key={img.path} className={styles.thumbnail}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={img.previewUrl}
          alt={img.file.name}
          className={styles.thumbnailImg}
        />
        <button
          type="button"
          className={styles.thumbnailRemove}
          onClick={() => removeImage(i)}
          aria-label={`Remove ${img.file.name}`}
        >
          ×
        </button>
      </div>
    ))}
  </div>
)}

// After:
{/* File chip list */}
<FileChipList
  files={attachedFiles}
  onRemove={removeFile}
/>
```

The `FileChipList` component already returns `null` when `files.length === 0`, so no
conditional wrapper is needed.

---

#### Task 3.2.2 — Remove obsolete CSS class references

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx` and
`web-app/src/components/sessions/OmnibarCreationPanel.module.css` (or equivalent `.css.ts`).

After removing the thumbnail grid, the following CSS class names are no longer referenced
in the TSX and can be removed from the stylesheet:
- `styles.thumbnailRow`
- `styles.thumbnail`
- `styles.thumbnailImg`
- `styles.thumbnailRemove`
- `styles.attachLimit` (removed when 3-file cap was removed in Story 2.2)

Locate the stylesheet file (check the `import styles from` statement at the top of
`OmnibarCreationPanel.tsx`) and remove the corresponding CSS rules. The CI `lint:css` step
will fail on unused variables if this is a `.css.ts` file with strict analysis.

---

### Story 3.3: Accessibility and Feature Registry

#### Task 3.3.1 — Verify `aria-label` on remove buttons

**File**: `web-app/src/components/sessions/FileChipList.tsx`

Each × button already has `aria-label={`Remove ${f.name}`}` from Task 3.1.2. Verify in
browser DevTools that the accessible name is `"Remove report.pdf"` for a file named
`report.pdf`. No code change needed if Task 3.1.2 is implemented correctly — this is a
verification step.

---

#### Task 3.3.2 — Update feature registry

**Files**:
- `docs/registry/features/file-chip-list.json` (new per-feature file)
- `docs/registry/features/image-upload.json` (existing, if present; update `id` and `testIds`)

Per `.claude/rules/feature-registry.md`, every new UI feature needs a registry entry:

```json
{
  "id": "file-chip-list",
  "type": "frontend",
  "description": "Horizontal chip row for displaying attached files with type icons",
  "component": "FileChipList",
  "filePath": "web-app/src/components/sessions/FileChipList.tsx",
  "tested": false,
  "testIds": [],
  "lastModified": "2026-05-16T00:00:00Z"
}
```

Set `"tested": true` and populate `testIds` once Jest tests are added (Story 3.4).

---

### Story 3.4: Component Tests

**Why**: `FileChipList` is a new public component; it needs unit tests to verify icon
selection, thumbnail rendering, remove callback, and accessibility attributes.

#### Task 3.4.1 — Create `FileChipList.test.tsx`

**File**: `web-app/src/components/sessions/FileChipList.test.tsx` (new file)

```tsx
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { FileChipList, AttachedFile } from "./FileChipList";

function makeFile(name: string, type: string, size = 1024): File {
  return new File(["x".repeat(size)], name, { type });
}

function makeAttached(name: string, type: string, previewUrl?: string): AttachedFile {
  const file = makeFile(name, type);
  return {
    file,
    path: `/tmp/stapler-paste/paste-abc${name}`,
    previewUrl,
    name,
    size: file.size,
  };
}

describe("FileChipList", () => {
  it("renders nothing when files array is empty", () => {
    const { container } = render(<FileChipList files={[]} onRemove={jest.fn()} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders a chip for each file", () => {
    const files = [
      makeAttached("report.pdf", "application/pdf"),
      makeAttached("script.py", "text/x-python"),
    ];
    render(<FileChipList files={files} onRemove={jest.fn()} />);
    expect(screen.getByText("report.pdf")).toBeInTheDocument();
    expect(screen.getByText("script.py")).toBeInTheDocument();
  });

  it("shows thumbnail for image files with previewUrl", () => {
    const files = [makeAttached("photo.png", "image/png", "blob:fake-url")];
    render(<FileChipList files={files} onRemove={jest.fn()} />);
    const img = screen.getByRole("img", { hidden: true });
    expect(img).toHaveAttribute("src", "blob:fake-url");
  });

  it("does not show img element for non-image files", () => {
    const files = [makeAttached("archive.zip", "application/zip")];
    render(<FileChipList files={files} onRemove={jest.fn()} />);
    expect(screen.queryByRole("img", { hidden: true })).toBeNull();
  });

  it("calls onRemove with correct index when × is clicked", () => {
    const onRemove = jest.fn();
    const files = [
      makeAttached("a.txt", "text/plain"),
      makeAttached("b.json", "application/json"),
    ];
    render(<FileChipList files={files} onRemove={onRemove} />);
    fireEvent.click(screen.getByLabelText("Remove b.json"));
    expect(onRemove).toHaveBeenCalledWith(1);
  });

  it("remove buttons have aria-label with filename", () => {
    const files = [makeAttached("report.pdf", "application/pdf")];
    render(<FileChipList files={files} onRemove={jest.fn()} />);
    expect(screen.getByLabelText("Remove report.pdf")).toBeInTheDocument();
  });

  it("chip list has accessible role and label", () => {
    const files = [makeAttached("file.txt", "text/plain")];
    render(<FileChipList files={files} onRemove={jest.fn()} />);
    expect(screen.getByRole("list", { name: "Attached files" })).toBeInTheDocument();
  });
});
```

Run with:
```bash
cd web-app && npx jest --no-coverage --testPathPatterns="FileChipList.test"
```

---

## Dependency Graph

```
Epic 1 (Backend) ──► Epic 2 (Frontend Logic) ──► Epic 3 (Frontend UI)
  Story 1.1               Story 2.1                 Story 3.1
  Story 1.2 ──────►       Story 2.2                 Story 3.2
  Story 1.3               Story 2.3                 Story 3.3
  Story 1.4               Story 2.4                 Story 3.4
```

Within each epic, stories execute sequentially. All tasks within a story execute
sequentially.

---

## Test Commands

```bash
# Backend tests (after Epic 1)
make build && go test ./server/services/...

# Frontend unit tests (after Epic 3)
cd web-app && npx jest --no-coverage --testPathPatterns="FileChipList.test"

# Full build validation
make quick-check

# Full CI
make ci
```

---

## Definition of Done

- [ ] `make build` succeeds (no compile errors)
- [ ] `go test ./server/services/...` passes (all new and existing tests)
- [ ] `cd web-app && npx jest --no-coverage` passes
- [ ] `make lint` passes (no CSS undefined-var or TypeScript errors)
- [ ] POST to `/api/upload/file` with `contentType: "application/json"` returns 200 with `.json` path
- [ ] POST to `/api/upload/file` with `contentType: "application/x-unknown"` and `originalFilename: "foo.rs"` returns 200 with `.rs` path
- [ ] A 20 MB file upload returns 200; a 20 MB + 1 byte file returns 413
- [ ] UI shows chip list with × button for each file
- [ ] Image files show inline thumbnail; non-image files show `FileCode`/`FileText`/`FileArchive`/`File` icon
- [ ] `Remove <filename>` accessible label present on each × button
- [ ] More than 3 files can be attached
- [ ] All file types accepted in file picker dialog
