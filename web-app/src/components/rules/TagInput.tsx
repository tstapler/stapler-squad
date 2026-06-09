"use client";

import { useRef, useState, KeyboardEvent, ClipboardEvent, useEffect } from "react";
import {
  container, containerPrefilled, chip, chipDisabled, chipRemove, input, helperText,
} from "./TagInput.css";

interface TagInputProps {
  value: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
  helperText?: string;
  isPrefilled?: boolean;
}

export function TagInput({ value, onChange, placeholder, disabled, helperText: helper, isPrefilled }: TagInputProps) {
  const [text, setText] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const [showHighlight, setShowHighlight] = useState(false);

  useEffect(() => {
    if (isPrefilled && value.length > 0) {
      setShowHighlight(true);
      const t = setTimeout(() => setShowHighlight(false), 2100);
      return () => clearTimeout(t);
    }
  }, [isPrefilled, value.length]);

  function addTag(raw: string) {
    const trimmed = raw.trim();
    if (!trimmed || value.includes(trimmed)) return;
    onChange([...value, trimmed]);
  }

  function removeTag(tag: string) {
    onChange(value.filter((t) => t !== tag));
  }

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if ((e.key === "Enter" || e.key === ",") && text) {
      e.preventDefault();
      addTag(text);
      setText("");
    } else if (e.key === "Backspace" && !text && value.length > 0) {
      onChange(value.slice(0, -1));
    }
  }

  function handlePaste(e: ClipboardEvent<HTMLInputElement>) {
    e.preventDefault();
    const pasted = e.clipboardData.getData("text");
    pasted.split(/[,\s]+/).forEach(addTag);
  }

  return (
    <div>
      <div
        className={`${container} ${showHighlight ? containerPrefilled : ""}`}
        onClick={() => inputRef.current?.focus()}
      >
        {value.map((tag) => (
          <span key={tag} className={`${chip} ${disabled ? chipDisabled : ""}`}>
            {tag}
            {!disabled && (
              <button
                type="button"
                className={chipRemove}
                onClick={(e) => { e.stopPropagation(); removeTag(tag); }}
                aria-label={`Remove ${tag}`}
              >
                ×
              </button>
            )}
          </span>
        ))}
        <input
          ref={inputRef}
          className={input}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          onBlur={() => { if (text) { addTag(text); setText(""); } }}
          placeholder={value.length === 0 ? placeholder : ""}
          disabled={disabled}
          aria-label={placeholder}
        />
      </div>
      {helper && <p className={helperText}>{helper}</p>}
    </div>
  );
}
