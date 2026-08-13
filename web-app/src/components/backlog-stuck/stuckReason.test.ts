import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason } from "@/gen/session/v1/backlog_pb";
import {
  getStuckReasonLabel,
  getStuckReasonClass,
  getStuckReasonIcon,
  isPrStatusUnknown,
  formatStuckDuration,
  formatAgo,
  PR_STATUS_STALE_THRESHOLD_MS,
} from "./stuckReason";

const ALL_REASONS: StuckReason[] = [
  StuckReason.UNSPECIFIED,
  StuckReason.PR_READY_UNMERGED,
  StuckReason.REWORK_CAP,
  StuckReason.ABANDONED_REVIEW,
  StuckReason.STALE_WORK,
  StuckReason.BOUNCING,
  StuckReason.PUSH_FAILED,
  StuckReason.REWORK_BLOCKED_STALE,
  StuckReason.PR_NEEDS_FIX,
  StuckReason.RESPAWN_BLOCKED_ACTIVE,
  StuckReason.LIKELY_FLAKY,
  StuckReason.BLOCKED_BY_DEPENDENCY,
];

describe("stuckReason", () => {
  describe("getStuckReasonLabel_should_returnTextLabelForEveryReason_When_MappedExhaustively", () => {
    it("returns a non-empty text label for every StuckReason enum value", () => {
      for (const reason of ALL_REASONS) {
        const label = getStuckReasonLabel(reason);
        expect(typeof label).toBe("string");
        expect(label.length).toBeGreaterThan(0);
      }
    });

    it("pairs every reason with a non-empty class (color + text, never color-only)", () => {
      for (const reason of ALL_REASONS) {
        const cls = getStuckReasonClass(reason);
        expect(cls).toBeTruthy();
        expect(String(cls).length).toBeGreaterThan(0);
      }
    });

    it("pairs every reason with a decorative icon", () => {
      for (const reason of ALL_REASONS) {
        expect(getStuckReasonIcon(reason).length).toBeGreaterThan(0);
      }
    });

    it("gives push_failed a distinct, descriptive label", () => {
      expect(getStuckReasonLabel(StuckReason.PUSH_FAILED)).toMatch(/push/i);
    });

    it("gives pr_needs_fix a real label, not the Unknown-reason fallback", () => {
      expect(getStuckReasonLabel(StuckReason.PR_NEEDS_FIX)).not.toBe(
        getStuckReasonLabel(StuckReason.UNSPECIFIED)
      );
      expect(getStuckReasonClass(StuckReason.PR_NEEDS_FIX)).not.toBe(
        getStuckReasonClass(StuckReason.UNSPECIFIED)
      );
    });

    it("gives respawn_blocked_active a real label, not the Unknown-reason fallback", () => {
      expect(getStuckReasonLabel(StuckReason.RESPAWN_BLOCKED_ACTIVE)).not.toBe(
        getStuckReasonLabel(StuckReason.UNSPECIFIED)
      );
      expect(getStuckReasonClass(StuckReason.RESPAWN_BLOCKED_ACTIVE)).not.toBe(
        getStuckReasonClass(StuckReason.UNSPECIFIED)
      );
    });

    it("gives likely_flaky a real label, class, and icon, not the Unknown-reason fallback", () => {
      expect(getStuckReasonLabel(StuckReason.LIKELY_FLAKY)).not.toBe(
        getStuckReasonLabel(StuckReason.UNSPECIFIED)
      );
      expect(getStuckReasonClass(StuckReason.LIKELY_FLAKY)).not.toBe(
        getStuckReasonClass(StuckReason.UNSPECIFIED)
      );
      expect(getStuckReasonIcon(StuckReason.LIKELY_FLAKY).length).toBeGreaterThan(0);
      // Copy must read as a hint to verify, not a confident verdict (validation.md).
      expect(getStuckReasonLabel(StuckReason.LIKELY_FLAKY)).toMatch(/possibly|verify/i);
    });

    it("falls back to the UNSPECIFIED label/class for an out-of-range value", () => {
      const bogus = 999 as StuckReason;
      expect(getStuckReasonLabel(bogus)).toBe(getStuckReasonLabel(StuckReason.UNSPECIFIED));
      expect(getStuckReasonClass(bogus)).toBe(getStuckReasonClass(StuckReason.UNSPECIFIED));
    });
  });

  // review-gate-stale-session-rework Story 2.2.1: REWORK_BLOCKED_STALE must be
  // visually distinguishable from the closely-related STALE_WORK (same
  // underlying "stale work session" concept, different item status/urgency)
  // — never identical copy, never color-only differentiation. The generic
  // exhaustiveness tests above already confirm REWORK_BLOCKED_STALE has a
  // non-empty label/class/icon; these two assert the specific distinctness
  // that generic coverage can't check.
  describe("getStuckReasonLabel_should_returnDistinctLabel_When_ReworkBlockedStale", () => {
    it("is neither equal to nor a substring of STALE_WORK's label", () => {
      const reworkBlockedLabel = getStuckReasonLabel(StuckReason.REWORK_BLOCKED_STALE);
      const staleWorkLabel = getStuckReasonLabel(StuckReason.STALE_WORK);
      expect(reworkBlockedLabel).not.toBe(staleWorkLabel);
      expect(staleWorkLabel).not.toContain(reworkBlockedLabel);
      expect(reworkBlockedLabel).not.toContain(staleWorkLabel);
    });
  });

  describe("getStuckReasonIcon_should_returnNonFallbackIcon_When_ReworkBlockedStale", () => {
    it("returns an icon distinct from STALE_WORK's and from UNSPECIFIED's fallback, paired with a non-empty label", () => {
      const icon = getStuckReasonIcon(StuckReason.REWORK_BLOCKED_STALE);
      expect(icon).not.toBe(getStuckReasonIcon(StuckReason.UNSPECIFIED));
      expect(icon).not.toBe(getStuckReasonIcon(StuckReason.STALE_WORK));
      // Icon must never be the sole signal — a text label always accompanies it.
      expect(getStuckReasonLabel(StuckReason.REWORK_BLOCKED_STALE).length).toBeGreaterThan(0);
    });
  });

  describe("isPrStatusUnknown", () => {
    it("is false for non pr_ready_unmerged reasons regardless of staleness", () => {
      const stale = timestampFromDate(new Date(Date.now() - 60 * 60 * 1000));
      expect(
        isPrStatusUnknown({ reason: StuckReason.REWORK_CAP, lastCheckedAt: stale })
      ).toBe(false);
    });

    it("is false when last_checked_at is within the 5-minute staleness threshold", () => {
      const fresh = timestampFromDate(new Date(Date.now() - 60 * 1000));
      expect(
        isPrStatusUnknown({ reason: StuckReason.PR_READY_UNMERGED, lastCheckedAt: fresh })
      ).toBe(false);
    });

    it("is true once last_checked_at exceeds the 5-minute staleness threshold", () => {
      const stale = timestampFromDate(
        new Date(Date.now() - PR_STATUS_STALE_THRESHOLD_MS - 60_000)
      );
      expect(
        isPrStatusUnknown({ reason: StuckReason.PR_READY_UNMERGED, lastCheckedAt: stale })
      ).toBe(true);
    });

    it("is true when last_checked_at is missing entirely", () => {
      expect(
        isPrStatusUnknown({ reason: StuckReason.PR_READY_UNMERGED, lastCheckedAt: undefined })
      ).toBe(true);
    });
  });

  describe("formatStuckDuration", () => {
    it("formats a multi-day duration as 'Nd'", () => {
      const since = timestampFromDate(new Date(Date.now() - 3 * 24 * 60 * 60 * 1000));
      expect(formatStuckDuration(since)).toBe("3d");
    });

    it("formats a sub-day duration as 'Nh'", () => {
      const since = timestampFromDate(new Date(Date.now() - 2 * 60 * 60 * 1000));
      expect(formatStuckDuration(since)).toBe("2h");
    });

    it("formats a sub-hour duration as 'Nm'", () => {
      const since = timestampFromDate(new Date(Date.now() - 18 * 60 * 1000));
      expect(formatStuckDuration(since)).toBe("18m");
    });
  });

  describe("formatAgo", () => {
    it("formats minutes as 'Nm ago'", () => {
      const ts = timestampFromDate(new Date(Date.now() - 47 * 60 * 1000));
      expect(formatAgo(ts)).toBe("47m ago");
    });

    it("returns 'unknown' for an undefined timestamp", () => {
      expect(formatAgo(undefined)).toBe("unknown");
    });
  });
});
