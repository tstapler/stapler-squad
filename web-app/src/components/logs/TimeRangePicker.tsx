"use client";

import { useState, useRef, useEffect } from 'react';
import { TIME_RANGE_PRESETS, TimeRangePreset } from '@/lib/utils/datetime';
import {
  container,
  trigger,
  icon,
  label,
  chevron,
  dropdown,
  presets,
  presetButton,
  presetButtonActive,
  divider,
  customButton,
  customRange,
  customHeader,
  backButton,
  customInputs,
  inputGroup,
  dateInput,
  applyButton,
} from './TimeRangePicker.css';

export interface TimeRange {
  start: Date;
  end: Date;
  preset?: string;
}

interface TimeRangePickerProps {
  value: TimeRange;
  onChange: (range: TimeRange) => void;
  className?: string;
}

export function TimeRangePicker({ value, onChange, className }: TimeRangePickerProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [showCustom, setShowCustom] = useState(false);
  const [customStart, setCustomStart] = useState('');
  const [customEnd, setCustomEnd] = useState('');
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Get current display label
  const getDisplayLabel = (): string => {
    if (value.preset) {
      const preset = TIME_RANGE_PRESETS.find(p => p.value === value.preset);
      return preset?.label || 'Custom';
    }
    return 'Custom range';
  };

  // Handle preset selection
  const handlePresetClick = (preset: TimeRangePreset) => {
    const range = preset.getRange();
    onChange({
      start: range.start,
      end: range.end,
      preset: preset.value,
    });
    setIsOpen(false);
    setShowCustom(false);
  };

  // Handle custom range submission
  const handleCustomSubmit = () => {
    if (customStart && customEnd) {
      const start = new Date(customStart);
      const end = new Date(customEnd);
      if (!isNaN(start.getTime()) && !isNaN(end.getTime()) && start < end) {
        onChange({
          start,
          end,
          preset: undefined,
        });
        setIsOpen(false);
        setShowCustom(false);
      }
    }
  };

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
        setShowCustom(false);
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
      setShowCustom(false);
    } else if (event.key === 'Enter' && !isOpen) {
      setIsOpen(true);
    }
  };

  return (
    <div
      className={`${container} ${className || ''}`}
      ref={dropdownRef}
      onKeyDown={handleKeyDown}
    >
      <button
        className={trigger}
        onClick={() => setIsOpen(!isOpen)}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label={`Time range: ${getDisplayLabel()}`}
        type="button"
      >
        <span className={icon}>🕐</span>
        <span className={label}>{getDisplayLabel()}</span>
        <span className={chevron}>{isOpen ? '▲' : '▼'}</span>
      </button>

      {isOpen && (
        <div className={dropdown} role="listbox" aria-label="Select time range">
          {!showCustom ? (
            <>
              <div className={presets}>
                {TIME_RANGE_PRESETS.map((preset) => (
                  <button
                    key={preset.value}
                    className={`${presetButton} ${value.preset === preset.value ? presetButtonActive : ''}`}
                    onClick={() => handlePresetClick(preset)}
                    role="option"
                    aria-selected={value.preset === preset.value}
                  >
                    {preset.label}
                  </button>
                ))}
              </div>
              <div className={divider} />
              <button
                className={customButton}
                onClick={() => setShowCustom(true)}
                type="button"
              >
                Custom range...
              </button>
            </>
          ) : (
            <div className={customRange}>
              <div className={customHeader}>
                <button
                  className={backButton}
                  onClick={() => setShowCustom(false)}
                  type="button"
                  aria-label="Back to presets"
                >
                  ← Back
                </button>
                <span>Custom Range</span>
              </div>
              <div className={customInputs}>
                <label className={inputGroup}>
                  <span>Start:</span>
                  <input
                    type="datetime-local"
                    value={customStart}
                    onChange={(e) => setCustomStart(e.target.value)}
                    className={dateInput}
                    aria-label="Start date and time"
                  />
                </label>
                <label className={inputGroup}>
                  <span>End:</span>
                  <input
                    type="datetime-local"
                    value={customEnd}
                    onChange={(e) => setCustomEnd(e.target.value)}
                    className={dateInput}
                    aria-label="End date and time"
                  />
                </label>
              </div>
              <button
                className={applyButton}
                onClick={handleCustomSubmit}
                disabled={!customStart || !customEnd}
                type="button"
              >
                Apply
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default TimeRangePicker;
