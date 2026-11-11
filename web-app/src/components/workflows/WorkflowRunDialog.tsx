"use client";
// +feature: workflows-management

import { useCallback, useEffect, useRef, useState } from "react";
import type { ChangeEvent, DragEvent } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import * as styles from "./WorkflowRunDialog.css";

export const MAX_WORKFLOW_ATTACHMENTS = 10; // matches the server-side cap
const MAX_IMAGE_BYTES = 10 * 1024 * 1024; // matches the upload handler's cap
const INPUT_PLACEHOLDER = "{{input}}";

/** True when the workflow's prompt has an {{input}} slot, so running it needs a dialog. */
export function workflowTakesInput(wf: { command: string; inputTemplate: string }): boolean {
  return wf.command.includes(INPUT_PLACEHOLDER) || wf.inputTemplate.includes(INPUT_PLACEHOLDER);
}

interface Attachment {
  key: number;
  name: string;
  previewUrl: string | null;
  status: "uploading" | "done" | "error";
  path?: string;
  error?: string;
}

function uploadErrorMessage(status: number): string {
  if (status === 413) return "Image is too large (max 10 MB).";
  if (status === 415) return "Unsupported image type — use PNG, JPEG, GIF, or WebP.";
  return "Upload failed.";
}

function makePreviewUrl(file: File): string | null {
  return typeof URL.createObjectURL === "function" ? URL.createObjectURL(file) : null;
}

async function uploadWorkflowImage(file: File): Promise<{ path: string } | { error: string }> {
  try {
    const formData = new FormData();
    formData.append("file", file);
    const resp = await fetch(`${getApiBaseUrl()}/v1/upload-workflow-attachment`, {
      method: "POST",
      body: formData,
    });
    if (!resp.ok) return { error: uploadErrorMessage(resp.status) };
    const data = (await resp.json()) as { path: string };
    return { path: data.path };
  } catch {
    return { error: "Network error — upload failed." };
  }
}

interface Props {
  workflowName: string;
  /** Resolves true when the run started (the dialog should close). */
  onRun: (arg: string, attachmentPaths: string[]) => Promise<boolean>;
  onCancel: () => void;
}

export function WorkflowRunDialog({ workflowName, onRun, onCancel }: Props) {
  const { track } = useAnalytics();
  const [arg, setArg] = useState("");
  const [items, setItems] = useState<Attachment[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const [running, setRunning] = useState(false);
  const nextKey = useRef(0);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const itemsRef = useRef<Attachment[]>([]);
  itemsRef.current = items;
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      itemsRef.current.forEach((i) => i.previewUrl && URL.revokeObjectURL?.(i.previewUrl));
    };
  }, []);

  const patch = useCallback((key: number, change: Partial<Attachment>) => {
    if (!mounted.current) return;
    setItems((prev) => prev.map((i) => (i.key === key ? { ...i, ...change } : i)));
  }, []);

  const addFiles = useCallback(
    (files: File[]) => {
      setNotice(null);
      const slots = MAX_WORKFLOW_ATTACHMENTS - itemsRef.current.filter((i) => i.status !== "error").length;
      const accepted: File[] = [];
      let rejected = 0;
      for (const f of files) {
        if (!f.type.startsWith("image/") || f.size > MAX_IMAGE_BYTES || accepted.length >= slots) rejected++;
        else accepted.push(f);
      }
      if (rejected > 0) {
        setNotice(
          `${rejected} file${rejected === 1 ? " was" : "s were"} skipped: images only, max 10 MB each, up to ${MAX_WORKFLOW_ATTACHMENTS} total.`
        );
      }
      const added: Attachment[] = accepted.map((f) => ({
        key: nextKey.current++,
        name: f.name,
        previewUrl: makePreviewUrl(f),
        status: "uploading",
      }));
      if (added.length === 0) return;
      itemsRef.current = [...itemsRef.current, ...added];
      setItems(itemsRef.current);
      accepted.forEach((file, idx) => {
        void uploadWorkflowImage(file).then((result) =>
          patch(
            added[idx].key,
            "path" in result ? { status: "done", path: result.path } : { status: "error", error: result.error }
          )
        );
      });
    },
    [patch]
  );

  function onFileInputChange(e: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? []);
    e.target.value = "";
    addFiles(files);
  }

  function onDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    setDragging(false);
    addFiles(Array.from(e.dataTransfer.files));
  }

  function remove(key: number) {
    const gone = itemsRef.current.find((i) => i.key === key);
    if (gone?.previewUrl) URL.revokeObjectURL?.(gone.previewUrl);
    itemsRef.current = itemsRef.current.filter((i) => i.key !== key);
    setItems(itemsRef.current);
    setNotice(null);
  }

  const uploading = items.some((i) => i.status === "uploading");
  const paths = items.flatMap((i) => (i.status === "done" && i.path ? [i.path] : []));

  async function submit() {
    setRunning(true);
    track({ name: "workflow_run_with_input", category: "user_action" });
    const started = await onRun(arg, paths);
    if (mounted.current && !started) setRunning(false);
  }

  return (
    <div className={styles.dialog} role="dialog" aria-modal="true" aria-labelledby="workflow-run-title" data-testid="workflow-run-dialog">
      <h2 id="workflow-run-title" className={styles.heading}>
        Run {workflowName}
      </h2>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="workflow-run-input">
          Input
        </label>
        <textarea
          id="workflow-run-input"
          className={styles.textarea}
          value={arg}
          onChange={(e) => setArg(e.target.value)}
          placeholder="Describe what this run should work on"
          data-testid="workflow-run-input"
        />
      </div>

      <div className={styles.fieldGroup}>
        <span className={styles.label} id="workflow-run-images-label">
          Images (optional)
        </span>
        <div
          className={`${styles.dropZone} ${dragging ? styles.dropZoneActive : ""}`}
          aria-labelledby="workflow-run-images-label"
          data-testid="workflow-run-dropzone"
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
        >
          <span>Drop photos or drawings here, or</span>
          <button
            type="button"
            className={styles.secondaryButton}
            data-testid="workflow-run-add-images"
            disabled={items.filter((i) => i.status !== "error").length >= MAX_WORKFLOW_ATTACHMENTS}
            onClick={() => {
              track({ name: "workflow_run_add_images", category: "user_action" });
              fileInputRef.current?.click();
            }}
          >
            Choose images
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            multiple
            className={styles.hiddenInput}
            data-testid="workflow-run-file-input"
            onChange={onFileInputChange}
          />
        </div>
        <p className={styles.hint}>
          Up to {MAX_WORKFLOW_ATTACHMENTS} images, 10 MB each. The agent reads them before it starts.
        </p>
        {notice && (
          <p className={styles.errorText} role="alert" data-testid="workflow-run-notice">
            {notice}
          </p>
        )}
        {items.length > 0 && (
          <ul className={styles.thumbs} data-testid="workflow-run-thumbs">
            {items.map((i) => (
              <li key={i.key} className={styles.thumb} data-testid="workflow-run-thumb" data-status={i.status}>
                {i.previewUrl ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={i.previewUrl} alt={i.name} className={`${styles.thumbImage} ${i.status === "uploading" ? styles.thumbDim : ""}`} />
                ) : (
                  <div className={styles.thumbImage} />
                )}
                <span className={styles.thumbName}>{i.name}</span>
                {i.status === "uploading" && <span>Uploading…</span>}
                {i.status === "error" && (
                  <span className={styles.thumbError} role="alert">
                    {i.error}
                  </span>
                )}
                <button
                  type="button"
                  className={styles.removeButton}
                  aria-label={`Remove ${i.name}`}
                  onClick={() => {
                    track({ name: "workflow_run_remove_image", category: "user_action" });
                    remove(i.key);
                  }}
                >
                  <span className={styles.removeGlyph} aria-hidden="true">
                    ×
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className={styles.actions}>
        <button
          type="button"
          className={styles.secondaryButton}
          disabled={running}
          onClick={() => {
            track({ name: "workflow_run_cancelled", category: "user_action" });
            onCancel();
          }}
        >
          Cancel
        </button>
        <button
          type="button"
          className={styles.primaryButton}
          data-testid="workflow-run-submit"
          disabled={uploading || running}
          onClick={() => void submit()}
        >
          {running ? "Starting…" : uploading ? "Uploading…" : "Run"}
        </button>
      </div>
    </div>
  );
}
