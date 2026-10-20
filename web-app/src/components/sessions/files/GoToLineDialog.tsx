"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { panel, header, heading, filter, textButton } from "./OutlinePanel.css";

interface GoToLineDialogProps {
  onSubmit: (line: number) => void;
  onClose: () => void;
}

/** Ctrl+G prompt: a line number (out-of-range values are clamped by the caller). */
export function GoToLineDialog({ onSubmit, onClose }: GoToLineDialogProps) {
  const [value, setValue] = useState("");
  const invoker = useRef<HTMLElement | null>(typeof document === "undefined" ? null : (document.activeElement as HTMLElement | null));
  useEffect(() => {
    const node = invoker.current;
    return () => node?.focus?.();
  }, []);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const line = Number(value.trim().replace(/^:/, ""));
    if (Number.isInteger(line)) onSubmit(line);
  };

  return (
    <form
      className={panel}
      role="dialog"
      aria-label="Go to line"
      onSubmit={submit}
      onKeyDown={(e) => { if (e.key === "Escape") { e.preventDefault(); onClose(); } }}
    >
      <div className={header}>
        <label className={heading} htmlFor="files-goto-line">Go to line</label>
        <input id="files-goto-line" className={filter} inputMode="numeric" autoFocus value={value} onChange={(e) => setValue(e.target.value)} />
        <button type="submit" className={textButton}>Go</button>
      </div>
    </form>
  );
}
