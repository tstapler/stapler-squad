"use client";

import { useState, useRef, useEffect } from 'react';
import styles from './MultiSelect.module.css';

interface MultiSelectOption {
  value: string;
  label: string;
  color?: string;
}

interface MultiSelectProps {
  label: string;
  options: MultiSelectOption[];
  value: string[];
  onChange: (selected: string[]) => void;
  placeholder?: string;
  className?: string;
}

export function MultiSelect({
  label,
  options,
  value,
  onChange,
  placeholder = 'All',
  className,
}: MultiSelectProps) {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Get display text
  const getDisplayText = (): string => {
    if (value.length === 0) return placeholder;
    if (value.length === 1) {
      const option = options.find(o => o.value === value[0]);
      return option?.label || value[0];
    }
    return `${value.length} selected`;
  };

  // Toggle option selection
  const toggleOption = (optionValue: string) => {
    if (value.includes(optionValue)) {
      onChange(value.filter(v => v !== optionValue));
    } else {
      onChange([...value, optionValue]);
    }
  };

  // Select all
  const selectAll = () => {
    onChange(options.map(o => o.value));
  };

  // Clear all
  const clearAll = () => {
    onChange([]);
  };

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };

    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen]);

  // Handle keyboard navigation
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'Escape') {
      setIsOpen(false);
    } else if (event.key === 'Enter' && !isOpen) {
      setIsOpen(true);
    }
  };

  return (
    <div
      className={`${styles.container} ${className || ''}`}
      ref={containerRef}
      onKeyDown={handleKeyDown}
    >
      <label className={styles.label}>{label}:</label>
      <button
        className={styles.trigger}
        onClick={() => setIsOpen(!isOpen)}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label={`${label}: ${getDisplayText()}`}
        type="button"
      >
        <span className={styles.text}>{getDisplayText()}</span>
        <span className={styles.chevron}>{isOpen ? '▲' : '▼'}</span>
      </button>

      {isOpen && (
        <div className={styles.dropdown} role="listbox" aria-label={`Select ${label}`}>
          <div className={styles.actions}>
            <button
              className={styles.actionButton}
              onClick={selectAll}
              type="button"
            >
              Select all
            </button>
            <button
              className={styles.actionButton}
              onClick={clearAll}
              type="button"
            >
              Clear
            </button>
          </div>
          <div className={styles.divider} />
          <div className={styles.options}>
            {options.map((option) => (
              <label
                key={option.value}
                className={styles.option}
              >
                <input
                  type="checkbox"
                  checked={value.includes(option.value)}
                  onChange={() => toggleOption(option.value)}
                  className={styles.checkbox}
                />
                <span
                  className={styles.optionLabel}
                  style={option.color ? { color: option.color } : undefined}
                >
                  {option.label}
                </span>
              </label>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// Pre-configured log level options
export const LOG_LEVEL_OPTIONS: MultiSelectOption[] = [
  { value: 'DEBUG', label: 'DEBUG', color: '#6c757d' },
  { value: 'INFO', label: 'INFO', color: '#17a2b8' },
  { value: 'WARNING', label: 'WARNING', color: '#ffc107' },
  { value: 'ERROR', label: 'ERROR', color: '#dc3545' },
  { value: 'FATAL', label: 'FATAL', color: '#ff0000' },
];

export default MultiSelect;
