# Pitfalls, Security Issues, and Edge Cases: Any-File Upload

Research for generalizing `image_upload_handler.go` (pre-session, base64/JSON) and
`session_image_upload_handler.go` (in-session, multipart) from image-only to any-file.

---

## 1. Security: Executable / Script Upload and Code Execution Risk

### What the server does
Neither handler ever executes uploaded files. Files are written to disk with `0o600`
permissions (owner read/write only) and their absolute paths are returned to the
frontend. The paths are later injected into the agent's `initialPrompt` as a
space-joined list (see `Omnibar.tsx` line 739, `instance_tmux.go` line 39):

```go
// buildLaunchCommand — paths are passed as a quoted CLI argument to claude
program = fmt.Sprintf("%s %q", program, i.Prompt)
```

`%q` applies Go string quoting, so the entire prompt (including file paths) is
shell-escaped as a single argument. The server itself has **no code execution risk**.

### Residual AI-level risk
The agent (Claude Code) that receives the file paths is an autonomous AI with tool
access. If a user uploads a shell script named `fix.sh` and the prompt text says
"please run this file", Claude Code may attempt to execute it using its `Bash` tool.
This is an AI behavioural issue, not an OS-level exploit, and is no worse than the
user typing `bash /tmp/fix.sh` into the chat manually.

**Mitigation**: The uploaded path context sent to the agent should be framed as
"here are the attached files for reference" rather than an implicit execution
instruction. No extension-level blocking is needed, but the prompt injection text
should be neutral (e.g., `"Attached files:\n" + paths.join("\n")`).

### Content-type sniffing cannot detect most executables
`net/http.DetectContentType` (used in the session handler) returns:
- ELF binary → `application/octet-stream`
- PE (.exe) → `application/octet-stream`
- Shell script (`#!/bin/bash`) → `text/plain; charset=utf-8`
- Python script → `text/plain; charset=utf-8`

Sniffing cannot reliably distinguish executable files from benign binaries or plain
text. Do **not** rely on magic-byte detection as a security gate for any-file upload.

---

## 2. Path Traversal: Filename to Disk Path

### Pre-session handler (`image_upload_handler.go`)
Currently safe: `extensionFor(contentType)` returns a hardcoded value from a
whitelist (`".png"`, `".jpg"`, etc.). The filename passed to `os.CreateTemp` is
`"paste-*"` + that fixed extension. The original filename from the client is
**never used**.

### Risk when generalizing
If the generalized handler uses the client-supplied filename (e.g., for keeping the
original name as part of the `CreateTemp` suffix), a crafted filename like
`../../etc/passwd` could escape the paste directory.

**Go's `os.CreateTemp(dir, pattern)` does not clean the pattern.** Example:
```
dir = /tmp/stapler-paste
pattern = "1234567890-*-../../etc/passwd"
→ file created at: /tmp/stapler-paste/1234567890-RAND-../../etc/passwd
  (which the OS resolves to /tmp/etc/passwd)
```

The session handler avoids this with `sanitizeFilename()` (lines 66–93 of
`session_image_upload_handler.go`), which calls `filepath.Base()` to strip directory
components, replaces `/`, `\`, and null bytes with `_`, and truncates to 100 chars.
**The pre-session handler lacks this sanitization entirely.**

**Fix**: Apply the same `sanitizeFilename()` logic (or a copy) in `image_upload_handler.go`
when deriving any part of the filename from client-supplied input.

### Extension derivation from MIME type
`mime.ExtensionsByType` can return dangerous extensions for some MIME types:
- `application/x-msdownload` → `[".cpl", ".dll", ".drv", ".exe", ".scr"]`
- `application/x-sharedlib` → `[".so"]`
- `text/x-python` → `[".py", ".pyx", ".wsgi"]`

If the handler naively picks `exts[0]`, it may write a file with an executable
extension. This does not cause OS-level execution but could mislead the AI agent.

**Fix**: Derive extension with a safe priority order:
1. Try `mime.ExtensionsByType(contentType)` — pick first result.
2. If empty or contentType is `application/octet-stream`, fall back to the file's
   original extension (sanitized — strip path components, allow only `[a-zA-Z0-9]`
   and `.`, max 10 chars).
3. Default to `.bin`.

The extension is purely cosmetic for path injection. No extension blocklist is
needed since the server never executes files.

---

## 3. Base64 Encoding Overhead: Memory Impact

### The encoding mismatch bug (pre-session handler)
`image_upload_handler.go` sets:
```go
r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes) // maxImageBytes = 20 MB
```

The body is JSON containing a **base64-encoded** file. Base64 inflates file size by
~33% (`4/3` ratio). A 20 MB binary file encodes to **~26.7 MB** of base64 in the
JSON body, which **exceeds the 20 MB `MaxBytesReader` limit** and is rejected with
HTTP 413.

The effective upload limit is therefore **~15 MB** of actual file data, not 20 MB.
The comments and constants say "20 MB" but the real limit is ~15 MB.

**Fix**: Set `MaxBytesReader` to `maxFileBytes * 4 / 3 + overhead`:
```go
const maxFileBytes = 20 * 1024 * 1024
r.Body = http.MaxBytesReader(w, r.Body, maxFileBytes*4/3+4096) // ~26.7 MB + padding
```

Or, better: switch the pre-session endpoint to `multipart/form-data` (matching the
session handler), which avoids base64 entirely. This halves the wire size and
eliminates the encoding overhead in both browser and server memory.

### Server memory per upload
With base64 JSON (current pre-session approach):
- `json.Decoder` buffers the full body: up to 26.7 MB.
- `base64.DecodeString` allocates the decoded bytes: up to 20 MB.
- Peak Go heap per request: **~47 MB**.

With 10 concurrent uploads of 20 MB files: **~470 MB peak RSS**.

With multipart (streaming):
- `io.Copy` streams directly from multipart reader to disk file.
- Peak Go heap per request: **~512 B** (the multipart buffer).

### Browser memory (no client-side size check)
The frontend has no file size validation before reading. For a 100 MB file:
- `FileReader.readAsDataURL` materialises the full file in memory: 100 MB.
- The base64 string result: ~133 MB.
- Total JS heap pressure: **~233 MB per file**.

With no file count cap and no size guard, selecting 5 × 100 MB files would push
the browser to ~1.2 GB of heap pressure before any network request is made.

**Fix**: Add a client-side size check before calling `fileToBase64`:
```ts
const MAX_FILE_BYTES = 20 * 1024 * 1024; // 20 MB
if (file.size > MAX_FILE_BYTES) {
  setAttachError(`${file.name} is too large (max 20 MB)`);
  continue;
}
```

---

## 4. Content-Type Spoofing

### Pre-session handler (trusts client claim entirely)
`extensionFor(req.ContentType)` trusts the `contentType` field in the JSON body.
There is no magic-byte verification. A client claiming `"image/png"` while sending
ELF bytes will write an ELF binary to disk with a `.png` extension.

After generalizing to any-file, the handler will derive the extension from the
client's claimed MIME type. A client claiming `"text/plain"` for a binary file
gets a `.txt` extension. **This is acceptable** for this use case because:

1. The server never executes the file.
2. The AI agent reads the file as instructed by the user — the extension is informational.
3. The sole user/developer is not an adversary to themselves.

**No server-side magic-byte verification is required** for the pre-session handler.
The session handler already does sniffing; for the session handler, when generalizing
to any-file, the `isAllowedImageType` guard should be removed (or replaced with a
permissive allowlist/no-op).

### Content-Type normalization
`mime.ExtensionsByType` is case-sensitive and may fail on improperly formatted
content types like `"Image/PNG"` or `"image/png; charset=utf-8"`.

**Fix**: Normalize before lookup:
```go
ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
exts, _ := mime.ExtensionsByType(ct)
```

---

## 5. TypeScript: `URL.createObjectURL()` for Non-Image Files

### Behaviour
`URL.createObjectURL(file)` works for **any** `File` or `Blob` object regardless of
type. It creates a `blob:` URL that the browser can serve. There are no errors or
warnings for non-image files.

### The previewUrl problem
The current code (line 215 of `OmnibarCreationPanel.tsx`) creates a `previewUrl` for
every file before knowing if the upload succeeds:
```ts
const previewUrl = URL.createObjectURL(file);
```

For non-image files:
- The blob URL is valid but displays as a broken image icon in `<img src={...}>`.
- The blob URL keeps the file's bytes in browser memory until revoked.
- For a 100 MB zip file, this wastes 100 MB in the browser until the component
  unmounts or the file is removed.

**Fix**: Only create the blob URL for image files:
```ts
const isImage = file.type.startsWith("image/");
const previewUrl = isImage ? URL.createObjectURL(file) : undefined;
```

Make `previewUrl` optional in the `AttachedFile` type (as required by NFR-4):
```ts
interface AttachedFile {
  file: File;
  path: string;
  previewUrl?: string; // only set for image/* files
}
```

Cleanup in `removeFile` and unmount must guard against undefined:
```ts
if (f.previewUrl) URL.revokeObjectURL(f.previewUrl);
```

### `createObjectURL` for the same file object selected twice
`URL.createObjectURL` with two references to the same file creates two independent
blob URLs. Both are valid and must be individually revoked.

---

## 6. Same File Selected Twice

### Current behaviour
`e.target.value = ""` (line 204) resets the input value after each selection, which
is correct — it allows selecting the same file again in a subsequent dialog open.
However, there is **no deduplication check**. Selecting the same file twice:

1. Creates two `File` objects (different JS identity, same content).
2. Creates two blob URLs.
3. Uploads the file twice (two separate HTTP requests, two temp files on disk).
4. Injects both paths into the agent prompt.

The agent receives duplicate paths and may process the file twice, potentially
producing duplicate results or confusing the agent about which file is "the" source.

**Fix**: Deduplicate by `(name, size, lastModified)` tuple before uploading:
```ts
const existing = attachedFiles.map(f => `${f.file.name}|${f.file.size}|${f.file.lastModified}`);
const toUpload = files.filter(f => 
  !existing.includes(`${f.name}|${f.size}|${f.lastModified}`)
);
```

Note: `lastModified` is millisecond precision and suffices for practical
deduplication within a single session; it is not a cryptographic guarantee.

---

## Summary Table

| # | Issue | Severity | Affected Handler | Fix |
|---|-------|----------|-----------------|-----|
| 1 | Code execution risk (AI level) | Low | Both | Neutral prompt framing |
| 2 | Path traversal via crafted filename | Medium | Pre-session only | Apply `sanitizeFilename()` |
| 3a | `MaxBytesReader` too small for base64 (15 MB not 20 MB) | High | Pre-session | Fix limit formula or switch to multipart |
| 3b | No client-side size check; large files exhaust JS heap | Medium | Frontend | Add `file.size` guard before FileReader |
| 4 | Content-Type not normalized before `mime.ExtensionsByType` | Low | Pre-session (generalized) | Normalize: lowercase, strip params |
| 5 | Blob URL created for non-image files wastes memory | Low | Frontend | Conditional `createObjectURL` |
| 6 | Same file selected twice → duplicated uploads | Low | Frontend | Deduplicate by name+size+mtime |

---

## Recommended Implementation Order

1. **Fix `MaxBytesReader` limit** (or switch to multipart) — correctness bug, not just security.
2. **Apply `sanitizeFilename`** to the pre-session handler's extension/suffix derivation.
3. **Add client-side `file.size` check** before FileReader to avoid browser OOM.
4. **Conditional `previewUrl`** — only create blob URLs for images.
5. **Deduplicate** on name+size+lastModified in `handleAttachFiles`.
6. **Normalize content-type** before `mime.ExtensionsByType` lookup.
