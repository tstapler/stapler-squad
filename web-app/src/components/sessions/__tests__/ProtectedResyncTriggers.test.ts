// Epic 8.2 (terminal-resync-reliability) — regression guard for the 3 pre-existing
// full-resync/refit triggers that `requirements.md`'s Constraints section (and the
// original `terminal-visibility-resync/requirements.md` "Out of scope / must not change"
// section) name as off-limits for this project:
//   1. Mount-time fit call            — XtermTerminal.tsx (double-RAF initial fitAddon.fit())
//   2. Manual Reconnect button handler — TerminalOutput.tsx's handleManualReconnect
//   3. ResizeObserver-driven fit callback — XtermTerminal.tsx's `new ResizeObserver(...)`
//
// Correction vs this epic's task framing: the *original* feature's requirements.md
// (project_plans/terminal-visibility-resync/requirements.md, "Out of scope / must not
// change") places triggers 1 and 3 in XtermTerminal.tsx, not TerminalOutput.tsx — only
// trigger 2 (handleManualReconnect) lives in TerminalOutput.tsx. This test reads both
// files from disk (read-only) rather than relying on a single-file assumption.
//
// This is a static/tripwire check, not a behavioral test: it asserts the pinned source
// snippets are byte-for-byte present, so an accidental edit during Phases 2/6 of this
// project (which touch useVisibilityResync.ts and TerminalOutput.tsx's resync plumbing)
// fails loudly here instead of silently drifting. It intentionally does not import or
// render either component (no xterm/jsdom setup needed), so it stays fast and has no
// dependency on this project's other in-flight changes.
//
// If one of these snippets needs to change for a *legitimate* reason unrelated to this
// project, update the expected snippet here in the same commit and say why in the PR.

import fs from "fs";
import path from "path";

const terminalOutputSrc = fs.readFileSync(
  path.join(__dirname, "../TerminalOutput.tsx"),
  "utf-8",
);
const xtermTerminalSrc = fs.readFileSync(
  path.join(__dirname, "../XtermTerminal.tsx"),
  "utf-8",
);

describe("protected resync triggers", () => {
  it("protectedResyncTriggers_should_RemainByteForByteUnchanged_When_VisibilityAndStaggerFixesLand", () => {
    // Trigger 1: mount-time fit call (XtermTerminal.tsx, double-RAF initial fit).
    // Pinned at web-app/src/components/sessions/XtermTerminal.tsx:640-681 as of this
    // commit (see Epic 8.2.1.1's report for the exact pin date/commit).
    const mountTimeFitSnippet = [
      "      // CRITICAL: Wait for browser to complete layout before fitting",
      "      // Use requestAnimationFrame to ensure DOM is rendered and measurements are accurate",
      "      requestAnimationFrame(() => {",
      "        requestAnimationFrame(() => {",
      "          // Double RAF ensures layout is stable before FitAddon measures dimensions",
    ].join("\n");
    expect(xtermTerminalSrc).toContain(mountTimeFitSnippet);
    expect(xtermTerminalSrc).toContain("          fitAddon.fit();\n          updateScrollbar(terminal);");

    // Trigger 2: manual Reconnect button handler (TerminalOutput.tsx).
    // Pinned at web-app/src/components/sessions/TerminalOutput.tsx:1159-1164.
    const handleManualReconnectSnippet = [
      "  const handleManualReconnect = useCallback(() => {",
      '    console.log("[TerminalOutput] Manual reconnect requested");',
      "    setConnectionAttempts(0);",
      "    setShowReconnectButton(false);",
      "    connect();",
      "  }, [connect]);",
    ].join("\n");
    expect(terminalOutputSrc).toContain(handleManualReconnectSnippet);

    // Trigger 3: ResizeObserver-driven fit callback (XtermTerminal.tsx).
    // Pinned at web-app/src/components/sessions/XtermTerminal.tsx:1040-1084 (the
    // `new ResizeObserver(...)` callback body).
    expect(xtermTerminalSrc).toContain(
      "      const resizeObserver = new ResizeObserver((entries: ResizeObserverEntry[]) => {",
    );
    expect(xtermTerminalSrc).toContain(
      [
        "        if ((widthChanged || heightChanged) && width > 0 && height > 0) {",
        "          lastContainerSize = { width, height };",
      ].join("\n"),
    );
    expect(xtermTerminalSrc).toContain(
      "        } else if ((widthChanged || heightChanged) && (width === 0 || height === 0)) {",
    );
  });
});
