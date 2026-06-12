"use client";

// +feature: approval-rules:yaml-import

import { useState } from "react";
import { Modal, ModalContent, ModalTitle, ModalClose } from "@/components/ui/Modal";
import { type ApprovalRuleProto } from "@/gen/session/v1/types_pb";
import { useValidateRules } from "@/lib/hooks/useValidateRules";
import { useBulkUpsertRules } from "@/lib/hooks/useBulkUpsertRules";
import { ParsedRuleCard, type ParsedRuleCardStatus } from "./ParsedRuleCard";
import {
  yamlTextarea,
  yamlTextareaLoading,
  previewList,
  duplicateRadioGroup,
  duplicateRadioLabel,
  applyButton,
  noValidRulesMessage,
  exampleToggle,
  exampleBlock,
  sectionLabel,
  formStack,
  partialErrorBanner,
} from "./ImportRulesModal.css";
import { ruleModalContent, modalHeader, modalTitleRow, modalBody, modalCloseButton } from "./ApprovalRulesPanel.css";

const EXAMPLE_YAML = `rules:
- name: Allow git log
  tool: Bash
  programs:
    - git
  subcommands:
    - log
  decision: allow
  reason: Read-only git history
- name: Deny git push
  tool: Bash
  programs:
    - git
  subcommands:
    - push
  decision: deny
`;

interface ImportRulesModalProps {
  open: boolean;
  onClose: () => void;
  onApplied: () => void;
  existingRules: ApprovalRuleProto[];
}

type DuplicateMode = "skip" | "overwrite";

/**
 * ImportRulesModal allows users to paste YAML rules, see live validation results,
 * and apply valid rules in bulk.
 */
export function ImportRulesModal({ open, onClose, onApplied, existingRules }: ImportRulesModalProps) {
  const [yamlContent, setYamlContent] = useState("");
  const [duplicateMode, setDuplicateMode] = useState<DuplicateMode>("skip");
  const [showExample, setShowExample] = useState(false);
  const [partialErrors, setPartialErrors] = useState<string[]>([]);

  const { results, loading: validating, validCount, errorCount } = useValidateRules(yamlContent);
  const { applyRules, loading: applying, error: applyError } = useBulkUpsertRules();

  // Build a set of existing rule names for duplicate detection.
  const existingNames = new Set(existingRules.map((r) => r.name));

  // Determine card status for each result.
  const cardStatus = (originalName: string, valid: boolean): ParsedRuleCardStatus => {
    if (!valid) return "error";
    if (existingNames.has(originalName)) {
      return duplicateMode === "overwrite" ? "overwrite" : "skip";
    }
    return "valid";
  };

  // Count truly applicable valid rules (valid + not skipped).
  const applicableCount = results.filter((r) => {
    if (!r.valid) return false;
    if (existingNames.has(r.originalName) && duplicateMode === "skip") return false;
    return true;
  }).length;

  const handleApply = async () => {
    setPartialErrors([]);
    const validRules = results
      .filter((r) => {
        if (!r.valid || !r.rule) return false;
        if (existingNames.has(r.originalName) && duplicateMode === "skip") return false;
        return true;
      })
      .map((r) => r.rule!);

    const { errors } = await applyRules(validRules, duplicateMode === "overwrite");

    if (errors.length > 0) {
      setPartialErrors(errors);
      return;
    }

    onApplied();
    onClose();
    setYamlContent("");
    setPartialErrors([]);
  };

  const applyButtonLabel = () => {
    if (errorCount > 0 && validCount > 0) {
      const errLabel = errorCount === 1 ? "1 has errors" : `${errorCount} have errors`;
      return `Apply ${validCount} rules (${errLabel})`;
    }
    return `Apply ${validCount} rules`;
  };

  return (
    <Modal open={open} onOpenChange={(o) => { if (!o) { onClose(); setYamlContent(""); setPartialErrors([]); } }}>
      <ModalContent
        showClose={false}
        className={ruleModalContent}
      >
        <div className={modalHeader}>
          <div className={modalTitleRow}>
            <ModalTitle>Import Rules from YAML</ModalTitle>
          </div>
          <ModalClose className={modalCloseButton} aria-label="Close dialog">
            ×
          </ModalClose>
        </div>

        <div className={modalBody}>
          <div className={formStack}>
            {/* YAML input */}
            <div>
              <textarea
                className={validating ? yamlTextareaLoading : yamlTextarea}
                value={yamlContent}
                onChange={(e) => setYamlContent(e.target.value)}
                placeholder="Paste your rules YAML here…"
                aria-label="YAML rules input"
                data-testid="yaml-input"
                rows={10}
              />
              <button
                type="button"
                className={exampleToggle}
                onClick={() => setShowExample((v) => !v)}
                data-testid="show-example-toggle"
              >
                {showExample ? "Hide example" : "Show example YAML"}
              </button>
              {showExample && (
                <pre className={exampleBlock}>{EXAMPLE_YAML}</pre>
              )}
            </div>

            {/* Preview */}
            {results.length > 0 && (
              <div>
                <div className={sectionLabel}>
                  Preview ({validCount} valid, {errorCount} with errors)
                </div>
                <div className={previewList} data-testid="preview-list">
                  {results.map((result, i) => (
                    <ParsedRuleCard
                      key={i}
                      result={result}
                      status={cardStatus(result.originalName, result.valid)}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* No valid rules message */}
            {results.length > 0 && validCount === 0 && (
              <p className={noValidRulesMessage} data-testid="no-valid-rules-message">
                No valid rules to apply. Fix the errors above and try again.
              </p>
            )}

            {/* Duplicate handling */}
            {validCount > 0 && (
              <div>
                <div className={sectionLabel}>Duplicate handling</div>
                <div className={duplicateRadioGroup} data-testid="duplicate-mode-group">
                  <label className={duplicateRadioLabel}>
                    <input
                      type="radio"
                      name="duplicateMode"
                      value="skip"
                      checked={duplicateMode === "skip"}
                      onChange={() => setDuplicateMode("skip")}
                      data-testid="duplicate-mode-skip"
                    />
                    Skip existing (default)
                  </label>
                  <label className={duplicateRadioLabel}>
                    <input
                      type="radio"
                      name="duplicateMode"
                      value="overwrite"
                      checked={duplicateMode === "overwrite"}
                      onChange={() => setDuplicateMode("overwrite")}
                      data-testid="duplicate-mode-overwrite"
                    />
                    Overwrite existing
                  </label>
                </div>
              </div>
            )}

            {/* Partial apply errors */}
            {(partialErrors.length > 0 || applyError) && (
              <div className={partialErrorBanner} data-testid="partial-error-banner">
                {partialErrors.length > 0
                  ? partialErrors.join(", ")
                  : applyError?.message}
              </div>
            )}

            {/* Apply button */}
            <button
              type="button"
              className={applyButton}
              disabled={applicableCount === 0 || applying || validating}
              onClick={handleApply}
              data-testid="apply-button"
            >
              {applying ? "Applying…" : applyButtonLabel()}
            </button>
          </div>
        </div>
      </ModalContent>
    </Modal>
  );
}
