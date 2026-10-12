# Plan: AI Backlog Input UX

**Date:** 2026-10-11

## Summary

Three targeted changes to `Omnibar.tsx` + a new `BacklogComposeWidget` component fix all four defects:

1. A new `backlogComposeText: string | null` state in `Omnibar.tsx` drives a "compose mode" that replaces the result list (parallel to the existing `backlogReviewText` review mode).
2. `onCreateBacklog` pre-fills compose mode from the current `input` value instead of replacing it with `"backlog: "`.
3. `BacklogComposeWidget` is a standalone multi-line textarea + image attachment + "Create" submit button.
4. On submit, `BacklogComposeWidget` calls `onClose()` immediately, then fires `createBacklogItemFromChat(rawText)` in the background and delivers a success/failure notification via `useNotificationCommands`.

## File Inventory

| File | Change |
|---|---|
| `web-app/src/components/sessions/Omnibar.tsx` | Add `backlogComposeText` state; wire `onCreateBacklog` to set it from current `input`; add compose early-return render branch |
| `web-app/src/components/backlog/BacklogComposeWidget.tsx` | New component: textarea, attachment, background submit |
| `web-app/src/components/backlog/BacklogComposeWidget.css.ts` | New vanilla-extract styles |
| `web-app/src/components/sessions/__tests__/Omnibar.backlogCompose.test.tsx` | Tests: text preservation, compose open/close, background submit notification |

No backend changes. No proto changes. No registry-generate changes (no new RPC or React page).

## Detailed Steps

### Step 1: Add `backlogComposeText` state to `Omnibar.tsx`

```tsx
const [backlogComposeText, setBacklogComposeText] = useState<string | null>(null);
```

Change `onCreateBacklog` wiring (Omnibar.tsx ~line 2034):
```tsx
// Before:
onCreateBacklog={() => {
  setInput("backlog: ");
  setResultHighlightIndex(-1);
  inputRef.current?.focus();
}}

// After:
onCreateBacklog={() => {
  setBacklogComposeText(input.trim());   // preserve current input; empty string → blank compose
  setResultHighlightIndex(-1);
}}
```

Change keyboard shortcut at line 1035 similarly (when arrow-key selects the backlog list item):
```tsx
// Before:
setInput("backlog: ");
// After:
setBacklogComposeText(input.trim());
```

### Step 2: Add compose early-return render branch to `Omnibar.tsx`

After the existing `backlogReviewText !== null` branch (line 1700), add:

```tsx
if (backlogComposeText !== null) {
  return (
    <div className={overlay} onClick={onClose} role="dialog" aria-modal="true" aria-labelledby="omnibar-title" data-testid="omnibar">
      <div className={modal} data-testid="omnibar-modal" onClick={(e) => e.stopPropagation()}>
        <BacklogComposeWidget
          initialText={backlogComposeText}
          onClose={onClose}
          onCancel={() => setBacklogComposeText(null)}
        />
      </div>
    </div>
  );
}
```

`onCancel` returns to the omnibar (clears compose mode without closing); `onClose` (from props) closes the whole modal.

### Step 3: Implement `BacklogComposeWidget`

```tsx
// web-app/src/components/backlog/BacklogComposeWidget.tsx
"use client";
// +feature: backlog:compose-widget

export function BacklogComposeWidget({ initialText, onClose, onCancel }) {
  const [text, setText] = useState(initialText);
  const [attachmentError, setAttachmentError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const { createBacklogItemFromChat } = useBacklogService();
  const { addNotification } = useNotificationCommands();
  const router = useRouter();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const uploadAttachment = async (file: File) => {
    if (!file.type.startsWith("image/")) {
      setAttachmentError("Only image files can be attached.");
      return;
    }
    if (file.size > MAX_ATTACHMENT_BYTES) {
      setAttachmentError(`File too large — maximum is 10 MB.`);
      return;
    }
    setUploading(true);
    try {
      const cursorPos = textareaRef.current?.selectionStart ?? text.length;
      const formData = new FormData();
      formData.append("file", file);
      const resp = await fetch(`${getApiBaseUrl()}/v1/upload-backlog-attachment`, {
        method: "POST",
        body: formData,
      });
      if (!resp.ok) {
        setAttachmentError("Upload failed.");
        return;
      }
      const data = await resp.json() as { path: string; filename: string };
      const url = `${getApiBaseUrl()}/local/serve/${data.path}`;
      const markdown = `![${data.filename}](${url})`;
      const next = text.slice(0, cursorPos) + "\n" + markdown + "\n" + text.slice(cursorPos);
      setText(next);
    } catch {
      setAttachmentError("Network error — upload failed.");
    } finally {
      setUploading(false);
    }
  };

  const handleSubmit = () => {
    if (!text.trim()) return;
    onClose();   // close immediately — background processing takes over
    void (async () => {
      const result = await createBacklogItemFromChat(text.trim());
      if (result) {
        const item = result.item;
        addNotification({
          sessionId: "",
          sessionName: item.title,
          message: `Backlog item created: "${item.title}"`,
          notificationType: "custom",
          onView: () => router.push(`/backlog?item=${item.id}`),
        });
      } else {
        addNotification({
          sessionId: "",
          sessionName: "Backlog",
          message: "Failed to create backlog item — check your input and try again.",
          notificationType: "custom",
        });
      }
    })();
  };

  return (
    <div>
      <h2 id="omnibar-title">Create backlog item</h2>
      <label htmlFor="backlog-compose-textarea">Describe the work</label>
      <textarea
        id="backlog-compose-textarea"
        ref={textareaRef}
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={6}
        placeholder="Describe what needs to be done…"
        data-testid="backlog-compose-textarea"
      />
      {attachmentError && <p role="alert">{attachmentError}</p>}
      <div>
        <button
          type="button"
          onClick={() => fileInputRef.current?.click()}
          disabled={uploading}
          aria-label="Attach image"
          data-testid="backlog-compose-attach"
        >
          {uploading ? "Uploading…" : "📎 Attach image"}
        </button>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          style={{ display: "none" }}
          onChange={(e) => { const f = e.target.files?.[0]; if (f) void uploadAttachment(f); }}
        />
        <button type="button" onClick={onCancel} data-testid="backlog-compose-cancel">Cancel</button>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={!text.trim() || uploading}
          data-testid="backlog-compose-submit"
        >
          Create
        </button>
      </div>
    </div>
  );
}
```

Actual styles go in `BacklogComposeWidget.css.ts` (vanilla-extract), following `BacklogItemForm.css.ts` conventions.

### Step 4: Tests

`Omnibar.backlogCompose.test.tsx`:
- Clicking "create backlog" button with pre-typed text → compose widget opens with that text pre-filled.
- Clicking "create backlog" button with empty input → compose widget opens with empty textarea.
- Cancelling compose → returns to omnibar discovery mode without closing.
- Submitting → calls `createBacklogItemFromChat`, then `addNotification` with item title.
- Submitting → omnibar closes immediately (before the RPC resolves).
- Failed submission → error notification fires.

## Adversarial Review

**Challenge 1: "Background fire-and-forget loses the user's text on browser refresh."**  
Accepted. The compose widget is ephemeral; a refresh discards in-progress state. This matches every other omnibar action. If persistence is needed, it's a separate item.

**Challenge 2: "`useNotificationCommands` requires `NotificationContext` in the provider tree — what if it's absent?"**  
`BacklogComposeWidget` is only ever rendered inside the omnibar modal, which is inside the app layout, which already wraps `NotificationContext.Provider`. No risk.

**Challenge 3: "The `notificationType: 'custom'` might not be a valid enum value."**  
Check: `addNotification` accepts a `NotificationData` shape where `notificationType` is typed. Use the correct enum value from `notificationMapping.ts` — likely `"info"` or a specific type constant, not a free string. Must verify during implementation.

**Challenge 4: "`onCancel` returns to discovery mode — but if the user had typed something and then opened compose, cancelling restores a blank omnibar."**  
The `input` state in `Omnibar.tsx` is not cleared when entering compose mode (only `backlogComposeText` is set). `onCancel` just calls `setBacklogComposeText(null)`, which returns to the normal omnibar render with `input` still intact. No data loss.

**Challenge 5: "Two states (`backlogComposeText` and `backlogReviewText`) can both be set simultaneously if code paths overlap."**  
They cannot simultaneously be non-null because: compose is set by `onCreateBacklog`; review is set by submitting a `backlog:` prefix input. The two entry points are mutually exclusive. Add a guard anyway: entering review mode clears compose (`setBacklogComposeText(null)`) and vice versa.

## Implementation Notes (deviations from the plan above)

- Upload logic was extracted from `BacklogItemForm` into `lib/hooks/useBacklogImageAttachment.ts` and shared with the compose widget; the sketch's inline copy used the wrong serve URL (`/api/local/serve${path}` is correct) and would have tripped jscpd.
- Background creation lives in `lib/hooks/useBacklogBackgroundCreate.ts`, called from `Omnibar` (which stays mounted) rather than from the widget, so the work isn't tied to the widget's unmount. The widget takes `onSubmit(text)`.
- Notification types: `task_complete` (success, `onView` deep-links to `/backlog?item=<id>`) and `error` — both valid members of `NotificationData["notificationType"]`.
- The keyboard-selected "Create Backlog Item" row (`dispatchHighlightedResultAction`) opens compose mode too.
- AC-7: `jest-axe` is not a dependency; verified structurally (visible `<label>`, native `<button>`s, ≥44px targets) and left to the CI Axe job for the scan.
