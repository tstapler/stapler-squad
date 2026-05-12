"use client";
// analytics-exempt

/**
 * Layout overlap test page.
 *
 * Renders a realistic header + session-detail modal shell (no real backend)
 * so Playwright tests can measure element positions without needing a live
 * server connection.
 *
 * Routes:
 *   /test/layout-overlap              — sessions page modal
 *   /test/layout-overlap?mode=rq      — review-queue page modal
 */

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

const HEADER_HEIGHT = "4rem"; // mirrors --header-height in globals.css

function SessionDetailShell() {
  return (
    <div
      data-testid="session-detail"
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100%",
        background: "#1e1e1e",
      }}
    >
      {/* Session header */}
      <div
        data-testid="session-header"
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "1.5rem",
          borderBottom: "1px solid #3e3e42",
          background: "#2d2d30",
          flexShrink: 0,
        }}
      >
        <span style={{ color: "#fff", fontWeight: 600 }}>test-session</span>
        <button data-testid="close-btn" style={{ background: "none", border: "none", color: "#9ca3af", cursor: "pointer" }}>✕</button>
      </div>

      {/* Tabs — always visible */}
      <div
        data-testid="session-tabs"
        style={{
          display: "flex",
          borderBottom: "1px solid #3e3e42",
          background: "#252526",
          padding: "0 1rem",
          gap: "0.5rem",
          flexShrink: 0,
        }}
      >
        {["Terminal", "Diff", "VCS", "Logs", "Info"].map((tab) => (
          <button
            key={tab}
            data-testid={`tab-${tab.toLowerCase()}`}
            style={{
              padding: "1rem 1.5rem",
              border: "none",
              background: "transparent",
              color: tab === "Terminal" ? "#0070f3" : "#9ca3af",
              fontSize: "0.95rem",
              cursor: "pointer",
              borderBottom: tab === "Terminal" ? "2px solid #0070f3" : "2px solid transparent",
            }}
          >
            {tab}
          </button>
        ))}
      </div>

      {/* Terminal toolbar */}
      <div
        data-testid="terminal-toolbar"
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "0.75rem 1rem",
          background: "#2d2d30",
          borderBottom: "1px solid #3e3e42",
          flexShrink: 0,
        }}
      >
        <span style={{ color: "#cccccc", fontSize: 14 }}>● Connected</span>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          {["Debug", "Record", "Raw", "Resize", "Clear", "Bottom", "Copy"].map((btn) => (
            <button
              key={btn}
              data-testid={`toolbar-btn-${btn.toLowerCase()}`}
              style={{ padding: "0.4rem 0.65rem", background: "#3e3e42", border: "1px solid #555", borderRadius: 4, color: "#ccc", fontSize: 13, cursor: "pointer" }}
            >
              {btn}
            </button>
          ))}
        </div>
      </div>

      {/* Terminal content */}
      <div
        data-testid="mock-terminal"
        style={{ flex: 1, background: "#1e1e1e", color: "#d4d4d4", minHeight: 0, padding: "0.75rem 1rem", fontFamily: "monospace", fontSize: 13 }}
      >
        <span>$ Mock terminal content — top anchor</span>
      </div>
    </div>
  );
}

function LayoutOverlapContent() {
  const searchParams = useSearchParams();
  const mode = searchParams.get("mode") ?? "sessions";
  const isReviewQueue = mode === "rq";

  const modalStyle: React.CSSProperties = isReviewQueue
    ? {
        position: "fixed",
        top: 0, left: 0, right: 0, bottom: 0,
        background: "rgba(0,0,0,0.8)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: "2rem",
        paddingTop: HEADER_HEIGHT,
        zIndex: 1000,
      }
    : {
        position: "fixed",
        top: 0, left: 0, right: 0, bottom: 0,
        background: "rgba(0,0,0,0.7)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: "1rem",
        paddingTop: `calc(${HEADER_HEIGHT} + 0.5rem)`,
        zIndex: 1000,
      };

  const modalContentStyle: React.CSSProperties = isReviewQueue
    ? {
        maxWidth: 1400,
        width: "98vw",
        maxHeight: `calc(100vh - ${HEADER_HEIGHT} - 2rem)`,
        height: `calc(100vh - ${HEADER_HEIGHT} - 2rem)`,
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
        background: "#1a1a1a",
        borderRadius: 8,
        border: "1px solid #333",
      }
    : {
        maxWidth: 1400,
        width: "98vw",
        maxHeight: `calc(100vh - ${HEADER_HEIGHT} - 1.5rem)`,
        height: `calc(100vh - ${HEADER_HEIGHT} - 1.5rem)`,
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
        background: "#1a1a1a",
        borderRadius: 8,
      };

  return (
    <div style={{ minHeight: "100vh" }}>
      <header
        data-testid="app-header"
        style={{
          background: "rgba(26,26,26,0.95)",
          borderBottom: "1px solid #333",
          padding: "0 1rem",
          position: "sticky",
          top: 0,
          zIndex: 1100,
          height: HEADER_HEIGHT,
          display: "flex",
          alignItems: "center",
        }}
      >
        <strong style={{ color: "#fff" }}>Stapler Squad</strong>
        <span style={{ marginLeft: "0.5rem", color: "#888", fontSize: 14 }}>Layout Overlap Test — mode: {mode}</span>
      </header>

      <main data-testid="page-body" style={{ padding: "2rem", color: "#ccc" }}>
        <p>This page renders the header + modal overlay so Playwright can measure positions.</p>
      </main>

      <div data-testid="modal-overlay" style={modalStyle}>
        <div data-testid="modal-content" style={modalContentStyle}>
          <SessionDetailShell />
        </div>
      </div>
    </div>
  );
}

export default function LayoutOverlapPage() {
  return (
    <Suspense>
      <LayoutOverlapContent />
    </Suspense>
  );
}
