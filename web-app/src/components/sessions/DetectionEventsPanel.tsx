"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { DetectionEventProto } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

interface DetectionEventsPanelProps {
  sessionId: string;
}

/**
 * DetectionEventsPanel renders the last 20 status-detection events for a session.
 * Only shown when ?debug=1 is present in the URL — useful for diagnosing why
 * a session isn't showing the expected status chip.
 */
export function DetectionEventsPanel({ sessionId }: DetectionEventsPanelProps) {
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const [events, setEvents] = useState<DetectionEventProto[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
  }, []);

  const fetchEvents = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      const response = await clientRef.current.getDetectionEvents({
        sessionId,
        limit: 20,
      });
      setEvents(response.events);
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, [sessionId]);

  useEffect(() => {
    void fetchEvents();
    const interval = setInterval(() => void fetchEvents(), 3000);
    return () => clearInterval(interval);
  }, [fetchEvents]);

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
      <h4 style={{ marginBottom: "8px", fontSize: "12px", fontWeight: 600, opacity: 0.7 }}>
        Detection Events (debug)
      </h4>
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
                <td style={{ padding: "3px 6px" }}>{e.resultStatus}</td>
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
    </section>
  );
}
