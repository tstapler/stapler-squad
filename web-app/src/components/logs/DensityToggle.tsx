"use client";

import styles from './DensityToggle.module.css';

export type LogDensity = 'compact' | 'comfortable' | 'spacious';

interface DensityToggleProps {
  /** Current density setting */
  value: LogDensity;
  /** Change handler */
  onChange: (density: LogDensity) => void;
  /** Additional class name */
  className?: string;
}

const DENSITY_OPTIONS: { value: LogDensity; label: string; icon: string }[] = [
  { value: 'compact', label: 'Compact', icon: '≡' },
  { value: 'comfortable', label: 'Comfortable', icon: '☰' },
  { value: 'spacious', label: 'Spacious', icon: '▤' },
];

export function DensityToggle({ value, onChange, className }: DensityToggleProps) {
  return (
    <div className={`${styles.container} ${className || ''}`} role="radiogroup" aria-label="Log density">
      {DENSITY_OPTIONS.map((option) => (
        <button
          key={option.value}
          className={`${styles.option} ${value === option.value ? styles.active : ''}`}
          onClick={() => onChange(option.value)}
          role="radio"
          aria-checked={value === option.value}
          aria-label={option.label}
          title={option.label}
          type="button"
        >
          {option.icon}
        </button>
      ))}
    </div>
  );
}

export default DensityToggle;
