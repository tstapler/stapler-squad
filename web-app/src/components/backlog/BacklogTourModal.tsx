// +feature: backlog:tour
"use client";

import { useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import * as styles from "@/components/ui/ModalTour.css";
import * as tourStyles from "./BacklogTourModal.css";
import { LifecycleDiagram } from "./BacklogEmptyState";

interface BacklogTourModalProps {
  isOpen: boolean;
  /**
   * Called whenever the modal closes, with whether the dismissal should be
   * persisted (so the tour won't auto-show again). `persist` reflects the
   * "Don't show this again" checkbox on the last step — Skip and the
   * backdrop/Escape path always persist, matching the app-wide onboarding
   * modal's behavior.
   */
  onComplete: (persist: boolean) => void;
}

type Step = 1 | 2 | 3 | 4;

const LAST_STEP: Step = 4;

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

/**
 * First-visit walkthrough for the backlog page. Mirrors the app-wide
 * OnboardingModal (same shared modal chrome/step-indicator styles from
 * components/ui/ModalTour.css, not OnboardingModal's own CSS module), scoped
 * to explain the backlog lifecycle and the Repository Path field specifically
 * — the field that has confused first-time users into pasting a GitHub URL
 * where a local clone path was expected.
 */
export function BacklogTourModal({ isOpen, onComplete }: BacklogTourModalProps) {
  const [step, setStep] = useState<Step>(1);
  const [dontShowAgain, setDontShowAgain] = useState(true);

  const handleSkip = () => {
    onComplete(true);
  };

  const handleNext = () => {
    if (step < LAST_STEP) setStep((prev) => (prev + 1) as Step);
  };

  const handleBack = () => {
    if (step > 1) setStep((prev) => (prev - 1) as Step);
  };

  const handleDone = () => {
    onComplete(dontShowAgain);
  };

  const handleOpenChange = (open: boolean) => {
    if (open) {
      setStep(1);
      setDontShowAgain(true);
    } else {
      onComplete(dontShowAgain);
    }
  };

  return (
    <Dialog.Root open={isOpen} onOpenChange={handleOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className={styles.overlay} />
        <Dialog.Content
          className={styles.content}
          aria-describedby="backlog-tour-step-description"
          data-testid="backlog-tour-modal"
        >
          <Dialog.Title className={styles.headline}>
            {step === 1 && "How backlog items work"}
            {step === 2 && "Filling out the form"}
            {step === 3 && "What happens after you hit Create"}
            {step === 4 && "Skip planning / Skip review gate"}
          </Dialog.Title>

          <StepIndicator current={step} total={LAST_STEP} />

          <button className={styles.skipButton} onClick={handleSkip} aria-label="Skip tour">
            Skip
          </button>

          <div id="backlog-tour-step-description">
            {step === 1 && (
              <>
                <p className={styles.body}>
                  Every backlog item moves through the same lifecycle, from a rough idea
                  to a finished, reviewed change.
                </p>
                <LifecycleDiagram />
              </>
            )}

            {step === 2 && (
              <>
                <p className={styles.body}>
                  <strong>Repository Path</strong> is the one field people get tripped up
                  on — it needs a local clone (like{" "}
                  <code className={styles.kbd}>~/code/my-repo</code>), not a link to a
                  page on GitHub.
                </p>
                <div className={tourStyles.calloutBox} data-testid="backlog-tour-repo-path-callout">
                  Don&apos;t have a local clone? Paste a GitHub URL instead (e.g.{" "}
                  <code className={styles.kbd}>https://github.com/owner/repo</code>) and
                  we&apos;ll clone it for you automatically when you save.
                </div>
              </>
            )}

            {step === 3 && (
              <p className={styles.body}>
                If you set a repository path, we automatically kick off triage — an agent
                reviews the item and suggests acceptance criteria. No repo yet? That&apos;s
                fine — the item just sits as an idea until you add one later.
              </p>
            )}

            {step === 4 && (
              <>
                <p className={styles.body}>
                  Two checkboxes on the create form skip parts of that automated
                  workflow:
                </p>
                <ul className={tourStyles.flagList}>
                  <li>
                    <strong>Skip planning phase</strong> — go straight to triage without a
                    separate planning pass.
                  </li>
                  <li>
                    <strong>Skip review gate</strong> — mark work done without an
                    automated review pass first.
                  </li>
                </ul>
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
              {step < LAST_STEP ? (
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
                  <button className={styles.primaryButton} onClick={handleDone}>
                    Got it
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
