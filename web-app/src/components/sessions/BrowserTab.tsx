'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { createClient } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';
import { SessionService, WriteToSessionRequestSchema } from '@/gen/session/v1/session_pb';
import { getConnectTransport } from '@/lib/api/transport';
import * as styles from './BrowserTab.css';
import CDPViewer from './CDPViewer';
import { type CDPState, CDPStatus } from '@/gen/session/v1/types_pb';

export type { CDPState };
export { CDPStatus };

interface BrowserTabProps {
  sessionId: string;
  /** The application base URL, e.g. window.location.origin + '/api'. Used to derive the WebSocket URL. */
  baseUrl: string;
  isVisible: boolean;
  cdpState: CDPState | undefined;
}

export interface InspectResult {
  html: string;
  notFound?: boolean;
  computedCss?: { name: string; value: string }[];
  screenshotBase64?: string;
}

/** REST URL for the inspect endpoint; baseUrl ends in /api (see BrowserTab props). */
export function buildInspectUrl(baseUrl: string, sessionId: string): string {
  return `${baseUrl.replace(/\/+$/, '')}/sessions/${sessionId}/inspect-element`;
}

// Page content is untrusted: a CR would submit the line and ESC could inject
// terminal escape sequences, so control characters never reach the PTY.
// eslint-disable-next-line no-control-regex
const CONTROL_CHARS = /[\u0000-\u001f\u007f-\u009f]+/g;
function stripControlChars(s: string): string {
  return s.replace(CONTROL_CHARS, ' ');
}

/** Text injected into the terminal by "Send to Agent" (no trailing newline: the user confirms). */
export function formatInspectContext(x: number, y: number, r: InspectResult): string {
  const css = (r.computedCss ?? []).map((p) => `${p.name}: ${p.value}`).join('; ');
  return `[Browser Inspector] Element at (${x}, ${y}):\nHTML: ${stripControlChars(r.html)}\nCSS: ${stripControlChars(css)}`;
}

export function buildWsUrl(baseUrl: string, sessionId: string): string {
  // Resolve protocol-relative (//host/path) and relative (/path) URLs to absolute
  // before doing scheme substitution, so we never produce a broken WebSocket URL.
  let absolute = baseUrl;
  if (absolute.startsWith('//')) {
    // Protocol-relative: prepend the current page protocol.
    absolute = `${window.location.protocol}${absolute}`;
  } else if (absolute.startsWith('/') || (!absolute.startsWith('http://') && !absolute.startsWith('https://') && !absolute.startsWith('ws://') && !absolute.startsWith('wss://'))) {
    // Relative path or no scheme: make it absolute using the current origin.
    absolute = `${window.location.protocol}//${window.location.host}${absolute.startsWith('/') ? '' : '/'}${absolute}`;
  }

  // If already a WebSocket URL, use as-is (no double-substitution).
  if (absolute.startsWith('ws://') || absolute.startsWith('wss://')) {
    const origin = absolute.replace(/\/+$/, '').replace(/\/api$/, '');
    return `${origin}/api/sessions/${sessionId}/cdp-stream`;
  }

  // Normalize: strip trailing slashes, then strip /api suffix if present.
  // Handles cases where baseUrl ends with '/' or lacks the /api suffix,
  // which would otherwise produce a doubled path like wss://host/api/api/sessions/...
  const origin = absolute.replace(/\/+$/, '').replace(/\/api$/, '');
  const wsScheme = origin.startsWith('https') ? 'wss' : 'ws';
  const host = origin.replace(/^https?:\/\//, '');
  return `${wsScheme}://${host}/api/sessions/${sessionId}/cdp-stream`;
}

const NO_FRAME_TIMEOUT_MS = 10_000;

export function BrowserTab({ sessionId, baseUrl, isVisible, cdpState }: BrowserTabProps) {
  const status = cdpState?.status ?? CDPStatus.CDP_STATUS_UNSPECIFIED;
  const isUnavailable = status === CDPStatus.CDP_STATUS_UNAVAILABLE || status === CDPStatus.CDP_STATUS_UNSPECIFIED;
  const isReady = status === CDPStatus.CDP_STATUS_STREAMING;
  const isWaiting = !isUnavailable && !isReady;

  // Track if we've ever been ready — keeps CDPViewer mounted once connected
  // so the WebSocket connection survives tab switches even if isReady temporarily flickers.
  // This avoids a premature WebSocket handshake during WAITING/NO_BROWSER states.
  const hasBeenReadyRef = useRef(false);
  if (isReady) hasBeenReadyRef.current = true;
  const shouldMountViewer = hasBeenReadyRef.current;

  const wsUrl = buildWsUrl(baseUrl, sessionId);

  // Connection state for reconnecting banner and manual reconnect
  const [connectionState, setConnectionState] = useState<'connected' | 'reconnecting' | 'disconnected'>('disconnected');
  // Incrementing key forces CDPViewer to re-mount on manual reconnect
  const [viewerKey, setViewerKey] = useState(0);

  // STREAMING only means Chrome was detected; surface a stalled stream instead of a silent blank canvas.
  const [hasFrame, setHasFrame] = useState(false);
  const [frameTimedOut, setFrameTimedOut] = useState(false);
  useEffect(() => {
    if (!isReady) setHasFrame(false); // a server-side reconnect clears the frame
  }, [isReady]);
  useEffect(() => {
    if (!isReady || hasFrame) {
      setFrameTimedOut(false);
      return;
    }
    const timer = setTimeout(() => setFrameTimedOut(true), NO_FRAME_TIMEOUT_MS);
    return () => clearTimeout(timer);
  }, [isReady, hasFrame, viewerKey]);

  // Inspect mode state. The toggle is only usable while the stream is live (isReady).
  const [inspectMode, setInspectMode] = useState(false);
  const [inspecting, setInspecting] = useState(false);
  const [inspectError, setInspectError] = useState<string | null>(null);
  const [inspectResult, setInspectResult] = useState<{ x: number; y: number; data: InspectResult } | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const closeInspector = useCallback(() => {
    abortRef.current?.abort();
    setInspectMode(false);
    setInspecting(false);
    setInspectError(null);
    setInspectResult(null);
  }, []);

  useEffect(() => () => abortRef.current?.abort(), []);

  // Stream dropped while inspecting: leave inspect mode.
  useEffect(() => {
    if (!isReady) closeInspector();
  }, [isReady, closeInspector]);

  useEffect(() => {
    if (!inspectMode && !inspectResult) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeInspector();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [inspectMode, inspectResult, closeInspector]);

  async function handleInspectClick(x: number, y: number) {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setInspecting(true);
    setInspectError(null);
    try {
      const res = await fetch(buildInspectUrl(baseUrl, sessionId), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ x, y }),
        signal: controller.signal,
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body?.error ?? `Inspection failed (${res.status})`);
      setInspectResult({ x, y, data: body as InspectResult });
    } catch (err) {
      if (controller.signal.aborted) return;
      setInspectError(err instanceof Error ? err.message : 'Inspection failed');
    } finally {
      if (abortRef.current === controller) setInspecting(false);
    }
  }

  async function handleSendToAgent() {
    if (!inspectResult) return;
    try {
      const client = createClient(SessionService, getConnectTransport());
      await client.writeToSession(create(WriteToSessionRequestSchema, {
        sessionId,
        input: formatInspectContext(inspectResult.x, inspectResult.y, inspectResult.data),
        pressEnter: false,
      }));
    } catch (err) {
      setInspectError(err instanceof Error ? err.message : 'Failed to send to agent');
    }
  }

  function handleReconnect() {
    setConnectionState('disconnected');
    setHasFrame(false);
    setFrameTimedOut(false);
    setViewerKey((k) => k + 1);
  }

  return (
    <div className={styles.browserTabContainer}>
      <div className={styles.toolbar}>
        <button
          type="button"
          className={styles.inspectButton}
          data-testid="inspect-toggle"
          aria-pressed={inspectMode}
          disabled={!isReady}
          title={isReady ? 'Pick an element to send to the agent (Esc to exit)' : 'Inspect is available once the browser stream is live'}
          onClick={() => (inspectMode ? closeInspector() : setInspectMode(true))}
        >
          Inspect
        </button>
        {inspecting && <span role="status">Inspecting…</span>}
        {inspectError && <span role="alert">{inspectError}</span>}
      </div>

      {/* Viewer area */}
      <div className={styles.viewerArea}>
        {isUnavailable && (
          <div className={styles.placeholderOverlay}>
            <span>Browser passthrough unavailable on this host</span>
            <span style={{ fontSize: '0.85em', opacity: 0.7 }}>
              Requires a running Chrome/Chromium instance with CDP enabled
            </span>
          </div>
        )}

        {isWaiting && !isUnavailable && (
          <div className={styles.placeholderOverlay}>
            {status === CDPStatus.CDP_STATUS_WAITING ? (
              <span>Connecting to browser...</span>
            ) : (
              <>
                <span>No browser detected</span>
                <span style={{ fontSize: '0.85em', opacity: 0.7 }}>
                  Run Chrome or Chromium in this session to see it here
                </span>
              </>
            )}
          </div>
        )}

        {/* Mount CDPViewer only once isReady has been true — prevents premature
            WebSocket handshake during WAITING/NO_BROWSER states before the CDP
            endpoint is available. Once mounted, keep it mounted (hasBeenReadyRef)
            so the connection survives tab switches. */}
        {shouldMountViewer && (
          <div
            className={styles.canvasWrapper}
            style={{
              visibility: isReady ? 'visible' : 'hidden',
              pointerEvents: isReady ? 'auto' : 'none',
            }}
          >
            {connectionState === 'reconnecting' && (
              <div className={styles.reconnectingBanner}>
                <span>Reconnecting…</span>
                <button className={styles.reconnectButton} onClick={handleReconnect}>
                  Reconnect
                </button>
              </div>
            )}
            {isReady && !hasFrame && (
              <div className={styles.placeholderOverlay} role="status" data-testid="waiting-first-frame">
                <span>{frameTimedOut ? 'No frames received from the browser' : 'Waiting for first frame…'}</span>
                {frameTimedOut && (
                  <span style={{ fontSize: '0.85em', opacity: 0.7 }}>
                    Chrome was detected but is not streaming. Try Reconnect, or check the server logs.
                  </span>
                )}
              </div>
            )}
            <CDPViewer
              key={viewerKey}
              wsUrl={wsUrl}
              isVisible={isVisible && isReady}
              onConnected={() => setConnectionState('connected')}
              onDisconnected={() => setConnectionState('reconnecting')}
              onFrame={() => setHasFrame(true)}
              inspectMode={inspectMode}
              onInspectClick={handleInspectClick}
            />
          </div>
        )}

        {inspectResult && (
          <div className={styles.resultPanel} data-testid="inspect-result-panel" role="dialog" aria-label="Inspected element">
            <div className={styles.resultHeader}>
              <strong>Element at ({inspectResult.x}, {inspectResult.y})</strong>
              <button type="button" className={styles.inspectButton} onClick={closeInspector}>✕ Close</button>
            </div>
            {inspectResult.data.notFound ? (
              <span>No element found at this position.</span>
            ) : (
              <>
                {inspectResult.data.screenshotBase64 ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img className={styles.resultScreenshot} alt="Element screenshot" src={`data:image/jpeg;base64,${inspectResult.data.screenshotBase64}`} />
                ) : (
                  <span>Screenshot unavailable</span>
                )}
                <pre className={styles.resultCode}><code>{inspectResult.data.html}</code></pre>
                <table className={styles.cssTable}>
                  <tbody>
                    {(inspectResult.data.computedCss ?? []).map((p) => (
                      <tr key={p.name}><td>{p.name}</td><td>{p.value}</td></tr>
                    ))}
                  </tbody>
                </table>
                <button type="button" className={styles.inspectButton} onClick={handleSendToAgent}>Send to Agent</button>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
