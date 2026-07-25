"use client";

import { useEffect, useRef } from "react";
import { AutoDecision } from "@/gen/session/v1/types_pb";
import { RULE_TEMPLATES, RuleTemplate } from "@/lib/ruleTemplates";
import {
  overlay, dialog, dialogTitle, dialogSubtitle, grid,
  card, cardIcon, cardTitle, cardDesc, cardDecision, footer, scratchLink,
} from "./TemplateLibrary.css";

function decisionLabel(d: AutoDecision): string {
  switch (d) {
    case AutoDecision.ALLOW: return "Auto-Allow";
    case AutoDecision.DENY: return "Auto-Deny";
    default: return "Escalate";
  }
}

interface TemplateLibraryProps {
  open: boolean;
  onClose: () => void;
  onSelect: (template: RuleTemplate) => void;
}

export function TemplateLibrary({ open, onClose, onSelect }: TemplateLibraryProps) {
  const dialogRef = useRef<HTMLDivElement>(null);

  // Focus trap + Escape key
  useEffect(() => {
    if (!open) return;
    dialogRef.current?.focus();
    const handleKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className={overlay}
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
      role="dialog"
      aria-modal="true"
      aria-label="Choose a rule template"
    >
      <div className={dialog} ref={dialogRef} tabIndex={-1}>
        <h2 className={dialogTitle}>Start from a template</h2>
        <p className={dialogSubtitle}>
          Select a starting point. All fields can be edited before saving.
        </p>

        <div className={grid}>
          {RULE_TEMPLATES.map((tpl) => (
            <button
              key={tpl.id}
              className={card}
              onClick={() => { onSelect(tpl); onClose(); }}
            >
              <span className={cardIcon}>{tpl.icon}</span>
              <span className={cardTitle}>{tpl.title}</span>
              <span className={cardDesc}>{tpl.description}</span>
              <span className={cardDecision}>{decisionLabel(tpl.decision)}</span>
            </button>
          ))}
        </div>

        <div className={footer}>
          <button className={scratchLink} onClick={onClose}>
            Start from scratch instead
          </button>
        </div>
      </div>
    </div>
  );
}
