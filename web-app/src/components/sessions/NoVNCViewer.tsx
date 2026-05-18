'use client';
import { useEffect, useRef } from 'react';
import RFB from '@novnc/novnc';

interface NoVNCViewerProps {
  wsUrl: string;
  isVisible: boolean;
  qualityLevel: number;
}

// This component gets dynamically imported with ssr: false by BrowserTab.
// It manages the noVNC RFB connection lifecycle: connect on mount, reconnect
// when wsUrl changes, update qualityLevel at runtime, and disconnect on unmount.
export default function NoVNCViewer({ wsUrl, isVisible, qualityLevel }: NoVNCViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;
    let rfb: InstanceType<typeof RFB> | null = null;
    try {
      rfb = new RFB(containerRef.current, wsUrl);
      rfb.scaleViewport = true;
      rfb.resizeSession = false;
      rfb.clipViewport = false;
      rfb.qualityLevel = qualityLevel;
      rfb.compressionLevel = 6;
      rfbRef.current = rfb;
    } catch (err) {
      console.warn('NoVNCViewer: RFB constructor failed', err);
    }
    return () => {
      if (rfbRef.current) {
        rfbRef.current.disconnect();
        rfbRef.current = null;
      }
    };
    // Reconnect only when wsUrl changes; qualityLevel is updated via its own effect
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wsUrl]);

  // Update quality at runtime without reconnecting
  useEffect(() => {
    if (rfbRef.current) {
      rfbRef.current.qualityLevel = qualityLevel;
    }
  }, [qualityLevel]);

  // Pause/resume rendering updates based on tab visibility
  useEffect(() => {
    if (!rfbRef.current) return;
    if (isVisible) {
      rfbRef.current.focus();
    } else {
      rfbRef.current.blur();
    }
  }, [isVisible]);

  // Visibility is managed by the parent via CSS; this div stays mounted
  return (
    <div
      ref={containerRef}
      role="img"
      aria-label="Remote browser display"
      style={{ width: '100%', height: '100%' }}
    />
  );
}
