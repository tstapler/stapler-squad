"use client";
// +feature: backlog:intent-review

import { useEffect, useRef, useState } from "react";
import { BacklogItemForm } from "./BacklogItemForm";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { BacklogItem, BacklogItemInput, ParsedBacklogItemDraft } from "@/lib/hooks/useBacklogService";

interface BacklogItemIntentReviewProps {
  /** The omnibar's raw free-text message ("backlog: <this>"). */
  initialText: string;
  /** Called once the item is actually created (after the user's edits). */
  onDone: (item: BacklogItem) => void;
  onCancel: () => void;
}

/** Truncates a message's first line for a client-side fallback title — mirrors
 * the server's deriveChatItemTitle (backlog_service_chat.go), used here only
 * while parsing is in flight or failed, never sent to the server directly. */
function fallbackTitle(message: string): string {
  const firstLine = message.split("\n")[0].trim();
  return firstLine.length > 80 ? firstLine.slice(0, 80) : firstLine;
}

/** Builds BacklogItemForm's initialValues from a parsed draft, or — when
 * draft is null (parse failed/timed out) — from the raw text, so the user's
 * typed message is never silently lost. */
function draftToInitialValues(
  draft: ParsedBacklogItemDraft | null,
  rawText: string
): Partial<BacklogItem> {
  if (!draft) {
    return { title: fallbackTitle(rawText), description: rawText };
  }
  return {
    title: draft.title,
    description: draft.description,
    acCriteria: draft.acceptanceCriteria.map((text, index) => ({
      index,
      text,
      status: "pending",
    })),
  };
}

function ParsingIndicator() {
  return (
    <div role="status" aria-live="polite" style={{ padding: "24px", textAlign: "center", color: "var(--text-secondary)" }}>
      Parsing your message into a backlog item…
    </div>
  );
}

function ParseFailedBanner() {
  return (
    <p
      role="alert"
      data-testid="backlog-intent-review-parse-failed-banner"
      style={{
        margin: "0 0 12px",
        padding: "8px 12px",
        fontSize: "13px",
        borderRadius: "6px",
        background: "var(--warning-bg, rgba(234, 179, 8, 0.12))",
        color: "var(--warning-text, #b45309)",
      }}
    >
      Couldn&rsquo;t parse this into a structured item — review the fields below before creating.
    </p>
  );
}

/**
 * Renders in place of the omnibar's fire-and-close raw-text creation once a
 * "backlog: <message>" submission is parsed by an LLM (ParseBacklogItemIntent)
 * into a structured draft. Reuses BacklogItemForm wholesale for the actual
 * review/edit/create UI (title/description/AC list, pipeline mode incl. the
 * existing SDD-handoff opt-in, auto-spawn-session checkbox) rather than
 * building a parallel form — see architecture.md's "reuse, don't duplicate"
 * recommendation. On parse failure, pre-fills the same form with the raw
 * text instead of losing it, per the "no silent misparse" acceptance
 * criterion.
 */
export function BacklogItemIntentReview({
  initialText,
  onDone,
  onCancel,
}: BacklogItemIntentReviewProps) {
  const { parseBacklogItemIntent, createBacklogItem } = useBacklogService();
  const [phase, setPhase] = useState<"parsing" | "review">("parsing");
  const [initialValues, setInitialValues] = useState<Partial<BacklogItem>>({});
  const [parseFailed, setParseFailed] = useState(false);

  // Guards against a stale parse response applying after the user has already
  // moved on (e.g. pressed Escape, or the omnibar re-opened with new text)
  // before the in-flight RPC resolves — mirrors the abandoned llm-omnibar
  // design's "abort-on-change" intent without needing an AbortController,
  // since ParseBacklogItemIntent has no side effects to actually cancel.
  const cancelledRef = useRef(false);
  useEffect(() => {
    cancelledRef.current = false;
    return () => {
      cancelledRef.current = true;
    };
  }, []);

  useEffect(() => {
    let active = true;
    setPhase("parsing");
    parseBacklogItemIntent(initialText).then((draft) => {
      if (!active || cancelledRef.current) return;
      setParseFailed(!draft);
      setInitialValues(draftToInitialValues(draft, initialText));
      setPhase("review");
    });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once per mount; initialText is fixed for this component's lifetime
  }, []);

  const handleSubmit = async (data: BacklogItemInput) => {
    const result = await createBacklogItem(data);
    if (result) onDone(result.item);
  };

  if (phase === "parsing") {
    return <ParsingIndicator />;
  }

  return (
    <div>
      {parseFailed && <ParseFailedBanner />}
      <BacklogItemForm
        initialValues={initialValues}
        onSubmit={handleSubmit}
        onCancel={onCancel}
      />
    </div>
  );
}
