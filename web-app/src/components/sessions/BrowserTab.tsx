'use client';
import { useState, useRef } from 'react';
import dynamic from 'next/dynamic';
import * as styles from './BrowserTab.css';

// VNCState mirrors proto VNCState (session/v1/types.proto).
// TODO: replace with the proto-generated type from @/gen/session/v1/types_pb.ts
// once SessionDetailView passes the generated type through vncState prop.
export interface VNCState {
  status?: number; // 0=UNSPECIFIED, 1=STARTING, 2=READY, 3=NO_BROWSER, 4=UNAVAILABLE
  displayNumber?: number;
  browserWindowDetected?: boolean;
}

export const VNCStatus = {
  UNSPECIFIED: 0,
  STARTING: 1,
  READY: 2,
  NO_BROWSER: 3,
  UNAVAILABLE: 4,
} as const;

// NoVNCViewer is dynamically imported with ssr: false — it uses DOM APIs
// not available during server-side rendering, matching the TerminalOutput pattern.
const NoVNCViewer = dynamic(() => import('./NoVNCViewer'), {
  ssr: false,
  loading: () => (
    <div className={styles.placeholderOverlay}>
      <span>Connecting to display...</span>
    </div>
  ),
});

interface BrowserTabProps {
  sessionId: string;
  /** The application base URL, e.g. window.location.origin + '/api'. Used to derive the WebSocket URL. */
  baseUrl: string;
  isVisible: boolean;
  vncState: VNCState | undefined;
}

type QualityLevel = 'low' | 'medium' | 'high';
const QUALITY_MAP: Record<QualityLevel, number> = { low: 3, medium: 6, high: 9 };
const QUALITY_LEVELS: QualityLevel[] = ['low', 'medium', 'high'];

function buildWsUrl(baseUrl: string, sessionId: string): string {
  // Normalize: strip trailing slashes, then strip /api suffix if present.
  // Handles cases where baseUrl ends with '/' or lacks the /api suffix,
  // which would otherwise produce a doubled path like wss://host/api/api/sessions/...
  const origin = baseUrl.replace(/\/+$/, '').replace(/\/api$/, '');
  const wsScheme = origin.startsWith('https') ? 'wss' : 'ws';
  const host = origin.replace(/^https?:\/\//, '');
  return `${wsScheme}://${host}/api/sessions/${sessionId}/vnc`;
}

export function BrowserTab({ sessionId, baseUrl, isVisible, vncState }: BrowserTabProps) {
  const [quality, setQuality] = useState<QualityLevel>('medium');

  const status = vncState?.status ?? VNCStatus.UNSPECIFIED;
  const isUnavailable = status === VNCStatus.UNAVAILABLE || status === VNCStatus.UNSPECIFIED;
  const isWaiting = !isUnavailable && (
    status === VNCStatus.STARTING ||
    status === VNCStatus.NO_BROWSER ||
    !vncState?.browserWindowDetected
  );
  const isReady = status === VNCStatus.READY && vncState?.browserWindowDetected === true;

  // Track if we've ever been ready — keeps NoVNCViewer mounted once connected
  // so the RFB connection survives tab switches even if isReady temporarily flickers.
  // This avoids the premature WebSocket handshake during STARTING/NO_BROWSER states.
  const hasBeenReadyRef = useRef(false);
  if (isReady) hasBeenReadyRef.current = true;
  const shouldMountViewer = hasBeenReadyRef.current;

  const wsUrl = buildWsUrl(baseUrl, sessionId);

  return (
    <div className={styles.browserTabContainer}>
      {/* Quality control strip — hidden when unavailable */}
      {!isUnavailable && (
        <div className={styles.qualityControls}>
          <span className={styles.qualityLabel}>Quality:</span>
          {QUALITY_LEVELS.map((q) => (
            <button
              key={q}
              className={`${styles.qualityButton}${quality === q ? ` ${styles.qualityButtonActive}` : ''}`}
              onClick={() => setQuality(q)}
              aria-pressed={quality === q}
            >
              {q.charAt(0).toUpperCase() + q.slice(1)}
            </button>
          ))}
        </div>
      )}

      {/* Viewer area */}
      <div className={styles.viewerArea}>
        {isUnavailable && (
          <div className={styles.placeholderOverlay}>
            <span>Browser passthrough unavailable on this host</span>
            <span style={{ fontSize: '0.85em', opacity: 0.7 }}>
              Requires Linux with Xvfb, x11vnc, and xdotool installed
            </span>
          </div>
        )}

        {isWaiting && !isUnavailable && (
          <div className={styles.placeholderOverlay}>
            {status === VNCStatus.STARTING ? (
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

        {/* Mount NoVNCViewer only once isReady has been true — prevents premature
            WebSocket handshake during STARTING/NO_BROWSER states when x11vnc hasn't
            opened its socket yet. Once mounted, keep it mounted (hasBeenReadyRef) so
            the RFB connection survives tab switches. */}
        {shouldMountViewer && (
          <div
            className={styles.canvasWrapper}
            style={{
              visibility: isReady ? 'visible' : 'hidden',
              pointerEvents: isReady ? 'auto' : 'none',
            }}
          >
            <NoVNCViewer
              wsUrl={wsUrl}
              isVisible={isVisible && isReady}
              qualityLevel={QUALITY_MAP[quality]}
            />
          </div>
        )}
      </div>
    </div>
  );
}
