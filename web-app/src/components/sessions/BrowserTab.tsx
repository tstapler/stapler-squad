'use client';
import { useRef } from 'react';
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

function buildWsUrl(baseUrl: string, sessionId: string): string {
  // Normalize: strip trailing slashes, then strip /api suffix if present.
  // Handles cases where baseUrl ends with '/' or lacks the /api suffix,
  // which would otherwise produce a doubled path like wss://host/api/api/sessions/...
  const origin = baseUrl.replace(/\/+$/, '').replace(/\/api$/, '');
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
            <CDPViewer
              wsUrl={wsUrl}
              isVisible={isVisible && isReady}
            />
          </div>
        )}
      </div>
    </div>
  );
}
