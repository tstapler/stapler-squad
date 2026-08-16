"use client";
// +feature: slack-notification-settings

import { useState, useEffect, useRef, useCallback } from "react";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { SessionService } from "@/gen/session/v1/session_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl } from "@/lib/config";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { InlineNotice } from "@/components/common/InlineNotice";
import {
  container,
  heading,
  description,
  loadingText,
  form,
  field,
  label as labelClass,
  inputRow,
  input,
  inputInvalid,
  hint,
  errorText,
  removeBtn,
  toggleRow,
  toggleLabel,
  betaNote,
  testResultAlert,
  testResultStatus,
  deliveryStatus,
  deliveryStatusFailed,
  actions,
  saveError as saveErrorClass,
} from "./SlackNotificationSettings.css";

// WEBHOOK_URL_PATTERN mirrors the server-side backstop in
// server/services/slack_config_service.go's slackWebhookURLPrefix check.
const WEBHOOK_URL_PATTERN = /^https:\/\/hooks\.slack\.com\/services\//;
const WEBHOOK_URL_ERROR =
  "This doesn't look like a Slack Incoming Webhook URL (expected https://hooks.slack.com/services/...)";

interface LastDelivery {
  attempted: boolean;
  success: boolean;
  error: string;
  attemptedAt?: Date;
}

export function SlackNotificationSettings() {
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [webhookConfigured, setWebhookConfigured] = useState(false);
  const [webhookInput, setWebhookInput] = useState("");
  const [webhookError, setWebhookError] = useState<string | null>(null);

  const [notifyOnQueueItem, setNotifyOnQueueItem] = useState(false);
  const [queueDepthThreshold, setQueueDepthThreshold] = useState(0);
  const [dashboardBaseUrl, setDashboardBaseUrl] = useState("");
  const [dashboardWarningDismissed, setDashboardWarningDismissed] =
    useState(false);

  const [lastDelivery, setLastDelivery] = useState<LastDelivery | null>(null);

  const [testSubmitting, setTestSubmitting] = useState(false);
  const [testResult, setTestResult] = useState<{
    success: boolean;
    message: string;
  } | null>(null);

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<
    typeof createClient<typeof SessionService>
  > | null>(null);

  const loadConfig = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      setLoading(true);
      setLoadError(null);
      const response = await clientRef.current.getSlackConfig({});
      const cfg = response.config;
      if (cfg) {
        setWebhookConfigured(cfg.webhookConfigured);
        setNotifyOnQueueItem(cfg.notifyOnQueueItem);
        setQueueDepthThreshold(cfg.queueDepthThreshold);
        setDashboardBaseUrl(cfg.dashboardBaseUrl);
        if (cfg.lastDelivery) {
          setLastDelivery({
            attempted: cfg.lastDelivery.attempted,
            success: cfg.lastDelivery.success,
            error: cfg.lastDelivery.error,
            attemptedAt: cfg.lastDelivery.attemptedAt
              ? timestampDate(cfg.lastDelivery.attemptedAt)
              : undefined,
          });
        } else {
          setLastDelivery(null);
        }
      }
    } catch {
      setLoadError("Couldn't load Slack settings.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
    loadConfig();
  }, [loadConfig]);

  const trimmedWebhook = webhookInput.trim();
  const hasValidTypedWebhook = WEBHOOK_URL_PATTERN.test(trimmedWebhook);
  const togglesEnabled = webhookConfigured || hasValidTypedWebhook;
  const testDisabled = !togglesEnabled || testSubmitting;

  function handleWebhookChange(e: React.ChangeEvent<HTMLInputElement>) {
    const value = e.target.value;
    setWebhookInput(value);
    const trimmed = value.trim();
    // Clear the error live as soon as the shape becomes valid (or the field
    // is emptied) — no re-submit required to see it clear (design/ux.md's
    // error-handling table).
    if (webhookError && (trimmed === "" || WEBHOOK_URL_PATTERN.test(trimmed))) {
      setWebhookError(null);
    }
  }

  function handleWebhookBlur() {
    const trimmed = webhookInput.trim();
    if (trimmed === "") {
      setWebhookError(null);
      return;
    }
    setWebhookError(
      WEBHOOK_URL_PATTERN.test(trimmed) ? null : WEBHOOK_URL_ERROR,
    );
  }

  async function handleTestSend() {
    if (!clientRef.current) return;
    setTestSubmitting(true);
    setTestResult(null);
    try {
      const resp = await clientRef.current.testSlackWebhook({
        webhookUrl: trimmedWebhook,
      });
      if (resp.success) {
        setTestResult({
          success: true,
          message: "Test message sent — check your Slack channel.",
        });
      } else {
        setTestResult({
          success: false,
          message: `Test failed: ${resp.error}`,
        });
      }
    } catch (err) {
      setTestResult({
        success: false,
        message: `Test failed: ${err instanceof Error ? err.message : String(err)}`,
      });
    } finally {
      setTestSubmitting(false);
    }
  }

  async function handleRemoveWebhook() {
    if (!clientRef.current) return;
    setSaving(true);
    setSaveError(null);
    try {
      await clientRef.current.updateSlackConfig({
        clearWebhookUrl: true,
        notifyOnQueueItem: false,
        queueDepthThreshold,
        approvalEnabled: false,
        dashboardBaseUrl,
      });
      setWebhookInput("");
      setTestResult(null);
      await loadConfig();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  async function handleSave() {
    if (!clientRef.current || webhookError) return;
    setSaving(true);
    setSaveError(null);
    try {
      await clientRef.current.updateSlackConfig({
        webhookUrl: trimmedWebhook,
        notifyOnQueueItem,
        queueDepthThreshold,
        // Phase 2 ("Allow Approve/Deny from Slack") is not shipped yet — the
        // checkbox is always disabled, so this is always sent false.
        approvalEnabled: false,
        dashboardBaseUrl: dashboardBaseUrl.trim(),
      });
      setWebhookInput("");
      await loadConfig();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  const showDashboardWarning =
    notifyOnQueueItem &&
    dashboardBaseUrl.trim() === "" &&
    !dashboardWarningDismissed;

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>Slack Notifications</h2>
        <p className={loadingText}>Loading…</p>
      </div>
    );
  }

  if (loadError) {
    return (
      <div className={container}>
        <h2 className={heading}>Slack Notifications</h2>
        <div role="alert" className={testResultAlert}>
          {loadError}
        </div>
        <div className={actions}>
          <button
            type="button"
            className="btn btn-primary"
            onClick={loadConfig}
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={container}>
      <h2 className={heading}>Slack Notifications</h2>
      <p className={description}>
        Get pinged in Slack when an agent needs your review or approval.
      </p>

      <div className={form}>
        <div className={field}>
          <label htmlFor="slack-webhook-url" className={labelClass}>
            Webhook URL
          </label>
          <div className={inputRow}>
            <input
              id="slack-webhook-url"
              type="text"
              className={`${input} ${webhookError ? inputInvalid : ""}`}
              value={webhookInput}
              onChange={handleWebhookChange}
              onBlur={handleWebhookBlur}
              placeholder={
                webhookConfigured
                  ? "•••• (configured)"
                  : "https://hooks.slack.com/services/..."
              }
              aria-describedby={
                webhookError
                  ? "slack-webhook-hint slack-webhook-error"
                  : "slack-webhook-hint"
              }
              aria-invalid={webhookError ? "true" : "false"}
            />
            {webhookConfigured && (
              <button
                type="button"
                className={removeBtn}
                onClick={handleRemoveWebhook}
                disabled={saving}
              >
                Remove
              </button>
            )}
          </div>
          <p id="slack-webhook-hint" className={hint}>
            Paste your Slack Incoming Webhook URL
          </p>
          {webhookError && (
            <p
              id="slack-webhook-error"
              role="alert"
              data-testid="slack-webhook-error"
              className={errorText}
            >
              {webhookError}
            </p>
          )}
        </div>

        <div>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={handleTestSend}
            disabled={testDisabled}
          >
            {testSubmitting ? "Sending…" : "Send test message"}
          </button>
          {testResult && (
            <div
              role={testResult.success ? "status" : "alert"}
              data-testid="slack-test-webhook-result"
              className={
                testResult.success ? testResultStatus : testResultAlert
              }
            >
              {testResult.message}
            </div>
          )}
        </div>

        <div className={toggleRow}>
          <input
            id="slack-notify-queue-item"
            type="checkbox"
            checked={notifyOnQueueItem}
            onChange={(e) => setNotifyOnQueueItem(e.target.checked)}
            disabled={!togglesEnabled}
          />
          <label htmlFor="slack-notify-queue-item" className={toggleLabel}>
            Notify on new review-queue item
          </label>
        </div>

        <div className={field}>
          <label htmlFor="slack-queue-depth-threshold" className={labelClass}>
            Queue-depth digest threshold (0 = off)
          </label>
          <input
            id="slack-queue-depth-threshold"
            type="number"
            min={0}
            className={input}
            value={queueDepthThreshold}
            onChange={(e) =>
              setQueueDepthThreshold(
                Math.max(0, parseInt(e.target.value, 10) || 0),
              )
            }
            disabled={!togglesEnabled}
            aria-describedby="slack-queue-depth-threshold-hint"
          />
          <p id="slack-queue-depth-threshold-hint" className={hint}>
            You&apos;ll get one digest per burst; a persistently full queue
            won&apos;t nudge you again until it drops below the threshold and
            re-crosses.
          </p>
        </div>

        <div className={toggleRow}>
          <input
            id="slack-approval-enabled"
            type="checkbox"
            checked={false}
            onChange={() => {}}
            disabled
          />
          <label htmlFor="slack-approval-enabled" className={toggleLabel}>
            Allow Approve/Deny from Slack
            <span className={betaNote}>
              Beta — requires public reachability; see docs
            </span>
          </label>
        </div>

        <div className={field}>
          <label htmlFor="slack-dashboard-base-url" className={labelClass}>
            Dashboard URL
          </label>
          <input
            id="slack-dashboard-base-url"
            type="text"
            className={input}
            value={dashboardBaseUrl}
            onChange={(e) => setDashboardBaseUrl(e.target.value)}
            aria-describedby="slack-dashboard-base-url-hint"
          />
          <p id="slack-dashboard-base-url-hint" className={hint}>
            Used to build &quot;view in dashboard&quot; links in Slack messages.
            Leave blank and links will only work on your home network.
          </p>
        </div>

        {showDashboardWarning && (
          <InlineNotice
            message="Your Slack links may not work outside your home network — set a Dashboard URL below."
            onDismiss={() => setDashboardWarningDismissed(true)}
            data-testid="slack-dashboard-warning"
          />
        )}

        {saveError && (
          <p role="alert" className={saveErrorClass}>
            {saveError}
          </p>
        )}

        <div className={actions}>
          <button
            type="button"
            className="btn btn-primary"
            onClick={handleSave}
            disabled={saving || Boolean(webhookError)}
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>

        <div
          role="status"
          data-testid="slack-last-delivery-status"
          className={`${deliveryStatus} ${lastDelivery && lastDelivery.attempted && !lastDelivery.success ? deliveryStatusFailed : ""}`}
        >
          {lastDelivery && lastDelivery.attempted ? (
            <>
              Last Slack delivery:{" "}
              {lastDelivery.attemptedAt
                ? formatRelativeTime(lastDelivery.attemptedAt.getTime())
                : "unknown"}{" "}
              —{" "}
              {lastDelivery.success
                ? "✓ delivered"
                : `✗ failed: ${lastDelivery.error}`}
            </>
          ) : (
            "Last Slack delivery: none yet"
          )}
        </div>
      </div>
    </div>
  );
}
