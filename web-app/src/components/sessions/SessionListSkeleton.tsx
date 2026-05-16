"use client";

import React from "react";
import {
  skeletonRow,
  dot,
  nameBar,
  agentPlaceholder,
  pathBar,
  timeBar,
  actionsSpacer,
} from "./SessionListSkeleton.css";

interface SessionListSkeletonProps {
  count?: number;
}

function SkeletonRow({ index }: { index: number }) {
  return (
    <div
      className={skeletonRow}
      aria-hidden="true"
      data-testid={`skeleton-row-${index}`}
    >
      <div className={dot} />
      <div className={nameBar} />
      <div className={agentPlaceholder} />
      <div className={pathBar} />
      <div className={timeBar} />
      <div className={actionsSpacer} />
    </div>
  );
}

export function SessionListSkeleton({ count = 6 }: SessionListSkeletonProps) {
  return (
    <div
      role="status"
      aria-label="Loading sessions…"
      aria-busy="true"
      data-testid="session-list-skeleton"
    >
      {Array.from({ length: count }, (_, i) => (
        <SkeletonRow key={i} index={i} />
      ))}
    </div>
  );
}
