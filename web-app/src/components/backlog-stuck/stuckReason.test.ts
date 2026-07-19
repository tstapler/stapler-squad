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

    it("falls back to the UNSPECIFIED label/class for an out-of-range value", () => {
      const bogus = 999 as StuckReason;
      expect(getStuckReasonLabel(bogus)).toBe(getStuckReasonLabel(StuckReason.UNSPECIFIED));
      expect(getStuckReasonClass(bogus)).toBe(getStuckReasonClass(StuckReason.UNSPECIFIED));
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
