"use client";

import { createContext, useContext } from "react";

/**
 * Announcer API (owner: components/ui/Announcer.tsx). Every notification surface
 * speaks through this instead of rendering its own role="status" or role="alert":
 *
 *   announce(message, politeness = "polite", key?)
 *     A plain message. `key` dedupes a message still waiting in the queue (the
 *     newer text replaces it). Assertive messages are spoken before polite ones;
 *     the polite queue holds at most 5, dropping the oldest. Use for receipts and
 *     status lines: "Moved 4 to tray", "3 kept: still needs attention".
 *
 *   announceArrival({ title, pinned })
 *     A new toast or notification. Arrivals inside a 500ms window are coalesced
 *     into one message so a burst is intelligible: informational arrivals become
 *     "N new notifications" (polite); pinned ones go to the assertive channel,
 *     which is reserved for decisions that must interrupt.
 *
 * Outside an AnnouncerProvider both are no-ops, so a surface renders in tests
 * and embedded views without one.
 */
export interface AnnounceApi {
  announce: (message: string, politeness?: "polite" | "assertive", key?: string) => void;
  announceArrival: (arrival: { title: string; pinned: boolean }) => void;
}

export const NOOP_ANNOUNCE: AnnounceApi = { announce: () => {}, announceArrival: () => {} };

export const AnnouncerContext = createContext<AnnounceApi>(NOOP_ANNOUNCE);

export function useAnnounce(): AnnounceApi {
  return useContext(AnnouncerContext);
}

export const COALESCE_WINDOW_MS = 500;
/** How long a message stays in its region before the next is spoken. */
export const ANNOUNCE_HOLD_MS = 1_000;
export const POLITE_QUEUE_MAX = 5;

interface Queued {
  text: string;
  key?: string;
}

export interface AnnouncerOutput {
  setPolite: (text: string) => void;
  setAssertive: (text: string) => void;
}

type Channel = "polite" | "assertive";

/**
 * Framework-free scheduling core of the Announcer, so the queue rules are
 * unit-testable. Each channel has its own region and speaks one message at a time;
 * an assertive message never waits behind a polite one.
 */
export class AnnouncerEngine {
  private readonly queues: Record<Channel, Queued[]> = { polite: [], assertive: [] };
  private readonly speaking: Record<Channel, boolean> = { polite: false, assertive: false };
  private readonly holdTimers: Record<Channel, ReturnType<typeof setTimeout> | null> = {
    polite: null,
    assertive: null,
  };
  private arrivals: Array<{ title: string; pinned: boolean }> = [];
  private arrivalTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(private readonly out: AnnouncerOutput) {}

  announce(message: string, politeness: Channel = "polite", key?: string): void {
    const queue = this.queues[politeness];
    const existing = key === undefined ? -1 : queue.findIndex((q) => q.key === key);
    if (existing >= 0) queue[existing] = { text: message, key };
    else queue.push({ text: message, key });
    if (politeness === "polite" && queue.length > POLITE_QUEUE_MAX) queue.shift();
    this.drain(politeness);
  }

  announceArrival(arrival: { title: string; pinned: boolean }): void {
    this.arrivals.push(arrival);
    if (this.arrivalTimer !== null) return;
    this.arrivalTimer = setTimeout(() => this.flushArrivals(), COALESCE_WINDOW_MS);
  }

  dispose(): void {
    if (this.arrivalTimer !== null) clearTimeout(this.arrivalTimer);
    this.arrivalTimer = null;
    for (const channel of ["polite", "assertive"] as const) {
      const timer = this.holdTimers[channel];
      if (timer !== null) clearTimeout(timer);
      this.holdTimers[channel] = null;
    }
  }

  private flushArrivals(): void {
    this.arrivalTimer = null;
    const batch = this.arrivals;
    this.arrivals = [];
    if (batch.length === 0) return;
    const pinned = batch.filter((a) => a.pinned);
    const newest = batch[batch.length - 1];

    if (batch.length === 1) {
      if (newest.pinned) this.announce(newest.title, "assertive");
      else this.announce("1 new notification");
    } else if (pinned.length === 0) {
      this.announce(`${batch.length} new notifications`);
    } else if (pinned.length === batch.length) {
      this.announce(
        `${batch.length} notifications need attention, newest: ${newest.title}`,
        "assertive",
      );
    } else {
      const needs = pinned.length === 1 ? "1 needs attention" : `${pinned.length} need attention`;
      this.announce(`${batch.length} new notifications, ${needs}`, "assertive");
    }
  }

  private drain(channel: Channel): void {
    if (this.speaking[channel]) return;
    const next = this.queues[channel].shift();
    if (!next) return;
    const write = channel === "assertive" ? this.out.setAssertive : this.out.setPolite;
    this.speaking[channel] = true;
    write(next.text);
    this.holdTimers[channel] = setTimeout(() => {
      this.holdTimers[channel] = null;
      // Clearing lets an identical later message change the region, so it is announced again.
      write("");
      this.speaking[channel] = false;
      this.drain(channel);
    }, ANNOUNCE_HOLD_MS);
  }
}
