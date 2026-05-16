"use client";

import { useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import * as styles from "./OnboardingModal.css";
import { ONBOARDED_KEY } from "./useOnboarding";

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
}

const SHORTCUTS = [
  { key: "⌘K / Ctrl+K", label: "Open omnibar" },
  { key: "?", label: "Shortcut cheatsheet" },
  { key: "[", label: "Toggle nav" },
  { key: "⌘P / Ctrl+P", label: "Pause session" },
  { key: "⌘D / Ctrl+D", label: "Delete session" },
  { key: "⌘↵ / Ctrl+↵", label: "Accept approval" },
] as const;

const ASCII_DIAGRAM = `main ─┬─► worktree-A  (Claude)
      │
      └─► worktree-B  (Aider)`;

type Step = 1 | 2 | 3 | 4;

function StepIndicator({ current, total }: { current: Step; total: number }) {
  return (
    <div className={styles.stepIndicatorRow}>
      {Array.from({ length: total }, (_, i) => {
        const stepNum = i + 1;
        let dotClass = styles.dot;
        if (stepNum < current) dotClass = `${styles.dot} ${styles.dotCompleted}`;
        if (stepNum === current) dotClass = `${styles.dot} ${styles.dotActive}`;
        return <span key={stepNum} className={dotClass} aria-hidden="true" />;
      })}
    </div>
  );
}

export function OnboardingModal({ isOpen, onClose }: OnboardingModalProps) {
  const [step, setStep] = useState<Step>(1);
  const [dontShowAgain, setDontShowAgain] = useState(true);
  const { open: openOmnibar } = useOmnibar();

  const handleSkip = () => {
    try {
      localStorage.setItem("stapler-squad:onboarded", "true");
    } catch {
      // ignore storage errors
    }
    onClose();
  };

  const handleNext = () => {
    if (step < 4) {
      setStep((prev) => (prev + 1) as Step);
    }
  };

  const handleBack = () => {
    if (step > 1) {
      setStep((prev) => (prev - 1) as Step);
    }
  };

  const handleGetStarted = () => {
    if (dontShowAgain) {
      try {
        localStorage.setItem(ONBOARDED_KEY, "true");
      } catch {
        // ignore storage errors
      }
    }
    onClose();
  };

  const handleTryOmnibar = () => {
    onClose();
    // Small delay to let modal close animation finish before opening omnibar
    setTimeout(() => openOmnibar(), 100);
  };

  const handleViewShortcuts = () => {
    onClose();
    // Dispatch a custom event that CockpitShell listens for
    setTimeout(() => {
      window.dispatchEvent(new CustomEvent("stapler-squad:open-shortcuts"));
    }, 100);
  };

  // Reset step when modal opens
  const handleOpenChange = (open: boolean) => {
    if (open) {
      setStep(1);
      setDontShowAgain(true);
    }
    if (!open) {
      onClose();
    }
  };

  return (
    <Dialog.Root open={isOpen} onOpenChange={handleOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className={styles.overlay} />
        <Dialog.Content
          className={styles.content}
          aria-describedby="onboarding-step-description"
        >
          <Dialog.Title className={styles.headline}>
            {step === 1 && "One place for all your AI coding sessions"}
            {step === 2 && "Each session is isolated"}
            {step === 3 && "Create or navigate in one keystroke"}
            {step === 4 && "Key shortcuts"}
          </Dialog.Title>

          <StepIndicator current={step} total={4} />

          <button
            className={styles.skipButton}
            onClick={handleSkip}
            aria-label="Skip onboarding"
          >
            Skip
          </button>

          <div id="onboarding-step-description">
            {step === 1 && (
              <>
                <p className={styles.body}>
                  stapler-squad runs each AI agent in an isolated tmux session so your agents
                  never step on each other.
                </p>
                <pre className={styles.asciiDiagram}>{ASCII_DIAGRAM}</pre>
              </>
            )}

            {step === 2 && (
              <p className={styles.body}>
                Every session gets its own git worktree and directory. Agents write code in
                parallel without conflicts. Switch between sessions instantly — each one
                resumes exactly where it left off.
              </p>
            )}

            {step === 3 && (
              <>
                <p className={styles.body}>
                  Press <kbd className={styles.kbd}>⌘K</kbd> (or{" "}
                  <kbd className={styles.kbd}>Ctrl+K</kbd>) to open the omnibar. Type a
                  path, GitHub URL, or session name.
                </p>
              </>
            )}

            {step === 4 && (
              <>
                <div className={styles.shortcutTable}>
                  {SHORTCUTS.map(({ key, label }) => (
                    <div key={label} className={styles.shortcutRow}>
                      <span className={styles.shortcutLabel}>{label}</span>
                      <kbd className={styles.kbd}>{key}</kbd>
                    </div>
                  ))}
                </div>
              </>
            )}
          </div>

          <div className={styles.footer}>
            <div>
              {step > 1 && (
                <button className={styles.secondaryButton} onClick={handleBack}>
                  Back
                </button>
              )}
            </div>

            <div className={styles.footerRight}>
              {step === 3 && (
                <button className={styles.secondaryButton} onClick={handleTryOmnibar}>
                  Try it now
                </button>
              )}

              {step === 4 && (
                <button className={styles.linkButton} onClick={handleViewShortcuts}>
                  View all shortcuts
                </button>
              )}

              {step < 4 ? (
                <button className={styles.primaryButton} onClick={handleNext}>
                  Next
                </button>
              ) : (
                <>
                  <label className={styles.checkboxRow}>
                    <input
                      type="checkbox"
                      checked={dontShowAgain}
                      onChange={(e) => setDontShowAgain(e.target.checked)}
                    />
                    <span className={styles.checkboxLabel}>Don&apos;t show this again</span>
                  </label>
                  <button className={styles.primaryButton} onClick={handleGetStarted}>
                    Get started
                  </button>
                </>
              )}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
