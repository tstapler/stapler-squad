"use client";

import React, { useRef, useState, useEffect, useCallback } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
import type { LogEntry } from "@/lib/hooks/useLogViewer";
import { LogRow } from "./LogRow";
import { ExpandedLogDetail } from "./ExpandedLogDetail";
import * as styles from "./VirtualLogList.css";

interface VirtualLogListProps {
  entries: LogEntry[];
  isFollowing: boolean;
  expandedRowIndex: number | null;
  selectedRowIndex: number | null;
  searchQuery: string;
  onAtBottomStateChange: (atBottom: boolean) => void;
  onToggleRow: (index: number) => void;
  virtuosoRef?: React.RefObject<VirtuosoHandle | null>;
}

/**
 * VirtualLogList — react-virtuoso backed virtual list with live-tail follow mode.
 * Implements Epic 2: virtual scroll, followOutput, atBottomStateChange, and aria-live throttling.
 * Epic 4: accordion row expansion via ExpandedLogDetail, selectedRowIndex highlight.
 */
export function VirtualLogList({
  entries,
  isFollowing,
  expandedRowIndex,
  selectedRowIndex,
  searchQuery,
  onAtBottomStateChange,
  onToggleRow,
  virtuosoRef,
}: VirtualLogListProps) {
  // Internal ref used when caller doesn't provide one
  const internalRef = useRef<VirtuosoHandle | null>(null);
  const ref = virtuosoRef ?? internalRef;

  // Throttled aria-live announcement: announce new log count at most once per 3s
  const [announcement, setAnnouncement] = useState("");
  const announcementTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingCountRef = useRef(0);

  const scheduleAnnouncement = useCallback((count: number) => {
    pendingCountRef.current = count;
    if (!announcementTimerRef.current) {
      announcementTimerRef.current = setTimeout(() => {
        announcementTimerRef.current = null;
        if (pendingCountRef.current > 0) {
          setAnnouncement(`${pendingCountRef.current} new log entries`);
          pendingCountRef.current = 0;
        }
      }, 3000);
    }
  }, []);

  // Announce when entries grow during live tail
  const prevLengthRef = useRef(entries.length);
  useEffect(() => {
    const added = entries.length - prevLengthRef.current;
    prevLengthRef.current = entries.length;
    if (added > 0 && isFollowing) {
      scheduleAnnouncement(added);
    }
  }, [entries.length, isFollowing, scheduleAnnouncement]);

  // Cleanup timer on unmount
  useEffect(() => {
    return () => {
      if (announcementTimerRef.current) {
        clearTimeout(announcementTimerRef.current);
      }
    };
  }, []);

  return (
    <div className={styles.scrollContainer} style={{ height: "100%" }}>
      {/* Throttled screen-reader announcement — hidden visually */}
      <div
        role="status"
        aria-live="polite"
        aria-atomic="true"
        className={styles.srOnly}
      >
        {announcement}
      </div>

      <Virtuoso
        ref={ref}
        data={entries}
        followOutput={isFollowing ? "smooth" : false}
        overscan={300}
        atBottomStateChange={onAtBottomStateChange}
        itemContent={(index, entry) => (
          <React.Fragment key={entry.id}>
            <LogRow
              entry={entry}
              index={index}
              isExpanded={expandedRowIndex === index}
              isSelected={selectedRowIndex === index}
              onToggle={() => onToggleRow(index)}
              searchQuery={searchQuery}
            />
            {expandedRowIndex === index && <ExpandedLogDetail entry={entry} />}
          </React.Fragment>
        )}
        role="log"
        aria-live="polite"
        aria-label="Log output"
      />
    </div>
  );
}
