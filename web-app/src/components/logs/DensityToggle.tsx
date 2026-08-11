"use client";

import { container, option, active } from './DensityToggle.css';

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
    <div className={`${container} ${className || ''}`} role="radiogroup" aria-label="Log density">
      {DENSITY_OPTIONS.map((opt) => (
        <button
          key={opt.value}
          className={`${option} ${value === opt.value ? active : ''}`}
          onClick={() => onChange(opt.value)}
          role="radio"
          aria-checked={value === opt.value}
          aria-label={opt.label}
          title={opt.label}
          type="button"
        >
          {opt.icon}
        </button>
      ))}
    </div>
  );
}

export default DensityToggle;
