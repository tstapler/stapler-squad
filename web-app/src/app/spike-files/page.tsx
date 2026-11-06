"use client";
// analytics-exempt
// THROWAWAY (files-viewer-revamp Phase 0 spikes 0.1.4a / 0.1.5 / 0.1.6). Delete after the owner sitting.
// Variants via ?v= : input (default, phone page), stateswap, a11y.
import { useEffect, useMemo, useState } from "react";
import { CmView, genDoc } from "./cm";
import { StateSwap } from "./StateSwap";
import { AccessibleViewSpike } from "./AccessibleViewSpike";
import { KeyCapturePanel } from "./KeyCapturePanel";
import { ViewportReadout } from "./ViewportReadout";

const A11Y_LINES = 3000;

function A11yVariant() {
  const text = useMemo(() => genDoc(A11Y_LINES), []);
  const lines = useMemo(() => text.split("\n"), [text]);
  return (
    <section role="region" aria-label="Editor">
      <a href="#spike-accessible-view" data-testid="spike-skip-link">Switch to accessible view</a>
      <h2>CodeMirror 6 read view</h2>
      <CmView testId="spike-cm-read" doc={text} readOnly height="300px" />
      <h2>Accessible view</h2>
      <AccessibleViewSpike lines={lines} />
    </section>
  );
}

function InputVariant() {
  const doc = useMemo(() => genDoc(5000), []);
  return (
    <div style={{ paddingBottom: 100 }}>
      <h2>Input and viewport spike</h2>
      <p>Tap in the editor so the soft keyboard opens. Check that the GREEN bar stays above the keyboard and the caret stays visible. Type `--` and a double-quoted word, then check for em dash or curly quotes.</p>
      <ViewportReadout />
      <CmView testId="spike-input-editor" doc={doc} hostile height="260px" />
      <KeyCapturePanel />
    </div>
  );
}

export default function SpikeFilesPage() {
  const [variant, setVariant] = useState<string | null>(null);
  useEffect(() => {
    setVariant(new URLSearchParams(window.location.search).get("v") ?? "input");
  }, []);
  if (!variant) return null;
  return (
    <div data-testid={`spike-variant-${variant}`}>
      {variant === "stateswap" && <StateSwap />}
      {variant === "a11y" && <A11yVariant />}
      {variant === "input" && <InputVariant />}
    </div>
  );
}
