"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { DetectionEventProto } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

const pad = (n: number) => String(n).padStart(2, "0");

// Maps the DetectedStatus int value (from proto result_status) to the Go constant name.
// Mirrors the iota order in session/detection/detector.go.
const STATUS_INT_TO_GO: Record<number, string> = {
  0: "StatusUnknown",
  1: "StatusReady",
  2: "StatusProcessing",
  3: "StatusNeedsApproval",
  4: "StatusInputRequired",
  5: "StatusError",
  6: "StatusTestsFailing",
  7: "StatusIdle",
  8: "StatusActive",
  9: "StatusSuccess",
};

interface CaptureResult {
  content: string;
  statusInt: number;
  patternName: string;
  suggestedFilename: string;
}

interface DetectionEventsPanelProps {
  sessionId: string;
  program?: string;
}

/**
 * DetectionEventsPanel renders the last 20 status-detection events for a session.
 * Only shown when ?debug=1 is present in the URL — useful for diagnosing why
 * a session isn't showing the expected status chip.
 *
 * The "Capture Test Case" button fetches the current terminal content and
 * generates a ready-to-paste testdata fixture + snapshot_test.go snippet.
 */
export function DetectionEventsPanel({ sessionId, program = "claude" }: DetectionEventsPanelProps) {
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const [events, setEvents] = useState<DetectionEventProto[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [capturing, setCapturing] = useState(false);
  const [captureResult, setCaptureResult] = useState<CaptureResult | null>(null);
  const [copiedField, setCopiedField] = useState<"content" | "snippet" | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
  }, []);

  useEffect(() => {
    if (!clientRef.current) return;
    let cancelled = false;
    const run = async () => {
      if (!clientRef.current) return;
      try {
        const response = await clientRef.current.getDetectionEvents({ sessionId, limit: 20 });
        if (!cancelled) { setEvents(response.events); setError(null); }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    };
    void run();
    const id = setInterval(() => void run(), 3000);
    return () => { cancelled = true; clearInterval(id); };
  }, [sessionId]);

  const handleCapture = useCallback(async () => {
    if (!clientRef.current) return;
    setCapturing(true);
    setCaptureResult(null);
    try {
      const resp = await clientRef.current.getTerminalSnapshot({ sessionId, lastNLines: 120 });
      const latestEvent = events[0];
      const statusInt = latestEvent?.resultStatus ?? 0;
      const patternName = latestEvent?.matchedPattern ?? "<none>";

      // Suggest a filename based on program + status + timestamp
      const statusLabel = (STATUS_INT_TO_GO[statusInt] ?? "unknown")
        .replace("Status", "")
        .toLowerCase();
      const ts = new Date();
      const stamp = `${ts.getFullYear()}${pad(ts.getMonth() + 1)}${pad(ts.getDate())}_${pad(ts.getHours())}${pad(ts.getMinutes())}${pad(ts.getSeconds())}`;
      const suggestedFilename = `${program}_${statusLabel}_${stamp}.txt`;

      setCaptureResult({ content: resp.content, statusInt, patternName, suggestedFilename });
    } catch (e) {
      setError(`Capture failed: ${String(e)}`);
    } finally {
      setCapturing(false);
    }
  }, [sessionId, events, program]);

  const copyToClipboard = useCallback((text: string, field: "content" | "snippet") => {
    void navigator.clipboard.writeText(text).then(() => {
      setCopiedField(field);
      setTimeout(() => setCopiedField(null), 2000);
    });
  }, []);

  const goStatusName = captureResult ? (STATUS_INT_TO_GO[captureResult.statusInt] ?? "StatusUnknown") : "";

  const goSnippet = captureResult
    ? `{\n\tfixture:     "${captureResult.suggestedFilename}",\n\texpected:    ${goStatusName},\n\tprogram:     "${program}",\n\tdescription: "FILL IN: describe what state the session is in",\n},`
    : "";

  return (
    <section
      style={{
        marginTop: "16px",
        fontFamily: "monospace",
        fontSize: "11px",
        border: "1px solid var(--border-color)",
        borderRadius: "6px",
        padding: "12px",
        background: "var(--card-background)",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "8px" }}>
        <h4 style={{ fontSize: "12px", fontWeight: 600, opacity: 0.7, margin: 0 }}>
          Detection Events (debug)
        </h4>
        <button
          onClick={() => void handleCapture()}
          disabled={capturing}
          style={{
            fontSize: "11px",
            padding: "3px 8px",
            borderRadius: "4px",
            border: "1px solid var(--border-color)",
            background: "var(--card-background)",
            color: "var(--text-primary)",
            cursor: capturing ? "not-allowed" : "pointer",
            opacity: capturing ? 0.5 : 1,
          }}
        >
          {capturing ? "Capturing…" : "📸 Capture test case"}
        </button>
      </div>

      {error && (
        <p style={{ color: "var(--error)", padding: "4px 0" }}>Error: {error}</p>
      )}

      {events.length === 0 && !error ? (
        <p style={{ color: "var(--text-secondary)", opacity: 0.6 }}>No events yet — detection runs on a polling interval.</p>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ opacity: 0.6 }}>
              <th style={{ textAlign: "left", padding: "3px 6px", fontWeight: 500 }}>Time</th>
              <th style={{ textAlign: "left", padding: "3px 6px", fontWeight: 500 }}>Pattern</th>
              <th style={{ textAlign: "left", padding: "3px 6px", fontWeight: 500 }}>Category</th>
              <th style={{ textAlign: "left", padding: "3px 6px", fontWeight: 500 }}>Status</th>
              <th style={{ textAlign: "left", padding: "3px 6px", fontWeight: 500 }}>Snippet</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e, i) => (
              <tr key={i} style={{ borderTop: "1px solid var(--border-color)" }}>
                <td style={{ padding: "3px 6px", whiteSpace: "nowrap", opacity: 0.7 }}>
                  {e.timestamp
                    ? new Date(Number(e.timestamp.seconds) * 1000).toLocaleTimeString()
                    : "—"}
                </td>
                <td style={{ padding: "3px 6px" }}>{e.matchedPattern}</td>
                <td style={{ padding: "3px 6px", opacity: 0.8 }}>{e.matchedCategory}</td>
                <td style={{ padding: "3px 6px" }}>{STATUS_INT_TO_GO[e.resultStatus] ?? e.resultStatus}</td>
                <td
                  style={{
                    padding: "3px 6px",
                    maxWidth: "260px",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    opacity: 0.6,
                  }}
                  title={e.tailSnippet}
                >
                  {e.tailSnippet.slice(0, 80)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {captureResult && (
        <div
          style={{
            marginTop: "12px",
            borderTop: "1px solid var(--border-color)",
            paddingTop: "12px",
          }}
        >
          <div style={{ marginBottom: "8px", opacity: 0.8 }}>
            <strong>Captured:</strong>{" "}
            {goStatusName} (pattern: <code>{captureResult.patternName}</code>)
          </div>

          {/* Step 1: terminal content */}
          <div style={{ marginBottom: "10px" }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "4px" }}>
              <span style={{ opacity: 0.7 }}>
                1. Save as{" "}
                <code style={{ background: "var(--hover-background)", padding: "1px 4px", borderRadius: "3px" }}>
                  session/detection/testdata/{captureResult.suggestedFilename}
                </code>
              </span>
              <button
                onClick={() => copyToClipboard(captureResult.content, "content")}
                style={copyBtnStyle}
              >
                {copiedField === "content" ? "Copied!" : "Copy"}
              </button>
            </div>
            <pre
              style={{
                margin: 0,
                padding: "8px",
                background: "var(--terminal-background, #1a1a1a)",
                color: "var(--terminal-foreground, #d4d4d4)",
                borderRadius: "4px",
                fontSize: "10px",
                maxHeight: "180px",
                overflowY: "auto",
                whiteSpace: "pre-wrap",
                wordBreak: "break-all",
              }}
            >
              {captureResult.content}
            </pre>
          </div>

          {/* Step 2: Go snippet */}
          <div>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "4px" }}>
              <span style={{ opacity: 0.7 }}>
                2. Add to{" "}
                <code style={{ background: "var(--hover-background)", padding: "1px 4px", borderRadius: "3px" }}>
                  session/detection/snapshot_test.go
                </code>
              </span>
              <button
                onClick={() => copyToClipboard(goSnippet, "snippet")}
                style={copyBtnStyle}
              >
                {copiedField === "snippet" ? "Copied!" : "Copy"}
              </button>
            </div>
            <pre
              style={{
                margin: 0,
                padding: "8px",
                background: "var(--hover-background)",
                borderRadius: "4px",
                fontSize: "10px",
                whiteSpace: "pre",
              }}
            >
              {goSnippet}
            </pre>
          </div>
        </div>
      )}
    </section>
  );
}

const copyBtnStyle: React.CSSProperties = {
  fontSize: "10px",
  padding: "2px 6px",
  borderRadius: "3px",
  border: "1px solid var(--border-color)",
  background: "var(--card-background)",
  color: "var(--text-primary)",
  cursor: "pointer",
  flexShrink: 0,
};
