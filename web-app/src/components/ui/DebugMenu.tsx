"use client";

import { useState, useEffect } from "react";
import styles from "./DebugMenu.module.css";

interface DebugMenuProps {
  isOpen: boolean;
  onClose: () => void;
}

export function DebugMenu({ isOpen, onClose }: DebugMenuProps) {
  const [terminalDebug, setTerminalDebug] = useState(false);

  // Load initial state from localStorage
  useEffect(() => {
    if (typeof window !== "undefined") {
      const value = localStorage.getItem("debug-terminal") === "true";
      setTerminalDebug(value);
    }
  }, [isOpen]);

  const handleTerminalDebugToggle = () => {
    const newValue = !terminalDebug;
    setTerminalDebug(newValue);

    if (typeof window !== "undefined") {
      if (newValue) {
        localStorage.setItem("debug-terminal", "true");
        console.log("✓ Terminal debug logging enabled");
      } else {
        localStorage.removeItem("debug-terminal");
        console.log("✗ Terminal debug logging disabled");
      }
    }
  };

  if (!isOpen) return null;

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.menu} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2 className={styles.title}>🛠️ Debug Menu</h2>
          <button
            className={styles.closeButton}
            onClick={onClose}
            aria-label="Close debug menu"
          >
            ×
          </button>
        </div>

        <div className={styles.content}>
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Logging</h3>

            <label className={styles.toggleRow}>
              <div className={styles.toggleLabel}>
                <span className={styles.toggleName}>Terminal Stream Logging</span>
                <span className={styles.toggleDescription}>
                  Show byte count for each terminal output chunk
                </span>
              </div>
              <button
                className={`${styles.toggle} ${terminalDebug ? styles.toggleOn : ""}`}
                onClick={handleTerminalDebugToggle}
                role="switch"
                aria-checked={terminalDebug}
              >
                <span className={styles.toggleSlider} />
              </button>
            </label>
          </div>

          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Console Commands</h3>
            <div className={styles.commandList}>
              <code className={styles.command}>
                localStorage.setItem("debug-terminal", "true")
              </code>
              <span className={styles.commandDescription}>Enable terminal logging</span>
            </div>
          </div>
        </div>

        <div className={styles.footer}>
          <button className={styles.doneButton} onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </div>
  );
}
