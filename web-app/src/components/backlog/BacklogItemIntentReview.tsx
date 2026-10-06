"use client";
// +feature: backlog:intent-review

import { useEffect, useState } from "react";
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

/**
 * Wraps ParseBacklogItemIntent with the omnibar review UI's loading/result
 * state — mirrors the abandoned llm-omnibar design's useParseIntent.ts shape,
 * scoped down to this feature's needs.
 */
function useParseBacklogItemIntent(initialText: string) {
  const { parseBacklogItemIntent } = useBacklogService();
  const [phase, setPhase] = useState<"parsing" | "review">("parsing");
  const [initialValues, setInitialValues] = useState<Partial<BacklogItem>>({});
  const [parseFailed, setParseFailed] = useState(false);

  // `active` guards against a stale parse response applying after the user
  // has already moved on (e.g. pressed Escape) before the in-flight RPC
  // resolves — mirrors the abandoned design's "abort-on-change" intent
  // without needing an AbortController, since the RPC has no side effects to
  // actually cancel.
  useEffect(() => {
    let active = true;
    setPhase("parsing");
    parseBacklogItemIntent(initialText).then((draft) => {
      if (!active) return;
      setParseFailed(!draft);
      setInitialValues(draftToInitialValues(draft, initialText));
      setPhase("review");
    });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once per mount; initialText is fixed for this hook's lifetime
  }, []);

  return { phase, initialValues, parseFailed };
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
 * Renders the review UI for a parsed "backlog: <message>" draft in place of
 * the old fire-and-close flow. Reuses BacklogItemForm for edit/create
 * (including its pipeline-mode and auto-spawn-session controls); falls back
 * to the raw text on parse failure so nothing is lost.
 */
export function BacklogItemIntentReview({
  initialText,
  onDone,
  onCancel,
}: BacklogItemIntentReviewProps) {
  const { createBacklogItem } = useBacklogService();
  const { phase, initialValues, parseFailed } = useParseBacklogItemIntent(initialText);

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
