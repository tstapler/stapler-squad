"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
import { XtermTerminal } from "./XtermTerminal";
import styles from "./TerminalOutput.module.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
}

export function TerminalOutput({ sessionId, baseUrl }: TerminalOutputProps) {
  const xtermRef = useRef<any>(null);
  const [connectionAttempts, setConnectionAttempts] = useState(0);
  const [showReconnectButton, setShowReconnectButton] = useState(false);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const previousConnectionStateRef = useRef(false);
  const lastResizeRef = useRef<{ cols: number; rows: number } | null>(null);

  // Callback to write scrollback directly to terminal
  const handleScrollbackReceived = useCallback((scrollback: string) => {
    console.log(`[TerminalOutput] Received ${scrollback.length} bytes of scrollback`);
    if (xtermRef.current) {
      xtermRef.current.write(scrollback);
    }
  }, []);

  // Callback to write output directly to terminal (bypasses React state for better performance)
  const handleOutput = useCallback((output: string) => {
    if (xtermRef.current) {
      xtermRef.current.write(output);
    }
  }, []);

  const { isConnected, error, sendInput, resize, connect, disconnect, scrollbackLoaded } = useTerminalStream({
    baseUrl,
    sessionId,
    scrollbackLines: 1000,
    onError: (err) => {
      console.error("Terminal stream error:", err);
      setConnectionAttempts((prev) => prev + 1);
    },
    onScrollbackReceived: handleScrollbackReceived,
    onOutput: handleOutput,
  });

  // Disconnect WebSocket on unmount
  useEffect(() => {
    return () => {
      disconnect();
    };
  }, [disconnect]);

  // Handle terminal data input
  const handleTerminalData = useCallback((data: string) => {
    sendInput(data);
  }, [sendInput]);

  // Handle terminal resize - only send if size actually changed
  const handleTerminalResize = useCallback((cols: number, rows: number) => {
    if (!isConnected) {
      console.log(`[TerminalOutput] Resize blocked - not connected (${cols}x${rows})`);
      return;
    }

    // Block resize messages during scrollback load to prevent feedback loop
    if (!scrollbackLoaded) {
      console.log(`[TerminalOutput] Resize blocked - scrollback loading (${cols}x${rows})`);
      // Save the size for after scrollback completes
      lastResizeRef.current = { cols, rows };
      return;
    }

    const lastResize = lastResizeRef.current;
    if (lastResize && lastResize.cols === cols && lastResize.rows === rows) {
      // Size unchanged, don't send resize message
      console.log(`[TerminalOutput] Resize blocked - unchanged (${cols}x${rows})`);
      return;
    }

    console.log(`[TerminalOutput] Sending resize: ${cols}x${rows} (prev: ${lastResize?.cols || 'none'}x${lastResize?.rows || 'none'})`);
    lastResizeRef.current = { cols, rows };
    resize(cols, rows);
  }, [isConnected, scrollbackLoaded, resize]);

  // Monitor connection state changes and show notifications
  useEffect(() => {
    const wasConnected = previousConnectionStateRef.current;
    previousConnectionStateRef.current = isConnected;

    if (!wasConnected && isConnected) {
      // Just connected
      console.log("[TerminalOutput] Connection established");
      setShowReconnectButton(false);
      setConnectionAttempts(0);

      // Clear any pending reconnect timeout
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
    } else if (wasConnected && !isConnected) {
      // Just disconnected
      console.log("[TerminalOutput] Connection lost, will attempt reconnection");

      // Show reconnect button after 5 seconds if still disconnected
      reconnectTimeoutRef.current = setTimeout(() => {
        if (!isConnected) {
          setShowReconnectButton(true);
        }
      }, 5000);
    }

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [isConnected]);

  // Auto-reconnect with exponential backoff
  useEffect(() => {
    if (!isConnected && error && connectionAttempts > 0 && connectionAttempts < 5) {
      const backoffDelay = Math.min(1000 * Math.pow(2, connectionAttempts - 1), 10000);
      console.log(`[TerminalOutput] Auto-reconnecting in ${backoffDelay}ms (attempt ${connectionAttempts})`);

      const timeout = setTimeout(() => {
        console.log("[TerminalOutput] Attempting reconnection...");
        connect();
      }, backoffDelay);

      return () => clearTimeout(timeout);
    }
  }, [isConnected, error, connectionAttempts, connect]);

  // Send resize after scrollback load completes
  useEffect(() => {
    if (scrollbackLoaded && isConnected && lastResizeRef.current) {
      const { cols, rows } = lastResizeRef.current;
      console.log(`[TerminalOutput] Scrollback complete, sending final resize: ${cols}x${rows}`);
      resize(cols, rows);
    }
  }, [scrollbackLoaded, isConnected, resize]);

  const handleManualReconnect = useCallback(() => {
    console.log("[TerminalOutput] Manual reconnect requested");
    setConnectionAttempts(0);
    setShowReconnectButton(false);
    connect();
  }, [connect]);

  const handleCopyOutput = () => {
    // XtermTerminal handles copy internally via browser selection
    document.execCommand('copy');
  };

  const handleScrollToBottom = () => {
    if (xtermRef.current?.terminal) {
      xtermRef.current.terminal.scrollToBottom();
    }
  };

  const handleClear = () => {
    if (xtermRef.current) {
      xtermRef.current.clear();
    }
  };

  const handleManualResize = () => {
    console.log("[TerminalOutput] Manual resize triggered");
    if (xtermRef.current) {
      xtermRef.current.fit();
      console.log("[TerminalOutput] Terminal resized manually");
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <div className={styles.status}>
          <span
            className={`${styles.statusIndicator} ${
              isConnected ? styles.connected : styles.disconnected
            }`}
          />
          <span className={styles.statusText}>
            {isConnected ? "Connected" : "Disconnected"}
          </span>
          {isConnected && !scrollbackLoaded && (
            <span className={styles.statusText}> • Loading scrollback...</span>
          )}
          {!isConnected && connectionAttempts > 0 && connectionAttempts < 5 && (
            <span className={styles.statusText}>
              {" "}• Reconnecting (attempt {connectionAttempts}/5)...
            </span>
          )}
          {!isConnected && connectionAttempts >= 5 && (
            <span className={styles.errorText}> • Connection failed</span>
          )}
          {error && !isConnected && (
            <span className={styles.errorText}> • {error.message}</span>
          )}
        </div>
        <div className={styles.actions}>
          {showReconnectButton && (
            <button
              className={styles.toolbarButton}
              onClick={handleManualReconnect}
              title="Reconnect to terminal"
              aria-label="Reconnect to terminal"
            >
              🔄 Reconnect
            </button>
          )}
          <button
            className={styles.toolbarButton}
            onClick={handleManualResize}
            title="Resize terminal to fit container"
            aria-label="Resize terminal"
          >
            ↔️ Resize
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleClear}
            title="Clear terminal"
            aria-label="Clear terminal"
          >
            🗑️ Clear
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleScrollToBottom}
            title="Scroll to bottom"
            aria-label="Scroll to bottom"
          >
            ↓ Bottom
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleCopyOutput}
            title="Copy output"
            aria-label="Copy terminal output to clipboard"
          >
            📋 Copy
          </button>
        </div>
      </div>
      <div className={styles.terminal}>
        <XtermTerminal
          ref={xtermRef}
          onData={handleTerminalData}
          onResize={handleTerminalResize}
          theme="dark"
          fontSize={14}
          scrollback={10000}
        />
      </div>
    </div>
  );
}
