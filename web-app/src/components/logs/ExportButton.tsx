"use client";

import { useState, useRef, useEffect } from 'react';
import type { LogEntry } from "@/gen/session/v1/session_pb";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { container, button, dropdown, option, icon, optionLabel, optionDesc, divider } from './ExportButton.css';

interface ExportButtonProps {
  /** Logs to export */
  logs: LogEntry[];
  /** Whether export is disabled */
  disabled?: boolean;
  /** Additional class name */
  className?: string;
}

type ExportFormat = 'json' | 'csv' | 'txt';

export function ExportButton({ logs, disabled, className }: ExportButtonProps) {
  const [showDropdown, setShowDropdown] = useState(false);
  const [exporting, setExporting] = useState(false);
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

  // Format log entry for export
  const formatLogEntry = (log: LogEntry) => ({
    timestamp: log.timestamp ? timestampDate(log.timestamp).toISOString() : '',
    level: log.level,
    source: log.source || '',
    message: log.message,
  });

  // Export as JSON
  const exportJSON = () => {
    const data = logs.map(formatLogEntry);
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    downloadBlob(blob, `logs-${getTimestamp()}.json`);
  };

  // Export as CSV
  const exportCSV = () => {
    const headers = ['timestamp', 'level', 'source', 'message'];
    const rows = logs.map(log => {
      const entry = formatLogEntry(log);
      return [
        entry.timestamp,
        entry.level,
        entry.source,
        // Escape quotes and wrap in quotes if contains comma or newline
        `"${entry.message.replace(/"/g, '""')}"`,
      ].join(',');
    });
    const csv = [headers.join(','), ...rows].join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    downloadBlob(blob, `logs-${getTimestamp()}.csv`);
  };

  // Export as plain text
  const exportTXT = () => {
    const lines = logs.map(log => {
      const entry = formatLogEntry(log);
      return `[${entry.timestamp}] [${entry.level}] [${entry.source}] ${entry.message}`;
    });
    const text = lines.join('\n');
    const blob = new Blob([text], { type: 'text/plain' });
    downloadBlob(blob, `logs-${getTimestamp()}.txt`);
  };

  // Generate timestamp for filename
  const getTimestamp = () => {
    return new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
  };

  // Download blob as file
  const downloadBlob = (blob: Blob, filename: string) => {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  // Handle export
  const handleExport = async (format: ExportFormat) => {
    setExporting(true);
    setShowDropdown(false);

    try {
      switch (format) {
        case 'json':
          exportJSON();
          break;
        case 'csv':
          exportCSV();
          break;
        case 'txt':
          exportTXT();
          break;
      }
    } finally {
      setExporting(false);
    }
  };

  // Copy to clipboard
  const copyToClipboard = async () => {
    setShowDropdown(false);
    const lines = logs.map(log => {
      const entry = formatLogEntry(log);
      return `[${entry.timestamp}] [${entry.level}] [${entry.source}] ${entry.message}`;
    });
    await navigator.clipboard.writeText(lines.join('\n'));
  };

  return (
    <div className={`${container} ${className || ''}`} ref={containerRef}>
      <button
        className={button}
        onClick={() => setShowDropdown(!showDropdown)}
        disabled={disabled || logs.length === 0 || exporting}
        aria-haspopup="menu"
        aria-expanded={showDropdown}
        aria-label={`Export ${logs.length} logs`}
        title={logs.length === 0 ? 'No logs to export' : `Export ${logs.length} logs`}
      >
        {exporting ? '...' : '↓'} Export
      </button>

      {showDropdown && (
        <div className={dropdown} role="menu">
          <button
            className={option}
            onClick={() => handleExport('json')}
            role="menuitem"
          >
            <span className={icon}>{ }</span>
            <span className={optionLabel}>JSON</span>
            <span className={optionDesc}>Structured data</span>
          </button>
          <button
            className={option}
            onClick={() => handleExport('csv')}
            role="menuitem"
          >
            <span className={icon}>📊</span>
            <span className={optionLabel}>CSV</span>
            <span className={optionDesc}>Spreadsheet</span>
          </button>
          <button
            className={option}
            onClick={() => handleExport('txt')}
            role="menuitem"
          >
            <span className={icon}>📄</span>
            <span className={optionLabel}>Plain Text</span>
            <span className={optionDesc}>Simple format</span>
          </button>
          <div className={divider} />
          <button
            className={option}
            onClick={copyToClipboard}
            role="menuitem"
          >
            <span className={icon}>📋</span>
            <span className={optionLabel}>Copy All</span>
            <span className={optionDesc}>To clipboard</span>
          </button>
        </div>
      )}
    </div>
  );
}

export default ExportButton;
