// +feature: estimated-value
"use client";

import { useId } from "react";
import { estimatedValueMarker, srOnly } from "./EstimatedValue.css";

interface EstimatedValueProps {
  /** The value to render, e.g. "$5.00". Rendered with a leading "~". */
  children: React.ReactNode;
  /** Tooltip text explaining why this figure is modeled/heuristic, not measured. */
  title: string;
  className?: string;
}

// EstimatedValue is the one shared visual treatment for any modeled/heuristic
// number on the insights dashboard (per-tool cost, activity cost, cache ROI,
// waste score) — reused rather than forked across those surfaces per
// research/pitfalls.md §2 and research/ux.md §4. Renders "~{children}" with
// an aria-describedby pointing at hidden text explaining the estimation
// method, so screen-reader users get the same caveat sighted users see via
// the title tooltip.
export function EstimatedValue({ children, title, className }: EstimatedValueProps) {
  const descriptionId = useId();
  return (
    <span>
      <span
        className={className ? `${estimatedValueMarker} ${className}` : estimatedValueMarker}
        title={title}
        aria-describedby={descriptionId}
      >
        ~{children}
      </span>
      <span id={descriptionId} className={srOnly}>
        {title}
      </span>
    </span>
  );
}
