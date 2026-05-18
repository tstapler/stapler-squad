'use client';
import { useRef, useState } from 'react';
import * as styles from './BrowserTab.css';
import CDPViewer from './CDPViewer';
import { VNCState, VNCStatus } from '@/gen/session/v1/types_pb';

export type { VNCState };
export { VNCStatus };

interface BrowserTabProps {
  sessionId: string;
  /** The application base URL, e.g. window.location.origin + '/api'. Used to derive the WebSocket URL. */
  baseUrl: string;
  isVisible: boolean;
  vncState: VNCState | undefined;
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

export function BrowserTab({ sessionId, baseUrl, isVisible, vncState }: BrowserTabProps) {
  const status = vncState?.status ?? VNCStatus.VNC_STATUS_UNSPECIFIED;
  const isUnavailable = status === VNCStatus.VNC_STATUS_UNAVAILABLE || status === VNCStatus.VNC_STATUS_UNSPECIFIED;
  const isWaiting = !isUnavailable && (
    status === VNCStatus.VNC_STATUS_STARTING ||
    status === VNCStatus.VNC_STATUS_NO_BROWSER ||
    !vncState?.browserWindowDetected
  );
  const isReady = status === VNCStatus.VNC_STATUS_READY && vncState?.browserWindowDetected === true;

  // Track if we've ever been ready — keeps CDPViewer mounted once connected
  // so the WebSocket connection survives tab switches even if isReady temporarily flickers.
  // This avoids a premature WebSocket handshake during STARTING/NO_BROWSER states.
  const hasBeenReadyRef = useRef(false);
  if (isReady) hasBeenReadyRef.current = true;
  const shouldMountViewer = hasBeenReadyRef.current;

  const wsUrl = buildWsUrl(baseUrl, sessionId);

  // Connection state for reconnecting banner and manual reconnect
  const [connectionState, setConnectionState] = useState<'connected' | 'reconnecting' | 'disconnected'>('disconnected');
  // Incrementing key forces CDPViewer to re-mount on manual reconnect
  const [viewerKey, setViewerKey] = useState(0);

  function handleReconnect() {
    setConnectionState('disconnected');
    setViewerKey((k) => k + 1);
  }

  return (
    <div className={styles.browserTabContainer}>
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
            {status === VNCStatus.VNC_STATUS_STARTING ? (
              <span>Starting virtual display...</span>
            ) : (
              <>
                <span>No browser open yet</span>
                <span style={{ fontSize: '0.85em', opacity: 0.7 }}>
                  Run a browser in this session to see it here
                </span>
              </>
            )}
          </div>
        )}

        {/* Mount CDPViewer only once isReady has been true — prevents premature
            WebSocket handshake during STARTING/NO_BROWSER states before the CDP
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
            <CDPViewer
              key={viewerKey}
              wsUrl={wsUrl}
              isVisible={isVisible && isReady}
              onConnected={() => setConnectionState('connected')}
              onDisconnected={() => setConnectionState('reconnecting')}
            />
          </div>
        )}
      </div>
    </div>
  );
}
