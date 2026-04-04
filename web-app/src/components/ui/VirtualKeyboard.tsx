"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import styles from "./VirtualKeyboard.module.css";

interface VirtualKeyboardProps {
  isOpen: boolean;
  onClose: () => void;
  onKeyPress: (key: string) => void;
}

export function VirtualKeyboard({ isOpen, onClose, onKeyPress }: VirtualKeyboardProps) {
  const keyboardRef = useRef<HTMLDivElement>(null);
  const [isShifted, setIsShifted] = useState(false);
  const [isCapsLocked, setIsCapsLocked] = useState(false);

  useFocusTrap(keyboardRef, isOpen);

  // Close on escape key
  useEffect(() => {
    if (!isOpen) return;
    
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };
    
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  // Handle key press - sends actual terminal codes for special keys
  const handleKeyPress = useCallback((key: string) => {
    onKeyPress(key);
  }, [onKeyPress]);

  // Simple render function with proper terminal escape codes
  const render = () => {
    return (
      <div 
        className={`${styles.backdrop} ${isOpen ? styles.open : ""}`} 
        onClick={onClose}
        ref={keyboardRef}
      >
        <div 
          className={`${styles.keyboard} ${isOpen ? styles.open : ""}`} 
          onClick={(e) => e.stopPropagation()}
        >
          <div className={styles.header}>
            <h3 className={styles.title}>Virtual Keyboard</h3>
            <button 
              className={styles.closeButton} 
              onClick={onClose}
              aria-label="Close virtual keyboard"
            >
              ✕
            </button>
          </div>
          
          <div className={styles.keysContainer}>
            <div className={styles.keyRow}>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("1"); }} aria-label="1">1</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("2"); }} aria-label="2">2</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("3"); }} aria-label="3">3</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("4"); }} aria-label="4">4</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("5"); }} aria-label="5">5</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("6"); }} aria-label="6">6</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("7"); }} aria-label="7">7</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("8"); }} aria-label="8">8</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("9"); }} aria-label="9">9</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("0"); }} aria-label="0">0</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("\x08"); }} aria-label="Backspace">⌫</button>
            </div>
            
            <div className={styles.keyRow}>
              <button className={`${styles.key} ${styles.tabKey}`} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("\t"); }} aria-label="Tab">Tab</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("q"); }} aria-label="q">q</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("w"); }} aria-label="w">w</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("e"); }} aria-label="e">e</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("r"); }} aria-label="r">r</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("t"); }} aria-label="t">t</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("y"); }} aria-label="y">y</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("u"); }} aria-label="u">u</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("i"); }} aria-label="i">i</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("o"); }} aria-label="o">o</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("p"); }} aria-label="p">p</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("\n"); }} aria-label="Enter">Enter</button>
            </div>
            
            <div className={styles.keyRow}>
              <button className={`${styles.key} ${styles.capsLockKey}`} onPointerDown={(e) => { e.preventDefault(); setIsCapsLocked(!isCapsLocked); }} aria-label="Caps Lock" style={{ backgroundColor: isCapsLocked ? "#4ec9b0" : "#3e3e42", color: isCapsLocked ? "#1e1e1e" : "#cccccc" }}>Caps Lock</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("a"); }} aria-label="a">a</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("s"); }} aria-label="s">s</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("d"); }} aria-label="d">d</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("f"); }} aria-label="f">f</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("g"); }} aria-label="g">g</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("h"); }} aria-label="h">h</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("j"); }} aria-label="j">j</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("k"); }} aria-label="k">k</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("l"); }} aria-label="l">l</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("\x03"); }} aria-label="Ctrl+C" style={{ backgroundColor: "#4ec9b0", color: "#1e1e1e", fontWeight: "bold" }}>Ctrl+C</button>
            </div>
            
            <div className={styles.keyRow}>
              <button className={`${styles.key} ${styles.specialKey}`} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("\x1b"); }} aria-label="Escape">Esc</button>
              <button className={`${styles.key} ${styles.specialKey}`} onPointerDown={(e) => { e.preventDefault(); handleKeyPress(" "); }} aria-label="Space">Space</button>
              <button className={`${styles.key} ${styles.specialKey}`} onPointerDown={(e) => { e.preventDefault(); handleKeyPress("/"); }} aria-label="Slash">/</button>
              <button className={styles.key} onPointerDown={(e) => { e.preventDefault(); setIsShifted(!isShifted); }} aria-label="Shift" style={{ backgroundColor: isShifted ? "#4ec9b0" : "#3e3e42", color: isShifted ? "#1e1e1e" : "#cccccc" }}>Shift</button>
            </div>
          </div>
        </div>
      </div>
    );
  };

  return render();
}