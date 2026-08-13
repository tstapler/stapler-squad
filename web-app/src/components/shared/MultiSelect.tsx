"use client";

import { useState, useRef, useEffect } from 'react';
import {
  container as containerClass,
  label as labelClass,
  trigger,
  text,
  chevron,
  dropdown,
  actions,
  actionButton,
  divider,
  options as optionsClass,
  option as optionClass,
  checkbox,
  optionLabel,
} from './MultiSelect.css';

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
  options: opts,
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
      const opt = opts.find(o => o.value === value[0]);
      return opt?.label || value[0];
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
    onChange(opts.map(o => o.value));
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
      className={`${containerClass} ${className || ''}`}
      ref={containerRef}
      onKeyDown={handleKeyDown}
    >
      <label className={labelClass}>{label}:</label>
      <button
        className={trigger}
        onClick={() => setIsOpen(!isOpen)}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label={`${label}: ${getDisplayText()}`}
        type="button"
      >
        <span className={text}>{getDisplayText()}</span>
        <span className={chevron}>{isOpen ? '▲' : '▼'}</span>
      </button>

      {isOpen && (
        <div className={dropdown} role="listbox" aria-label={`Select ${label}`}>
          <div className={actions}>
            <button
              className={actionButton}
              onClick={selectAll}
              type="button"
            >
              Select all
            </button>
            <button
              className={actionButton}
              onClick={clearAll}
              type="button"
            >
              Clear
            </button>
          </div>
          <div className={divider} />
          <div className={optionsClass}>
            {opts.map((opt) => (
              <label
                key={opt.value}
                className={optionClass}
              >
                <input
                  type="checkbox"
                  checked={value.includes(opt.value)}
                  onChange={() => toggleOption(opt.value)}
                  className={checkbox}
                />
                <span
                  className={optionLabel}
                  style={opt.color ? { color: opt.color } : undefined}
                >
                  {opt.label}
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
