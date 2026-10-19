"use client";
// THROWAWAY (Task 0.1.5a): live visualViewport numbers plus two bottom bars to compare against the soft keyboard.
import { useEffect, useState } from "react";

interface Metrics {
  inner: number;
  vvHeight: number;
  vvOffsetTop: number;
  keyboard: number;
  maxKeyboard: number;
}

export function ViewportReadout() {
  const [m, setM] = useState<Metrics>({ inner: 0, vvHeight: 0, vvOffsetTop: 0, keyboard: 0, maxKeyboard: 0 });

  useEffect(() => {
    const vv = window.visualViewport;
    const read = () => {
      const vvHeight = vv?.height ?? window.innerHeight;
      const vvOffsetTop = vv?.offsetTop ?? 0;
      const keyboard = Math.max(0, Math.round(window.innerHeight - vvHeight - vvOffsetTop));
      setM((p) => ({ inner: window.innerHeight, vvHeight: Math.round(vvHeight), vvOffsetTop: Math.round(vvOffsetTop), keyboard, maxKeyboard: Math.max(p.maxKeyboard, keyboard) }));
    };
    read();
    vv?.addEventListener("resize", read);
    vv?.addEventListener("scroll", read);
    return () => {
      vv?.removeEventListener("resize", read);
      vv?.removeEventListener("scroll", read);
    };
  }, []);

  const bar = { position: "fixed", left: 0, right: 0, height: 44, color: "#fff", display: "flex", alignItems: "center", zIndex: 100000 } as const;
  return (
    <>
      <p data-testid="spike-vv-readout" style={{ fontFamily: "monospace", fontSize: 12 }}>
        {`innerHeight=${m.inner} vv.height=${m.vvHeight} vv.offsetTop=${m.vvOffsetTop} keyboard=${m.keyboard} maxKeyboard=${m.maxKeyboard} size=${typeof window === "undefined" ? "" : `${window.innerWidth}x${window.innerHeight}`}`}
      </p>
      <div data-testid="spike-bar-fixed" style={{ ...bar, bottom: 0, background: "#a00" }}>RED: fixed bottom:0 (layout viewport)</div>
      <div data-testid="spike-bar-pinned" style={{ ...bar, bottom: "var(--keyboard-height, 0px)", background: "#070" }}>GREEN: pinned to visualViewport (--keyboard-height)</div>
    </>
  );
}
