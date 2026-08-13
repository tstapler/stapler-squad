# Findings: UX Patterns for Multi-File Attachment in AI Chat / Code Assistant UIs

## Summary

Developer-facing AI tools have converged on a **horizontal chip/pill row** for displaying
attached files beneath the text input, with each chip showing a file-type icon, truncated
filename, and an inline × close button. Images get a small thumbnail in place of the
generic icon. Non-image files use a categorized icon (document, code, archive) with a
color or shape cue. This pattern fits dense UIs because chips consume minimal vertical
space and each file is individually removable without a separate "clear all" affordance.

---

## Options Surveyed

| Tool | Attachment display | Image handling | Remove pattern |
|---|---|---|---|
| **Claude.ai** (web) | Horizontal chip row above input; up to 20 files | Inline thumbnail in chip | × icon on each chip |
| **ChatGPT** (web) | Chips/pills row, stacks if many files; up to 10 per message | Image thumbnail in pill | × icon on chip |
| **Cursor** (@-mention context) | Context pills in chat header; "@filename" label | N/A (code files only via mention) | Backspace / click-× to dismiss |
| **GitHub Copilot Chat** (VS Code) | Small icon + filename shown in attachment area; up to 3 images per prompt (images only as of 2025-03) | Image thumbnail | × or trash icon |
| **Carbon Design System** (IBM) | Vertical list of file rows with loading/success/error state | No built-in thumbnail | × icon on each row |
| **ServiceNow Horizon** | Attachment cards (thumbnail + name + metadata + action menu) | Thumbnail for images | Action menu with delete |
| **HPE Design System** | File list rows | No thumbnail | Trash icon button |

---

## Trade-off Matrix

| Display style | Space efficiency | Visual scannability | Works for many files | Thumbnail support | Complexity |
|---|---|---|---|---|---|
| **Horizontal chip row** | High | Medium | Medium (wraps, can scroll) | Yes (small inline) | Low |
| **Vertical list** | Low | High (more room for metadata) | High | Yes | Low |
| **Card grid** | Low | Very high | Low | Yes (large) | Medium |
| **@-mention pills** (Cursor) | Very high | Low | High | No | Very low |

For a dense developer UI the **horizontal chip row** wins on space efficiency while still
being immediately scannable. Vertical lists are preferred in design-system guidance
(Carbon, HPE) because they scale to many files and support per-file status indicators
(upload progress, error), but they consume more vertical real estate.

---

## Key Design Patterns Observed

### 1. Horizontal chips below the input (dominant pattern)

Claude.ai and ChatGPT both render a `flex-wrap` row of chips just above or below the
text input. Each chip contains:
- A small file-type icon (or thumbnail for images) on the left
- Filename truncated with ellipsis
- × close button on the right (or top-right corner)

Chips wrap to a second line when the row is full. This is the pattern closest to what
the requirements describe as "file list."

### 2. File-type categorization with icons + color

Most tools assign a distinct icon and/or color per file category rather than per MIME
type. The common taxonomy (used by Carbon, ChatGPT, VS Code Copilot, and icon libraries
like Font Awesome):

| Category | Icon | Color cue |
|---|---|---|
| Image | Mountain/image glyph or thumbnail | Blue/teal |
| Code / script | `</>` or curly brace glyph | Yellow/orange |
| Document (PDF, Word, etc.) | Lined-page glyph | Red (PDF) / Blue (Word) |
| Spreadsheet | Grid glyph | Green |
| Archive (zip, tar) | Box/package glyph | Gray/brown |
| Generic / binary | Plain page glyph | Gray |

Font Awesome `fa-file-image`, `fa-file-code`, `fa-file-pdf`, `fa-file-zipper`,
`fa-file` are the standard glyphs. VS Code itself uses a subset of these for its
Explorer file icons (via the Seti icon theme).

### 3. Remove affordance: inline × button per chip

Every major tool (Claude.ai, ChatGPT, Carbon, HPE, Mesh) uses an **inline close/delete
button on each chip**. Conventions:
- Icon: `×` character, `✕`, or a small `Trash` SVG
- Placement: right side of the chip, or top-right corner (overlapping the chip border)
  for a "badge" style
- Hover state: the chip background darkens or the × turns red/primary-color
- No separate "remove all" button — each file is removed individually

The Carbon Design System explicitly documents: *"To remove an uploaded file, click the
× (or delete) icon."*

### 4. Images get thumbnails; non-images get icons

Claude.ai, ChatGPT, and GitHub Copilot Chat all show:
- **Images**: a small square thumbnail (16–32 px) generated client-side via
  `URL.createObjectURL()` or `FileReader`, replacing the generic file icon
- **Non-images**: a categorized file icon from a standard glyph set

This dual treatment is the de-facto standard. The thumbnail must be clipped to a circle
or rounded square to fit within the chip without distorting its layout.

### 5. Filename truncation

All tools truncate long filenames with an ellipsis in the middle (`file…name.ext`) or
at the end (`longfilena….ext`). Max displayed width is ~120–180 px in most
implementations. A tooltip on hover shows the full filename (Carbon DS documents this
explicitly).

---

## Risk and Failure Modes

| Failure | Condition | Mitigation |
|---|---|---|
| Chip row grows too tall with many files | >6–8 chips in a narrow panel | Max-height + vertical scroll on chip container, or cap display with "+N more" label |
| Thumbnail renders blurry | Large image scaled to 24 px | Use `object-fit: cover` and `image-rendering: pixelated` or `crisp-edges` |
| Filename too long breaks layout | No truncation | CSS `max-width` + `overflow: hidden; text-overflow: ellipsis` on filename span |
| Wrong icon shown for edge-case MIME | MIME not in category map | Map to `generic` category as fallback |

---

## Migration and Adoption Cost

This research informs a UI change to `OmnibarCreationPanel.tsx`. The patterns above
require:
- A new `AttachedFile` chip component (replaces image-thumbnail-only display)
- A file-type icon helper (maps MIME/extension → icon + color)
- Client-side thumbnail generation (already present for images; keep as-is)
- The existing `AttachedImage` state list becomes `AttachedFile[]` (see NFR-4)

No new external dependencies are required — File API, URL.createObjectURL, and SVG
icons from the existing icon set (Lucide/Heroicons already used in the project) are
sufficient.

---

## Operational Concerns

- All thumbnail generation is client-side; no server round-trip for preview
- MIME-to-category mapping should be exhaustive enough to cover common dev file types
  (`.go`, `.ts`, `.py`, `.rs`, `.json`, `.yaml`, `.zip`, `.tar.gz`, `.pdf`, `.png`)
- The chip row must be keyboard-accessible: each × button gets `aria-label="Remove <filename>"`

---

## Prior Art and Lessons Learned

- **Claude.ai** is the most directly relevant prior art: chips above the input, images
  as thumbnails, × buttons, no file count badge. Chip styling matches the input's
  border-radius and dark theme.
- **ChatGPT** wraps chips to a second row when > ~4 files are attached; the chip row
  becomes scrollable horizontally on narrow viewports.
- **Carbon DS** (IBM) is the most documented design system pattern and matches well:
  vertical file list with status indicators is their baseline but chips are mentioned
  as a compact variant in their GitHub issues (#20388).
- **VS Code Copilot** (as of 2025-03) limits to 3 image attachments and does not
  generalize to non-image files — not a good reference for multi-type behavior.

---

## Open Questions

- [ ] Does the project already use Lucide icons? Check `web-app/package.json` — if yes,
  `File`, `FileCode`, `FileImage`, `FileArchive`, `FileText` cover all categories
  without a new dependency.
- [ ] Should the chip row scroll horizontally or wrap? Horizontal scroll works better
  at narrow widths but hides chips from view; wrapping is more accessible.

---

## Recommendation

**Recommended pattern**: Horizontal chip row with thumbnail-or-icon + truncated filename + × button.

**Reasoning**: This is the convergent solution across Claude.ai, ChatGPT, and Copilot
Chat — all tools a developer using this product already knows. It is the most
space-efficient option for a dense UI (horizontal, compact chips), supports mixed
image/non-image files through the thumbnail-vs-icon duality, and each × button satisfies
FR-4 (individual file removal) without extra UI chrome.

**Accept these costs**: With many files (>8) the chip row wraps and can push the text
input down, requiring a max-height cap or scroll. A vertical list would handle large
counts more gracefully, but the requirements do not anticipate extremely large file lists.

**Reject these alternatives**:
- *Card grid*: excessive vertical space for a dense developer UI; reserved for
  media-centric (photos) contexts.
- *@-mention pills only (Cursor style)*: no thumbnail support, no file metadata; too
  minimal for the requirement to show file type badge/icon.
- *Vertical list (Carbon DS default)*: better for status indicators during upload but
  wastes vertical space that should go to the session prompt.

---

## Sources

- [Upload files to Claude — Claude Help Center](https://support.claude.com/en/articles/8241126-upload-files-to-claude)
- [Copilot Chat Vision input public preview — GitHub Changelog](https://github.blog/changelog/2025-03-05-copilot-chat-users-can-now-use-the-vision-input-in-vs-code-and-visual-studio-public-preview/)
- [Carbon Design System — File Uploader Usage](https://carbondesignsystem.com/components/file-uploader/usage/)
- [ServiceNow Horizon — Attachments](https://horizon.servicenow.com/workspace/components/now-record-common-attachments-connected)
- [HPE Design System — FileInput](https://design-system.hpe.design/components/fileinput)
- [File type icons guide — Vibe Icons](https://www.vibe-icons.com/blog/file-type-icons)
- [File upload UI tips — Eleken](https://www.eleken.co/blog-posts/file-upload-ui)
- [Cursor context management — Steve Kinney](https://stevekinney.com/courses/ai-development/cursor-context)
- [ChatGPT Plus File Uploads guide — AIonX](https://aionx.co/chatgpt-reviews/chatgpt-plus-file-uploads/)
- [Font Awesome file type icons — W3Schools](https://www.w3schools.com/icons/fontawesome_icons_filetype.php)
