"use client";

// The only code allowed to call `track` with a `files.` name (ADR-035 section 4): typed labels,
// stamped dimensions, sampling, and a guarantee that analytics never throws into UI code.
import { useContext, useEffect, useMemo, useRef } from "react";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import type { AnalyticsEvent } from "@/lib/analytics/types";
import {
  FILES_EVENTS, bucketKeyCount, slugOf,
  type ClientEventName, type FilesEventName, type LabelsOf,
} from "./events";
import {
  COALESCE_MS, createCoalescer, createTokenBucket, createWindowAggregator, realClock, type Clock,
} from "./sampling";

export type FilesComponent =
  | "FilesTab" | "FileTree" | "FileContentViewer" | "CodeEditor" | "SearchPanel" | "OpenTabsBar"
  | "QuickOpenPalette" | "StatusBar" | "MobileFilesLayout" | "MergeView" | "BannerStack" | "FileTransport";

export type KeyClass = LabelsOf<"files.mobile.extra_key_used">["key_class"];
/** Emitted through dedicated paths (rate cap, key aggregator), never by call sites. */
type InternalName = "files.meta.dropped" | "files.mobile.extra_key_used";
export type EmitName = Exclude<ClientEventName, InternalName>;
export type TimedName = { [N in EmitName]: (typeof FILES_EVENTS)[N]["category"] extends "performance" ? N : never }[EmitName];

type EmitArgs<N extends FilesEventName> = keyof LabelsOf<N> extends never ? [labels?: undefined] : [labels: LabelsOf<N>];

export interface FilesEmitter {
  emit<N extends EmitName>(name: N, ...labels: EmitArgs<N>): void;
  /** Starts a timer; the returned function emits the performance event with `durationMs` set. */
  time<N extends TimedName>(name: N): (...labels: EmitArgs<N>) => void;
  /** For a duration measured elsewhere because a label (such as scale) is only known afterwards. */
  emitTimed<N extends TimedName>(name: N, durationMs: number, ...labels: EmitArgs<N>): void;
  emitExtraKey(keyClass: KeyClass): void;
  /** Flushes aggregated events now (call on `pagehide`). */
  flush(): void;
}

interface Stamps { form: string; input: string; editing: string; v2: string }
interface Flags { editing: boolean; v2: boolean }

const MOBILE_MAX_WIDTH = 768;

function readStamps(flags: Flags): Stamps {
  const hasWindow = typeof window !== "undefined";
  const coarse = hasWindow && typeof window.matchMedia === "function" && window.matchMedia("(pointer: coarse)").matches;
  return {
    form: hasWindow && window.innerWidth < MOBILE_MAX_WIDTH ? "mobile" : "desktop",
    // Stands in for InputProfile (ADR-033) until that module lands.
    input: coarse ? "touch" : "hardware",
    editing: flags.editing ? "on" : "off",
    v2: flags.v2 ? "on" : "off",
  };
}

interface Core {
  flags: Flags;
  scope(component: FilesComponent, sessionId?: string): FilesEmitter;
}

function createCore(track: (e: AnalyticsEvent) => void, clock: Clock): Core {
  const flagsHolder = { flags: { editing: false, v2: true } as Flags };
  const send = (event: Omit<AnalyticsEvent, "labels"> & { labels?: Record<string, string> }, sessionId?: string, component: FilesComponent = "FilesTab") => {
    try {
      track({ ...event, sessionId, component, labels: { ...event.labels, ...readStamps(flagsHolder.flags) } });
    } catch {
      // analytics must never break the viewer
    }
  };
  const allowed = createCoalescer(clock);
  const bucket = createTokenBucket((slug, count) => {
    send({ name: "files.meta.dropped", category: "performance", labels: { event: slug, count: bucketKeyCount(count) } });
  }, clock);
  const keys = createWindowAggregator((keyClass, count) => {
    send({ name: "files.mobile.extra_key_used", category: "user_action", labels: { key_class: keyClass, count: bucketKeyCount(count) } });
  }, clock);

  const scope: Core["scope"] = (component, sessionId) => {
    const emitAt = (name: EmitName, labels: object | undefined, durationMs?: number) => {
      try {
        const window_ = COALESCE_MS[name];
        if (window_ !== undefined && !allowed(name, window_)) return;
        if (!bucket(slugOf(name))) return;
        send({ name, category: FILES_EVENTS[name].category, durationMs, labels: labels as Record<string, string> }, sessionId, component);
      } catch {
        // swallowed: see send()
      }
    };
    const emitter: FilesEmitter = {
      emit: (name, ...args) => emitAt(name, args[0]),
      emitTimed: (name, durationMs, ...args) => emitAt(name, args[0], durationMs),
      time: (name) => {
        const startedAt = clock.now();
        return (...args) => emitAt(name, args[0], clock.now() - startedAt);
      },
      emitExtraKey: (keyClass) => keys.add(keyClass),
      flush: () => keys.flush(),
    };
    return emitter;
  };
  return {
    get flags() { return flagsHolder.flags; },
    set flags(f: Flags) { flagsHolder.flags = f; },
    scope,
  };
}

/** Test seam: builds a standalone emitter without React. */
export function createFilesEmitter(track: (e: AnalyticsEvent) => void, clock: Clock = realClock, flags?: Partial<Flags>, component: FilesComponent = "FilesTab", sessionId?: string): FilesEmitter {
  const core = createCore(track, clock);
  core.flags = { ...core.flags, ...flags };
  return core.scope(component, sessionId);
}

const cores = new WeakMap<object, Core>();

const NOOP: FilesEmitter = { emit: () => undefined, emitTimed: () => undefined, time: () => () => undefined, emitExtraKey: () => undefined, flush: () => undefined };

export function useFilesAnalytics(opts: { component?: FilesComponent; sessionId?: string } = {}): FilesEmitter {
  const ctx = useContext(AnalyticsContext);
  const { flags } = useFeatureFlags();
  const component = opts.component ?? "FilesTab";
  const core = useMemo(() => {
    if (!ctx) return null;
    let shared = cores.get(ctx);
    if (!shared) {
      shared = createCore((e) => ctx.track(e), realClock);
      cores.set(ctx, shared);
    }
    return shared;
  }, [ctx]);
  // Read at emit time, so a flag flip reaches the next event without a remount.
  if (core) core.flags = { editing: flags["files:editing"] ?? false, v2: flags["files:viewer-v2"] ?? true };

  const emitter = useMemo(() => core?.scope(component, opts.sessionId) ?? NOOP, [core, component, opts.sessionId]);

  useEffect(() => {
    const onHide = () => emitter.flush();
    window.addEventListener("pagehide", onHide);
    return () => window.removeEventListener("pagehide", onHide);
  }, [emitter]);

  return emitter;
}
