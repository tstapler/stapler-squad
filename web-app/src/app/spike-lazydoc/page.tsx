"use client";
// analytics-exempt
// THROWAWAY (Task 0.1.4c, ADR-036 S-T3): phone and Playwright page for the lazy-document models.
// ?m=a2 (gap-line, default) or ?m=a1 (window-swap). Delete after the owner sitting (Task 0.1.4d).
import { useCallback, useEffect, useState } from "react";
import { A2Host } from "./A2Host";
import { A1Host } from "./A1Host";

export default function SpikeLazyDocPage() {
  const [model, setModel] = useState<string | null>(null);
  const [stats, setStats] = useState("");
  const [goto, setGoto] = useState("500000");
  useEffect(() => {
    setModel(new URLSearchParams(window.location.search).get("m") ?? "a2");
  }, []);
  const onStats = useCallback((s: string) => setStats(s), []);
  if (!model) return null;
  const jump = () => {
    const api = (window as unknown as { __spike?: { api: { scrollToAbs(n: number): void } } }).__spike?.api;
    api?.scrollToAbs(Number.parseInt(goto, 10));
  };
  return (
    <div data-testid={`lazydoc-model-${model}`}>
      <h2>Lazy document spike: {model === "a2" ? "a2 gap-line" : "a1 window-swap"} (1,000,000 lines, about 50 MB virtual)</h2>
      <p>
        <a href="?m=a2">a2</a> | <a href="?m=a1">a1</a>. Scroll fast, jump to a far line, then tap and type. Record PASS or FAIL for this model.
      </p>
      <label>
        Go to line <input type="number" inputMode="numeric" value={goto} onChange={(e) => setGoto(e.target.value)} style={{ minHeight: 44 }} />
      </label>
      <button type="button" style={{ minHeight: 44 }} onClick={jump}>Go</button>
      <p data-testid="lazydoc-stats" style={{ fontFamily: "monospace", fontSize: 12 }}>{stats}</p>
      {model === "a1" ? <A1Host onStats={onStats} /> : <A2Host onStats={onStats} />}
    </div>
  );
}
