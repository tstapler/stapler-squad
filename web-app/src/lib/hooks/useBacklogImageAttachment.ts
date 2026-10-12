"use client";

import { useCallback, useState } from "react";
import type { ClipboardEvent, ChangeEvent, RefObject } from "react";
import { getApiBaseUrl } from "@/lib/config";

export const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024; // 10 MB — matches server-side cap

interface Options {
  textareaRef: RefObject<HTMLTextAreaElement | null>;
  setValue: (updater: (prev: string) => string) => void;
}

/**
 * Uploads an image to /v1/upload-backlog-attachment and inserts a markdown
 * image reference at the textarea cursor as it is when the upload finishes
 * (not when it started), so typing during the upload is never overwritten.
 */
export function useBacklogImageAttachment({ textareaRef, setValue }: Options) {
  const [error, setError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);

  const upload = useCallback(
    async (file: File) => {
      if (!file.type.startsWith("image/")) {
        setError("Only image files can be attached.");
        return;
      }
      if (file.size > MAX_ATTACHMENT_BYTES) {
        setError("Image is too large (max 10 MB).");
        return;
      }
      setError(null);
      setUploading(true);
      try {
        const formData = new FormData();
        formData.append("file", file);
        const resp = await fetch(`${getApiBaseUrl()}/v1/upload-backlog-attachment`, {
          method: "POST",
          body: formData,
        });
        if (!resp.ok) {
          setError(
            resp.status === 413
              ? "Image is too large (max 10 MB)."
              : resp.status === 415
                ? "Unsupported image type — use PNG, JPEG, GIF, or WebP."
                : "Upload failed."
          );
          return;
        }
        const data = (await resp.json()) as { path: string; filename: string };
        const markdown = `![${data.filename}](${encodeURI(`/api/local/serve${data.path}`)})`;
        const el = textareaRef.current;
        const start = Math.min(el?.selectionStart ?? Infinity, el?.value.length ?? Infinity);
        const end = Math.min(el?.selectionEnd ?? Infinity, el?.value.length ?? Infinity);
        setValue((prev) => prev.slice(0, Math.min(start, prev.length)) + markdown + prev.slice(Math.min(end, prev.length)));
        requestAnimationFrame(() => {
          const pos = Math.min(start, el?.value.length ?? start) + markdown.length;
          el?.setSelectionRange(pos, pos);
        });
      } catch {
        setError("Network error — upload failed.");
      } finally {
        setUploading(false);
        textareaRef.current?.focus();
      }
    },
    [textareaRef, setValue]
  );

  const onFileInputChange = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      e.target.value = "";
      if (file) void upload(file);
    },
    [upload]
  );

  const onPaste = useCallback(
    (e: ClipboardEvent<HTMLTextAreaElement>) => {
      const item = Array.from(e.clipboardData.items).find((i) => i.type.startsWith("image/"));
      const file = item?.getAsFile();
      if (!file) return;
      e.preventDefault();
      void upload(file);
    },
    [upload]
  );

  return { error, uploading, onFileInputChange, onPaste };
}
