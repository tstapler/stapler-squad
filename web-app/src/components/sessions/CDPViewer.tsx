'use client';
import { useEffect, useRef } from 'react';

export interface CDPViewerProps {
  wsUrl: string;
  isVisible: boolean;
  onConnected?: () => void;
  onDisconnected?: () => void;
}

// CDPViewer connects to a WebSocket that streams JPEG frames (binary messages)
// and renders them to a canvas. Mouse and keyboard events are forwarded back
// to the server as CDP Input.dispatch* JSON messages.
export default function CDPViewer({ wsUrl, isVisible, onConnected, onDisconnected }: CDPViewerProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const unmountedRef = useRef(false);
  // Keep latest callbacks in a ref so they can be used in stable event handlers
  const onConnectedRef = useRef(onConnected);
  const onDisconnectedRef = useRef(onDisconnected);
  const isVisibleRef = useRef(isVisible);

  useEffect(() => { onConnectedRef.current = onConnected; }, [onConnected]);
  useEffect(() => { onDisconnectedRef.current = onDisconnected; }, [onDisconnected]);
  useEffect(() => { isVisibleRef.current = isVisible; }, [isVisible]);

  // ------------------------------------------------------------------
  // WebSocket lifecycle
  // ------------------------------------------------------------------

  useEffect(() => {
    unmountedRef.current = false;

    // renderFrame only accesses canvasRef (a stable ref) so it does not need
    // to be listed in the effect's dependency array.
    async function renderFrame(data: ArrayBuffer) {
      const canvas = canvasRef.current;
      if (!canvas) return;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;

      if (typeof createImageBitmap !== 'undefined') {
        // Fast path: use createImageBitmap (supported in modern browsers)
        const blob = new Blob([data], { type: 'image/jpeg' });
        try {
          const bitmap = await createImageBitmap(blob);
          canvas.width = bitmap.width;
          canvas.height = bitmap.height;
          ctx.drawImage(bitmap, 0, 0);
          bitmap.close();
        } catch {
          // Ignore decode errors on individual frames
        }
      } else {
        // Fallback: use an Image element
        const blob = new Blob([data], { type: 'image/jpeg' });
        const url = URL.createObjectURL(blob);
        const img = new Image();
        img.onload = () => {
          canvas.width = img.naturalWidth;
          canvas.height = img.naturalHeight;
          ctx.drawImage(img, 0, 0);
          URL.revokeObjectURL(url);
        };
        img.onerror = () => URL.revokeObjectURL(url);
        img.src = url;
      }
    }

    function connect() {
      if (unmountedRef.current) return;

      const ws = new WebSocket(wsUrl);
      ws.binaryType = 'arraybuffer';
      wsRef.current = ws;

      ws.addEventListener('open', () => {
        if (unmountedRef.current) { ws.close(); return; }
        onConnectedRef.current?.();
      });

      ws.addEventListener('message', (event: MessageEvent) => {
        if (event.data instanceof ArrayBuffer) {
          void renderFrame(event.data);
        }
        // Text messages from server (status, etc.) are intentionally ignored
      });

      ws.addEventListener('close', () => {
        wsRef.current = null;
        onDisconnectedRef.current?.();
        if (!unmountedRef.current) {
          // Reconnect with 2 s backoff
          reconnectTimerRef.current = setTimeout(connect, 2000);
        }
      });

      ws.addEventListener('error', () => {
        // Error is always followed by a close event; let close handle reconnection
      });
    }

    connect();

    return () => {
      unmountedRef.current = true;
      if (reconnectTimerRef.current !== null) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [wsUrl]);

  // ------------------------------------------------------------------
  // Input event helpers
  // ------------------------------------------------------------------

  function sendJson(obj: unknown) {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(obj));
    }
  }

  function getCanvasCoords(e: React.MouseEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current;
    if (!canvas) return { x: 0, y: 0 };
    const rect = canvas.getBoundingClientRect();
    // Scale CSS pixels → Chrome device pixels using canvas intrinsic size
    const scaleX = canvas.width / rect.width;
    const scaleY = canvas.height / rect.height;
    return {
      x: Math.round((e.clientX - rect.left) * scaleX),
      y: Math.round((e.clientY - rect.top) * scaleY),
    };
  }

  function getModifiers(e: React.MouseEvent | React.KeyboardEvent): number {
    // CDP modifier bitmask: Alt=1, Ctrl=2, Meta/Cmd=4, Shift=8
    let m = 0;
    if (e.altKey) m |= 1;
    if (e.ctrlKey) m |= 2;
    if (e.metaKey) m |= 4;
    if (e.shiftKey) m |= 8;
    return m;
  }

  function cdpButton(e: React.MouseEvent): 'left' | 'right' | 'middle' | 'none' {
    switch (e.button) {
      case 0: return 'left';
      case 1: return 'middle';
      case 2: return 'right';
      default: return 'none';
    }
  }

  // ------------------------------------------------------------------
  // Mouse handlers
  // ------------------------------------------------------------------

  function handleMouseDown(e: React.MouseEvent<HTMLCanvasElement>) {
    if (!isVisibleRef.current) return;
    const { x, y } = getCanvasCoords(e);
    sendJson({
      method: 'Input.dispatchMouseEvent',
      params: {
        type: 'mousePressed',
        x,
        y,
        button: cdpButton(e),
        clickCount: 1,
        modifiers: getModifiers(e),
      },
    });
  }

  function handleMouseUp(e: React.MouseEvent<HTMLCanvasElement>) {
    if (!isVisibleRef.current) return;
    const { x, y } = getCanvasCoords(e);
    sendJson({
      method: 'Input.dispatchMouseEvent',
      params: {
        type: 'mouseReleased',
        x,
        y,
        button: cdpButton(e),
        clickCount: 1,
        modifiers: getModifiers(e),
      },
    });
  }

  function handleMouseMove(e: React.MouseEvent<HTMLCanvasElement>) {
    if (!isVisibleRef.current) return;
    const { x, y } = getCanvasCoords(e);
    sendJson({
      method: 'Input.dispatchMouseEvent',
      params: {
        type: 'mouseMoved',
        x,
        y,
        button: 'none',
        clickCount: 0,
        modifiers: getModifiers(e),
      },
    });
  }

  // ------------------------------------------------------------------
  // Keyboard handlers
  // ------------------------------------------------------------------

  function handleKeyDown(e: React.KeyboardEvent<HTMLCanvasElement>) {
    if (!isVisibleRef.current) return;
    e.preventDefault();
    sendJson({
      method: 'Input.dispatchKeyEvent',
      params: {
        type: 'keyDown',
        key: e.key,
        code: e.code,
        modifiers: getModifiers(e),
      },
    });
  }

  function handleKeyUp(e: React.KeyboardEvent<HTMLCanvasElement>) {
    if (!isVisibleRef.current) return;
    sendJson({
      method: 'Input.dispatchKeyEvent',
      params: {
        type: 'keyUp',
        key: e.key,
        code: e.code,
        modifiers: getModifiers(e),
      },
    });
  }

  return (
    <canvas
      ref={canvasRef}
      role="img"
      aria-label="Remote browser display"
      tabIndex={0}
      style={{ width: '100%', height: '100%', display: 'block' }}
      onMouseDown={handleMouseDown}
      onMouseUp={handleMouseUp}
      onMouseMove={handleMouseMove}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
    />
  );
}
