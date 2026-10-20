/** Counts the badge rule composes; peer and answer inputs are supplied by their own epics. */
export interface BadgeInputs {
  unreadActionable: number;
  unconfirmedAnswers: number;
  unacknowledgedFailures: number;
  escalatedPeerRequests: number;
  /** Plain `sent` answers, Finished, Stopped-outcome-unknown, running, departed, un-escalated peer help, Reminders: never counted. */
  ignored?: number;
}

export interface BadgeCounts {
  bell: number;
  /** The Background tab shows only the exceptions part. */
  background: number;
}

export function badgeCount(i: BadgeInputs): BadgeCounts {
  return {
    bell: i.unreadActionable + i.unconfirmedAnswers + i.unacknowledgedFailures + i.escalatedPeerRequests,
    background: i.unacknowledgedFailures,
  };
}
