// +feature: onboarding-hook-install
"use client";

import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
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

type Step = 1 | 2 | 3 | 4 | 5;

const LAST_STEP: Step = 5;

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

  // Hook installation step state.
  const [installRules, setInstallRules] = useState(true);
  const [installNotifications, setInstallNotifications] = useState(true);
  const [rulesAvailable, setRulesAvailable] = useState(true);
  const [notificationsAvailable, setNotificationsAvailable] = useState(true);
  const [rulesInstalled, setRulesInstalled] = useState(false);
  const [notificationsInstalled, setNotificationsInstalled] = useState(false);
  const [hookBusy, setHookBusy] = useState(false);
  const [hookMessage, setHookMessage] = useState<string | null>(null);

  const client = useMemo(
    () => createClient(SessionService, createConnectTransport({ baseUrl: getApiBaseUrl() })),
    []
  );

  // Guards: avoid setState after unmount, and only seed the toggle defaults once
  // so navigating Back→forward to the hooks step doesn't clobber the user's edits.
  const mountedRef = useRef(true);
  const seededRef = useRef(false);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const refreshHookStatus = useCallback(async () => {
    try {
      const res = await client.getHookStatus({});
      if (!mountedRef.current) return;
      setRulesInstalled(res.rulesInstalled);
      setNotificationsInstalled(res.notificationsInstalled);
      setRulesAvailable(res.rulesAvailable);
      setNotificationsAvailable(res.notificationsAvailable);
      // Pre-check a toggle only when the hook is available and not already installed.
      // Seed once so re-entering the step preserves any manual toggle changes.
      if (!seededRef.current) {
        seededRef.current = true;
        setInstallRules(res.rulesAvailable && !res.rulesInstalled);
        setInstallNotifications(res.notificationsAvailable && !res.notificationsInstalled);
      }
    } catch {
      if (mountedRef.current) setHookMessage("Could not check hook status.");
    }
  }, [client]);

  // Load current hook status when the user reaches the hooks step.
  useEffect(() => {
    if (isOpen && step === 5) {
      void refreshHookStatus();
    }
  }, [isOpen, step, refreshHookStatus]);

  const handleInstallHooks = async () => {
    setHookBusy(true);
    setHookMessage(null);
    try {
      const res = await client.installHooks({ installRules, installNotifications });
      if (!mountedRef.current) return;
      if (res.status) {
        setRulesInstalled(res.status.rulesInstalled);
        setNotificationsInstalled(res.status.notificationsInstalled);
      }
      setHookMessage(res.messages.join(" ") || "Hooks updated.");
    } catch {
      if (mountedRef.current) {
        setHookMessage("Failed to install hooks. You can set them up later from the docs.");
      }
    } finally {
      if (mountedRef.current) setHookBusy(false);
    }
  };

  const handleSkip = () => {
    try {
      localStorage.setItem("stapler-squad:onboarded", "true");
    } catch {
      // ignore storage errors
    }
    onClose();
  };

  const handleNext = () => {
    if (step < LAST_STEP) {
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
      // Re-seed toggle defaults from fresh status on a new onboarding run.
      seededRef.current = false;
      setHookMessage(null);
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
            {step === 5 && "Enable Claude Code hooks"}
          </Dialog.Title>

          <StepIndicator current={step} total={5} />

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

            {step === 5 && (
              <>
                <p className={styles.body}>
                  Optionally hook stapler-squad into Claude Code so it works outside the
                  app too. You can change this later.
                </p>

                <label className={styles.checkboxRow}>
                  <input
                    type="checkbox"
                    checked={installRules}
                    disabled={!rulesAvailable || rulesInstalled || hookBusy}
                    onChange={(e) => setInstallRules(e.target.checked)}
                  />
                  <span className={styles.checkboxLabel}>
                    Enable rule enforcement
                    {rulesInstalled
                      ? " (already installed)"
                      : !rulesAvailable
                        ? " (ssq-hooks not installed — run `make install`)"
                        : " — gate tool calls through your rules"}
                  </span>
                </label>

                <label className={styles.checkboxRow}>
                  <input
                    type="checkbox"
                    checked={installNotifications}
                    disabled={!notificationsAvailable || notificationsInstalled || hookBusy}
                    onChange={(e) => setInstallNotifications(e.target.checked)}
                  />
                  <span className={styles.checkboxLabel}>
                    Enable notifications
                    {notificationsInstalled
                      ? " (already installed)"
                      : !notificationsAvailable
                        ? " (ssq-hook-handler not found)"
                        : " — chimes and alerts when Claude needs you"}
                  </span>
                </label>

                {hookMessage && <p className={styles.body}>{hookMessage}</p>}
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

              {step < LAST_STEP ? (
                <button className={styles.primaryButton} onClick={handleNext}>
                  Next
                </button>
              ) : (
                <>
                  {(installRules || installNotifications) && (
                    <button
                      className={styles.secondaryButton}
                      onClick={handleInstallHooks}
                      disabled={hookBusy}
                    >
                      {hookBusy ? "Installing…" : "Install"}
                    </button>
                  )}
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
