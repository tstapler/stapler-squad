"use client";

import { useState, useRef, useEffect } from 'react';
import {
  container,
  toggleButton,
  toggleButtonActive,
  indicator,
  indicatorActive,
  indicatorPulse,
  label,
  pauseButton,
  settingsButton,
  dropdown,
  dropdownHeader,
  options,
  option,
  optionSelected,
  check,
  lastUpdate,
} from './LiveTailToggle.css';

interface LiveTailToggleProps {
  /** Whether live tail is enabled */
  isEnabled: boolean;
  /** Toggle live tail on/off */
  onToggle: () => void;
  /** Whether currently paused */
  isPaused: boolean;
  /** Pause/resume callback */
  onPauseToggle: () => void;
  /** Current refresh interval in ms */
  interval: number;
  /** Callback when interval changes */
  onIntervalChange: (interval: number) => void;
  /** Last refresh timestamp */
  lastRefresh?: Date | null;
  /** Additional class name */
  className?: string;
}

const INTERVAL_OPTIONS = [
  { value: 1000, label: '1s' },
  { value: 2000, label: '2s' },
  { value: 5000, label: '5s' },
  { value: 10000, label: '10s' },
  { value: 30000, label: '30s' },
];

export function LiveTailToggle({
  isEnabled,
  onToggle,
  isPaused,
  onPauseToggle,
  interval,
  onIntervalChange,
  lastRefresh,
  className,
}: LiveTailToggleProps) {
  const [showDropdown, setShowDropdown] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setShowDropdown(false);
      }
    };

    if (showDropdown) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [showDropdown]);

  // Format last refresh time
  const formatLastRefresh = () => {
    if (!lastRefresh) return '';
    const seconds = Math.floor((Date.now() - lastRefresh.getTime()) / 1000);
    if (seconds < 5) return 'just now';
    if (seconds < 60) return `${seconds}s ago`;
    return `${Math.floor(seconds / 60)}m ago`;
  };

  return (
    <div
      className={`${container} ${className || ''}`}
      ref={containerRef}
    >
      <button
        className={`${toggleButton} ${isEnabled ? toggleButtonActive : ''}`}
        onClick={onToggle}
        aria-pressed={isEnabled}
        aria-label={isEnabled ? 'Stop live tail' : 'Start live tail'}
        title={isEnabled ? 'Click to stop live updates' : 'Click to start live updates'}
      >
        <span className={`${indicator} ${isEnabled ? indicatorActive : ''} ${isEnabled && !isPaused ? indicatorPulse : ''}`} />
        <span className={label}>
          {isEnabled ? (isPaused ? 'Paused' : 'Live') : 'Live Tail'}
        </span>
      </button>

      {isEnabled && (
        <>
          <button
            className={pauseButton}
            onClick={onPauseToggle}
            aria-label={isPaused ? 'Resume live tail' : 'Pause live tail'}
            title={isPaused ? 'Resume updates' : 'Pause updates'}
          >
            {isPaused ? '▶' : '⏸'}
          </button>

          <button
            className={settingsButton}
            onClick={() => setShowDropdown(!showDropdown)}
            aria-label="Live tail settings"
            aria-expanded={showDropdown}
            title="Change refresh interval"
          >
            ⚙
          </button>

          {showDropdown && (
            <div className={dropdown}>
              <div className={dropdownHeader}>Refresh Interval</div>
              <div className={options}>
                {INTERVAL_OPTIONS.map((opt) => (
                  <button
                    key={opt.value}
                    className={`${option} ${interval === opt.value ? optionSelected : ''}`}
                    onClick={() => {
                      onIntervalChange(opt.value);
                      setShowDropdown(false);
                    }}
                  >
                    {opt.label}
                    {interval === opt.value && <span className={check}>✓</span>}
                  </button>
                ))}
              </div>
            </div>
          )}

          {lastRefresh && (
            <span className={lastUpdate} title={lastRefresh.toISOString()}>
              Updated {formatLastRefresh()}
            </span>
          )}
        </>
      )}
    </div>
  );
}

export default LiveTailToggle;
