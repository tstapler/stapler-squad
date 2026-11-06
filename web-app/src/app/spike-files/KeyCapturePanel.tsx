"use client";
// THROWAWAY (Task 0.1.5c): on-screen keydown capture; never sent anywhere. Removed with the page.
import { useEffect, useState } from "react";

export interface CapturedKey {
  key: string;
  code: string;
  keyCode: number;
  isComposing: boolean;
  isTrusted: boolean;
  // Beyond the plan's field list: S2 (Ctrl/Meta chords) cannot be evaluated without the modifier flags.
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}

export function KeyCapturePanel() {
  const [events, setEvents] = useState<CapturedKey[]>([]);
  const [copied, setCopied] = useState("");

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const { key, code, keyCode, isComposing, isTrusted, ctrlKey, metaKey, altKey, shiftKey } = e;
      setEvents((prev) => [...prev, { key, code, keyCode, isComposing, isTrusted, ctrlKey, metaKey, altKey, shiftKey }].slice(-300));
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, []);

  const json = JSON.stringify(events, null, 1);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(json);
      setCopied("Copied");
    } catch {
      setCopied("Clipboard blocked: long-press the JSON box below and copy");
    }
  };

  return (
    <section aria-label="Key event capture" data-testid="spike-key-panel">
      <p>Type below or in the editor. Each keydown is listed here and stays on this screen.</p>
      <input data-testid="spike-plain-input" aria-label="Plain text input" style={{ width: "100%", minHeight: 44 }} />
      <button type="button" style={{ minHeight: 44 }} data-testid="spike-copy-json" onClick={copy}>Copy as JSON</button>
      <button type="button" style={{ minHeight: 44 }} onClick={() => setEvents([])}>Clear</button>
      <span role="status">{copied}</span>
      <p data-testid="spike-key-count">{events.length} keydown events</p>
      <ol style={{ fontFamily: "monospace", fontSize: 12, maxHeight: 160, overflow: "auto" }}>
        {events.map((e, i) => (
          <li key={i}>{`key=${JSON.stringify(e.key)} code=${e.code} keyCode=${e.keyCode} isComposing=${e.isComposing} isTrusted=${e.isTrusted} ctrl=${e.ctrlKey} meta=${e.metaKey}`}</li>
        ))}
      </ol>
      <textarea readOnly aria-label="Captured events as JSON" value={json} rows={4} style={{ width: "100%" }} />
    </section>
  );
}
