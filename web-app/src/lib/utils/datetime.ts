/**
 * Utility functions for date and time formatting with timezone support
 */

/**
 * Format a timestamp for display with full date, time, and timezone
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM PST")
 */
export function formatTimestamp(date: Date | number): string {
  const dateObj = typeof date === 'number' ? new Date(date) : date;

  return dateObj.toLocaleString(undefined, {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
    timeZoneName: 'short',
  });
}

/**
 * Format a timestamp for display with date and time (no timezone)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM")
 */
export function formatTimestampShort(date: Date | number): string {
  const dateObj = typeof date === 'number' ? new Date(date) : date;

  return dateObj.toLocaleString(undefined, {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  });
}

/**
 * Format a timestamp for display with just time (no date)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "3:45:30 PM")
 */
export function formatTime(date: Date | number): string {
  const dateObj = typeof date === 'number' ? new Date(date) : date;

  return dateObj.toLocaleTimeString(undefined, {
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  });
}

/**
 * Format a timestamp as relative time (e.g., "5m ago", "2h ago")
 * @param timestamp - Unix timestamp in milliseconds
 * @returns Relative time string
 */
export function formatRelativeTime(timestamp: number): string {
  const date = new Date(timestamp);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "Just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

/**
 * Get the user's timezone abbreviation (e.g., "PST", "EST", "UTC")
 */
export function getUserTimezone(): string {
  const date = new Date();
  const timeZoneName = date.toLocaleString(undefined, { timeZoneName: 'short' }).split(' ').pop();
  return timeZoneName || Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/**
 * Time range preset configuration
 */
export interface TimeRangePreset {
  label: string;
  value: string;
  getRange: () => { start: Date; end: Date };
}

/**
 * Standard time range presets for log filtering
 */
export const TIME_RANGE_PRESETS: TimeRangePreset[] = [
  {
    label: 'Last 5 minutes',
    value: '5m',
    getRange: () => ({
      start: new Date(Date.now() - 5 * 60 * 1000),
      end: new Date(),
    }),
  },
  {
    label: 'Last 15 minutes',
    value: '15m',
    getRange: () => ({
      start: new Date(Date.now() - 15 * 60 * 1000),
      end: new Date(),
    }),
  },
  {
    label: 'Last 1 hour',
    value: '1h',
    getRange: () => ({
      start: new Date(Date.now() - 60 * 60 * 1000),
      end: new Date(),
    }),
  },
  {
    label: 'Last 4 hours',
    value: '4h',
    getRange: () => ({
      start: new Date(Date.now() - 4 * 60 * 60 * 1000),
      end: new Date(),
    }),
  },
  {
    label: 'Last 24 hours',
    value: '24h',
    getRange: () => ({
      start: new Date(Date.now() - 24 * 60 * 60 * 1000),
      end: new Date(),
    }),
  },
  {
    label: 'Last 7 days',
    value: '7d',
    getRange: () => ({
      start: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
      end: new Date(),
    }),
  },
  {
    label: 'All time',
    value: 'all',
    getRange: () => ({
      start: new Date(0),
      end: new Date(),
    }),
  },
];

/**
 * Get a time range preset by its value
 */
export function getTimeRangePreset(value: string): TimeRangePreset | undefined {
  return TIME_RANGE_PRESETS.find(preset => preset.value === value);
}

/**
 * Format a time range for display
 */
export function formatTimeRangeLabel(start: Date, end: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - start.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  // Check if end is "now" (within last minute)
  const isEndNow = now.getTime() - end.getTime() < 60000;

  if (isEndNow) {
    if (diffMins <= 5) return 'Last 5 minutes';
    if (diffMins <= 15) return 'Last 15 minutes';
    if (diffHours <= 1) return 'Last hour';
    if (diffHours <= 4) return 'Last 4 hours';
    if (diffHours <= 24) return 'Last 24 hours';
    if (diffDays <= 7) return 'Last 7 days';
  }

  // Custom range format
  return `${formatTimestampShort(start)} - ${formatTimestampShort(end)}`;
}
