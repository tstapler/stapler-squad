# Requirements: Multi-File Upload for Actions Tab

## Problem Statement

The image upload button in the session actions tab currently only supports images (PNG, JPEG, GIF, WEBP), accepts at most 3 files, and enforces image-only MIME type validation on both frontend and backend. Users need to be able to upload any file type (code, documents, archives, etc.) and attach multiple files at once so the agent can operate on them.

## Stakeholder

Tyler Stapler (sole user / developer)

## Current State

- File: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`
- Input: `accept="image/*"`, max 3 files
- Upload endpoint: `POST /api/upload/image` — rejects non-image MIME types with HTTP 400
- Frontend type: `AttachedImage { file, path, previewUrl }`
- UI: shows image thumbnails only; 3-image hard cap enforced on both ends
- Files are stored in `$TMPDIR/stapler-paste/` and their absolute paths are injected into the session's `initialPrompt`

## Functional Requirements

### FR-1: Accept Any File Type
The file input must accept any file type (`accept="*/*"` or no `accept` attribute). The backend must not reject files based on MIME type — it should derive a safe file extension from the MIME type when available, and fall back to the original file extension or `.bin`.

### FR-2: Multiple File Upload
The user must be able to select multiple files in a single dialog open. The `multiple` attribute is already set — this requirement ensures the backend handles each file independently.

### FR-3: No Artificial File Count Limit
Remove the hard cap of 3 files. The backend imposes no per-session count limit (individual file size limit of 20 MB is retained).

### FR-4: File List UI
When files are attached, display them as a list:
- Filename (truncated if long)
- File type badge or icon (image, code, document, archive, other)
- For images: show a small thumbnail preview
- For non-images: show a generic file icon
- Each entry has a remove (×) button

### FR-5: Retain Image Thumbnail Behavior
Images continue to show a small preview thumbnail in the file list. Other file types show a document/file icon.

### FR-6: Paths Injected into Prompt (unchanged)
The existing behavior of injecting absolute server-side paths into the session `initialPrompt` is unchanged. All uploaded file paths are joined with spaces (or newlines) and prepended to the user's prompt so the agent can reference them.

## Non-Functional Requirements

### NFR-1: Security — File Extension Sanitization
The backend must sanitize the filename/extension before writing to disk. Do not use the original filename as-is. Derive extension from MIME type when possible; fallback to `.bin`. Keep existing `0o600` file permissions.

### NFR-2: Individual File Size Limit
Retain the existing 20 MB per-file limit.

### NFR-3: No Backend Route Rename Required (backward compat)
The route can stay at `/api/upload/image` or be renamed to `/api/upload/file` — no external consumers. Prefer rename for clarity.

### NFR-4: TypeScript Type Rename
Rename `AttachedImage` → `AttachedFile` and update `previewUrl` to be optional (only for images).

## Out of Scope

- Drag-and-drop upload
- Progress bars / chunked upload
- File manager / persistent file library
- Backend compression or transcoding
- Paste-from-clipboard (separate feature)

## Acceptance Criteria

1. User can open file picker and select any file type
2. User can select multiple files at once
3. After selection, a list shows filename + type for each file; images also show thumbnail
4. Each file can be removed individually
5. More than 3 files can be attached
6. Session creation proceeds with all file paths injected into the prompt
7. Backend accepts non-image MIME types without returning 400
8. Backend sanitizes file extensions
9. Existing image upload behavior continues to work unchanged

## Technical Context

| Component | Path |
|---|---|
| Frontend attach button + handler | `web-app/src/components/sessions/OmnibarCreationPanel.tsx` lines 202–239, 579–596 |
| Frontend type | `AttachedImage` interface ~line 123 |
| Backend handler | `server/services/image_upload_handler.go` |
| Route registration | `server/server.go` line 439 |
| Old paste cleanup | `image_upload_handler.go` `CleanupOldPasteFiles()` |
