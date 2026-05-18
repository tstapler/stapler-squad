# Validation Plan: Multi-File Upload (Any File Type)

Generated: 2026-05-16

---

## Summary

| Test Type | Count |
|---|---|
| Go unit tests | 6 |
| Frontend Jest/RTL tests | 6 |
| **Total** | **12** |

Requirements coverage: **9/9 FRs + NFRs covered (100%)**

---

## Go Unit Tests — `server/services/`

Target file: `server/services/image_upload_handler_test.go`

All tests use the existing `httptest.NewRecorder` pattern and call the handler directly with a crafted `*http.Request`.

---

### T-GO-01: `TestFileUploadHandler_AcceptsAnyMimeType`

**Covers**: FR-1, AC-7

**Purpose**: Confirm the handler returns HTTP 200 (or 201) for file types that were previously blocked.

**Setup**: For each subcase, construct a JSON request body with:
- `contentType`: the MIME type under test
- `base64Data`: a minimal valid base64-encoded payload (~100 bytes, well under 20 MB)

**Sub-cases**:

| Sub-case | `contentType` | `base64Data` source |
|---|---|---|
| PDF | `application/pdf` | `%PDF-1.4` header + padding |
| ZIP | `application/zip` | PK magic bytes + padding |
| Go source | `text/x-go` | `package main\n` UTF-8 |

**Assertions**:
- Response status code is `http.StatusOK` (200)
- Response body contains a non-empty `path` field
- No error in response body

---

### T-GO-02: `TestFileUploadHandler_RejectsOversizedFile`

**Covers**: NFR-2, AC (implicit: 20 MB limit retained)

**Purpose**: Confirm a file exceeding 20 MB is rejected before being written to disk.

**Setup**: Generate a base64 string representing exactly `20*1024*1024 + 1` bytes (binary zeros). Because the `MaxBytesReader` must be set to `maxFileBytes*4/3 + overhead`, this test also validates the fix for pitfall 3a (base64 overhead encoding).

**Assertions**:
- Response status code is `http.StatusRequestEntityTooLarge` (413) or `http.StatusBadRequest` (400)
- No temp file is created in `$TMPDIR/stapler-paste/` (verify via `os.ReadDir` before and after)

**Note**: If the implementation switches to multipart, adapt the request construction accordingly. The assertion on the 20 MB raw limit remains unchanged.

---

### T-GO-03: `TestFileUploadHandler_SanitizesExtension`

**Covers**: NFR-1, pitfall section 2 (path traversal)

**Purpose**: Confirm that a client-supplied `originalFilename` containing path traversal sequences does not escape the paste directory and results in a safe file extension.

**Setup**: Send a request with:
- `contentType`: `application/octet-stream`
- `originalFilename` (if the generalized handler accepts it): `../../evil.sh`

**Assertions**:
- Response status is 200
- Returned `path` field does NOT contain `..` or `/etc/` or any path outside `$TMPDIR/stapler-paste/`
- File extension in the returned path is `.bin` (octet-stream fallback) or the sanitized extension from the MIME type, NOT `.sh`
- File actually exists on disk at the returned path

---

### T-GO-04: `TestFileUploadHandler_FallsBackToBin`

**Covers**: FR-1 (fallback behavior), NFR-1

**Purpose**: Confirm that an unknown or unregistered MIME type results in a `.bin` extension rather than an error or a server-derived dangerous extension.

**Setup**: Send a request with:
- `contentType`: `application/x-stapler-unknown-type-xyzzy` (guaranteed unrecognized by `mime.ExtensionsByType`)
- Minimal valid payload

**Assertions**:
- Response status is 200
- Returned `path` ends with `.bin`

---

### T-GO-05: `TestFileUploadHandler_StillAcceptsImages`

**Covers**: FR-5, AC-9

**Purpose**: Regression test confirming the existing image upload path is not broken by the generalization.

**Setup**: For each image sub-case:

| Sub-case | `contentType` |
|---|---|
| PNG | `image/png` |
| JPEG | `image/jpeg` |
| GIF | `image/gif` |
| WEBP | `image/webp` |

Each uses a minimal valid image payload (1x1 pixel, base64-encoded).

**Assertions**:
- Response status is 200
- Returned `path` ends with the expected extension (`.png`, `.jpg`, `.gif`, `.webp`)

---

### T-GO-06: `TestCleanupHandlesNonImageFiles`

**Covers**: FR-6 (paths injected and cleaned up), implicit NFR (no orphan files)

**Purpose**: Confirm that `CleanupOldPasteFiles()` correctly removes non-image files written by the generalized handler, not just files with image extensions.

**Setup**:
1. Create temp files in `$TMPDIR/stapler-paste/` with extensions: `.pdf`, `.zip`, `.go`, `.bin`, `.png`
2. Set all `ModTime` values to 25 hours ago (past the cleanup threshold)
3. Call `CleanupOldPasteFiles()`

**Assertions**:
- All 5 files are removed (or all 5 files older than the threshold are removed if threshold is checked)
- No error returned from cleanup
- A file with a fresh `ModTime` (current time) is NOT removed

---

## Frontend Jest/RTL Tests — `web-app/src/`

Target component: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`
Target test file: `web-app/src/components/sessions/OmnibarCreationPanel.test.tsx`

All tests use React Testing Library's `render`, `fireEvent`, and `screen` APIs. The upload API call is mocked via `jest.mock` or `msw`. File objects are constructed with `new File([bytes], name, { type })`.

---

### T-FE-01: Go file not rejected client-side

**Covers**: FR-1 (frontend does not block non-image types)

**Purpose**: Confirm that selecting a `.go` source file does not trigger a client-side rejection message or leave the file list empty.

**Setup**:
1. Render `OmnibarCreationPanel` with a mocked upload service that returns `{ path: "/tmp/stapler-paste/paste-abc.go" }`
2. Create `new File(["package main"], "main.go", { type: "text/x-go" })`
3. Fire a `change` event on the file input with this file

**Assertions**:
- No error/rejection alert rendered in the document
- The file list contains one entry with text `main.go`
- The upload mock was called once

---

### T-FE-02: Selecting 5+ files all appear (no 3-cap)

**Covers**: FR-3, AC-5

**Purpose**: Confirm the hard cap of 3 files has been removed from the frontend.

**Setup**:
1. Create 5 distinct `File` objects (e.g., `file1.txt` through `file5.txt`)
2. Fire a single `change` event on the file input with all 5 files

**Assertions**:
- File list renders exactly 5 entries
- No "maximum files" error message is rendered
- Upload mock is called 5 times (or once with all 5, depending on implementation)

---

### T-FE-03: Duplicate file is not added twice

**Covers**: Pitfall 6 (deduplication)

**Purpose**: Confirm that selecting the same file a second time does not add a second entry to the list.

**Setup**:
1. Create `new File(["hello"], "report.pdf", { type: "application/pdf", lastModified: 1700000000000 })`
2. Fire a `change` event adding this file
3. Fire a second `change` event with an identical file object (same `name`, `size`, `lastModified`)

**Assertions**:
- File list contains exactly 1 entry (not 2)
- Upload mock was called exactly once

---

### T-FE-04: 21 MB file is rejected client-side

**Covers**: NFR-2, Pitfall 3b (client-side size guard)

**Purpose**: Confirm that a file exceeding 20 MB is rejected in the browser before any upload attempt, protecting against JS heap exhaustion.

**Setup**:
1. Create a `File` object with `size` property set to `21 * 1024 * 1024` (use `Object.defineProperty` on a mock or construct via `new Blob`)
2. Fire a `change` event with this file

**Assertions**:
- An error message matching `/too large/i` or `/20 MB/i` is rendered
- Upload mock is NOT called
- File list remains empty

---

### T-FE-05: Images get a preview URL, non-images do not

**Covers**: FR-4, FR-5, NFR-4, Pitfall 5

**Purpose**: Confirm conditional `createObjectURL` behavior: blob URL created only for image files.

**Setup**:
Mock `URL.createObjectURL` to return a deterministic string (`"blob:mock-url"`).

Sub-case A (image file):
1. Fire `change` with `new File([bytes], "photo.png", { type: "image/png" })`
2. Inspect the rendered list entry

Sub-case B (non-image file):
1. Fire `change` with `new File([bytes], "archive.zip", { type: "application/zip" })`
2. Inspect the rendered list entry

**Assertions**:
- Sub-case A: `URL.createObjectURL` was called; the entry contains an `<img>` element with `src="blob:mock-url"`
- Sub-case B: `URL.createObjectURL` was NOT called; the entry contains no `<img>` element; a generic file icon is rendered instead

---

### T-FE-06: Clicking × removes only that file

**Covers**: FR-4 (remove button per entry), AC-4

**Purpose**: Confirm per-file removal works correctly and does not affect other files in the list.

**Setup**:
1. Fire `change` event with 3 files: `a.txt`, `b.pdf`, `c.go`
2. Find the remove button (×) for `b.pdf`
3. Click it

**Assertions**:
- File list renders exactly 2 entries
- `a.txt` entry is present
- `c.go` entry is present
- `b.pdf` entry is absent
- If `b.pdf` had a previewUrl (it does not — it's not an image), `URL.revokeObjectURL` would have been called; for this non-image case, confirm `revokeObjectURL` is NOT called (guards against the blob URL leak from pitfall 5)

---

## Requirement-to-Test Traceability Matrix

| Requirement | Description | Tests |
|---|---|---|
| **FR-1** | Accept any file type (frontend + backend) | T-GO-01, T-GO-04, T-FE-01 |
| **FR-2** | Multiple file upload handled independently | T-GO-01 (3 sub-cases), T-FE-02 |
| **FR-3** | No artificial file count limit | T-FE-02 |
| **FR-4** | File list UI with remove button | T-FE-05, T-FE-06 |
| **FR-5** | Image thumbnails retained, non-images get icon | T-GO-05, T-FE-05 |
| **FR-6** | Paths injected into prompt (cleanup preserved) | T-GO-06 |
| **NFR-1** | Extension sanitization (no path traversal) | T-GO-03, T-GO-04 |
| **NFR-2** | 20 MB per-file limit (backend + frontend) | T-GO-02, T-FE-04 |
| **NFR-3** | Route rename / backward compat | Not a behavioral test; verified by integration smoke |
| **NFR-4** | `AttachedFile` type / optional `previewUrl` | T-FE-05 |
| **Pitfall 2** | Path traversal via crafted filename | T-GO-03 |
| **Pitfall 3a** | `MaxBytesReader` base64 overhead | T-GO-02 |
| **Pitfall 3b** | Client-side size guard (browser OOM) | T-FE-04 |
| **Pitfall 5** | Conditional `createObjectURL` (memory leak) | T-FE-05, T-FE-06 |
| **Pitfall 6** | Deduplication by name+size+lastModified | T-FE-03 |
| **AC-4** | Each file can be removed individually | T-FE-06 |
| **AC-5** | More than 3 files can be attached | T-FE-02 |
| **AC-7** | Backend accepts non-image MIME types | T-GO-01 |
| **AC-8** | Backend sanitizes file extensions | T-GO-03, T-GO-04 |
| **AC-9** | Existing image upload behavior unchanged | T-GO-05 |

---

## NFR-3 Note

NFR-3 (route rename to `/api/upload/file`) is a configuration/registration concern, not a behavioral correctness test. It is verified by the smoke test in CI that the route returns a non-404 response. No dedicated unit test is required; include a single route registration assertion in the integration or e2e layer.

---

## Coverage Fraction

- **Functional requirements**: FR-1 through FR-6 — all 6 covered ✓
- **Non-functional requirements**: NFR-1, NFR-2, NFR-4 covered; NFR-3 covered by integration smoke ✓
- **Acceptance criteria**: AC-4, AC-5, AC-7, AC-8, AC-9 covered; AC-1/2/3/6 covered transitively by T-FE-01 and T-GO-01 ✓
- **Pitfalls from research**: Pitfalls 2, 3a, 3b, 5, 6 explicitly covered; Pitfall 1 (AI behavior) and Pitfall 4 (content-type normalization) are implementation details exercised indirectly by T-GO-01 and T-GO-04 ✓

**Overall requirement coverage: 9/9 FRs+NFRs (100%), 5/6 pitfalls with dedicated tests (pitfall 4 covered indirectly)**
