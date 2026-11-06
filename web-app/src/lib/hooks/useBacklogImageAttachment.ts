"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { ClipboardEvent, ChangeEvent, RefObject } from "react";
import { getApiBaseUrl } from "@/lib/config";

export const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024; // 10 MB — matches server-side cap
const TOO_LARGE_MESSAGE = "Image is too large (max 10 MB).";

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
  const [pending, setPending] = useState(0);
  const isMounted = useRef(true);
  useEffect(() => {
    isMounted.current = true;
    return () => {
      isMounted.current = false;
    };
  }, []);

  const upload = useCallback(
    async (file: File) => {
      if (!file.type.startsWith("image/")) {
        setError("Only image files can be attached.");
        return;
      }
      if (file.size > MAX_ATTACHMENT_BYTES) {
        setError(TOO_LARGE_MESSAGE);
        return;
      }
      setError(null);
      setPending((n) => n + 1);
      try {
        const formData = new FormData();
        formData.append("file", file);
        const resp = await fetch(`${getApiBaseUrl()}/v1/upload-backlog-attachment`, {
          method: "POST",
          body: formData,
        });
        if (!resp.ok) {
          if (!isMounted.current) return;
          setError(
            resp.status === 413
              ? TOO_LARGE_MESSAGE
              : resp.status === 415
                ? "Unsupported image type — use PNG, JPEG, GIF, or WebP."
                : "Upload failed."
          );
          return;
        }
        const data = (await resp.json()) as { path: string; filename: string };
        if (!isMounted.current) return;
        const markdown = `![${data.filename}](${encodeURI(`/api/local/serve${data.path}`)})`;
        const el = textareaRef.current;
        const start = Math.min(el?.selectionStart ?? Infinity, el?.value.length ?? Infinity);
        const end = Math.min(el?.selectionEnd ?? Infinity, el?.value.length ?? Infinity);
        setValue((prev) => prev.slice(0, Math.min(start, prev.length)) + markdown + prev.slice(Math.min(end, prev.length)));
        requestAnimationFrame(() => {
          const pos = Math.min(start, el?.value.length ?? start) + markdown.length;
          el?.setSelectionRange(pos, pos);
        });
        el?.focus();
      } catch {
        if (isMounted.current) setError("Network error — upload failed.");
      } finally {
        if (isMounted.current) setPending((n) => n - 1);
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

  return { error, uploading: pending > 0, onFileInputChange, onPaste };
}
