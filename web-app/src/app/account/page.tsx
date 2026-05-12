"use client";

import { useState, useEffect, useCallback } from "react";
import { usePageView } from "@/lib/analytics/usePageView";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/contexts/AuthContext";
import { routes } from "@/lib/routes";
import {
  generateInvite,
  listCredentials,
  revokeCredential,
  type InviteResponse,
  type CredentialInfo,
} from "@/lib/auth/passkey";
import * as s from "./account.css";

// ─── Countdown ────────────────────────────────────────────────────────────────

function Countdown({ ttlSeconds }: { ttlSeconds: number }) {
  const [left, setLeft] = useState(ttlSeconds);

  useEffect(() => {
    if (left <= 0) return;
    const id = setInterval(() => {
      setLeft((n) => {
        if (n <= 1) {
          clearInterval(id);
          return 0;
        }
        return n - 1;
      });
    }, 1000);
    return () => clearInterval(id);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (left <= 0) {
    return <p className={`${s.countdown} ${s.countdownExpired}`}>Invite expired — generate a new one</p>;
  }

  const m = Math.floor(left / 60);
  const sec = left % 60;
  return (
    <p className={s.countdown}>
      Expires in {m}:{String(sec).padStart(2, "0")}
    </p>
  );
}

// ─── Add Device Modal ─────────────────────────────────────────────────────────

function AddDeviceModal({ onClose, onSuccess }: { onClose: () => void; onSuccess: () => void }) {
  const [label, setLabel] = useState("");
  const [invite, setInvite] = useState<InviteResponse | null>(null);
  const [showUrl, setShowUrl] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);

  const generate = async () => {
    setLoading(true);
    setError("");
    try {
      const data = await generateInvite(label || "New Device");
      setInvite(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const copyUrl = async () => {
    if (!invite) return;
    await navigator.clipboard.writeText(invite.registration_url);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (!invite) {
    return (
      <div className={s.modalOverlay} onClick={(e) => e.target === e.currentTarget && onClose()}>
        <div className={s.modal}>
          <h2 className={s.modalTitle}>Add New Device</h2>
          <p className={s.stepDesc}>
            Give this device a name so you can identify it later (e.g. &ldquo;iPhone 15&rdquo;).
          </p>
          <input
            type="text"
            placeholder="Device name (optional)"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && generate()}
            style={{
              width: "100%",
              padding: "0.5rem 0.75rem",
              borderRadius: "6px",
              border: "1px solid var(--border-color)",
              background: "var(--input-background)",
              color: "var(--input-text)",
              fontSize: "0.875rem",
              boxSizing: "border-box",
            }}
          />
          {error && <p className={s.errorText}>{error}</p>}
          <div className={s.modalActions}>
            <button className={s.ghostButton} onClick={onClose}>Cancel</button>
            <button className={s.primaryButton} onClick={generate} disabled={loading}>
              {loading ? "Generating…" : "Generate Invite"}
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={s.modalOverlay} onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className={s.modal}>
        <h2 className={s.modalTitle}>Add New Device</h2>

        <ol className={s.stepList}>
          {/* Step 1: Install CA cert */}
          <li className={s.step}>
            <span className={s.stepNumber}>1</span>
            <div className={s.stepContent}>
              <p className={s.stepTitle}>Install the CA certificate on the new device</p>
              <p className={s.stepDesc}>
                Scan the left QR code to download the certificate, then install it:
              </p>
              <p className={s.stepDesc}>
                <strong>iOS</strong>: Download → Settings → General → VPN &amp; Device Management → [cert] → Install.
                Then Settings → General → About → Certificate Trust Settings → enable.
              </p>
              <p className={s.stepDesc}>
                <strong>Android</strong>: Download → Settings → Security → Encryption &amp; credentials → Install a certificate → CA Certificate.
              </p>
              <p className={s.stepDesc}>
                <strong>macOS / Windows</strong>: Download and double-click the .pem file to add it to the trust store.
              </p>
              <div className={s.qrRow}>
                <div className={s.qrBlock}>
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={invite.ca_qr_data_url} alt="CA cert QR code" className={s.qrImage} />
                  <span className={s.qrLabel}>CA Certificate</span>
                </div>
              </div>
            </div>
          </li>

          {/* Step 2: Register passkey */}
          <li className={s.step}>
            <span className={s.stepNumber}>2</span>
            <div className={s.stepContent}>
              <p className={s.stepTitle}>Register a passkey on the new device</p>
              <p className={s.stepDesc}>
                Scan the QR code with the new device&rsquo;s camera, then follow the browser prompt.
              </p>
              <div className={s.qrRow}>
                <div className={s.qrBlock}>
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={invite.reg_qr_data_url} alt="Registration QR code" className={s.qrImage} />
                  <span className={s.qrLabel}>Registration</span>
                </div>
              </div>
              <Countdown ttlSeconds={invite.ttl_seconds} />
              {showUrl ? (
                <div className={s.urlRow}>
                  <span className={s.urlText} title={invite.registration_url}>
                    {invite.registration_url}
                  </span>
                  <button className={s.ghostButton} onClick={copyUrl}>
                    {copied ? "Copied!" : "Copy"}
                  </button>
                </div>
              ) : (
                <button
                  className={s.ghostButton}
                  onClick={() => setShowUrl(true)}
                  style={{ alignSelf: "flex-start" }}
                >
                  Can&rsquo;t scan? Show URL
                </button>
              )}
            </div>
          </li>
        </ol>

        <div className={s.modalActions}>
          <button className={s.ghostButton} onClick={() => { setInvite(null); setLabel(""); }}>
            Regenerate
          </button>
          <button className={s.primaryButton} onClick={() => { onSuccess(); onClose(); }}>
            Done
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── Revoke Confirm Dialog ────────────────────────────────────────────────────

function RevokeConfirmDialog({
  credential,
  isLast,
  onConfirm,
  onCancel,
}: {
  credential: CredentialInfo;
  isLast: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <div className={s.modalOverlay} onClick={(e) => e.target === e.currentTarget && onCancel()}>
      <div className={s.modal}>
        <h2 className={s.modalTitle}>Revoke passkey?</h2>
        <p className={s.stepDesc}>
          Remove <strong>{credential.display_name || "this passkey"}</strong>? The device will no longer be able to sign in.
        </p>
        {isLast && (
          <p className={s.warningText}>
            This is your last passkey. Revoking it will sign out all active sessions and lock you out of remote access until you use the CLI to register a new passkey.
          </p>
        )}
        <div className={s.modalActions}>
          <button className={s.ghostButton} onClick={onCancel}>Cancel</button>
          <button className={s.dangerButton} onClick={onConfirm}>
            {isLast ? "Revoke and lock out" : "Revoke"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── Credential Row ───────────────────────────────────────────────────────────

function CredentialRow({
  cred,
  isLast,
  onRevoke,
}: {
  cred: CredentialInfo;
  isLast: boolean;
  onRevoke: (cred: CredentialInfo) => void;
}) {
  const createdAt = cred.created_at
    ? new Date(cred.created_at).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })
    : "Unknown";
  const lastUsed = cred.last_used_at
    ? new Date(cred.last_used_at).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })
    : "Never";

  return (
    <div className={s.credentialRow}>
      <div className={s.credentialInfo}>
        <span className={s.credentialName}>{cred.display_name || "Passkey"}</span>
        <span className={s.credentialMeta}>
          Registered {createdAt} · Last used {lastUsed}
        </span>
      </div>
      <button className={s.dangerButton} onClick={() => onRevoke(cred)} title="Revoke this passkey">
        Revoke
      </button>
    </div>
  );
}

// ─── Account Page ─────────────────────────────────────────────────────────────

export default function AccountPage() {
  usePageView();
  const router = useRouter();
  const { authEnabled, authenticated, loading: authLoading } = useAuth();

  const [credentials, setCredentials] = useState<CredentialInfo[]>([]);
  const [credsLoading, setCredsLoading] = useState(true);
  const [credsError, setCredsError] = useState("");
  const [showAddModal, setShowAddModal] = useState(false);
  const [revoking, setRevoking] = useState<CredentialInfo | null>(null);

  useEffect(() => {
    if (!authLoading && authEnabled && !authenticated) {
      router.replace(routes.login);
    }
  }, [authLoading, authEnabled, authenticated, router]);

  const loadCredentials = useCallback(async () => {
    setCredsLoading(true);
    setCredsError("");
    try {
      const data = await listCredentials();
      setCredentials(data);
    } catch (e) {
      setCredsError(e instanceof Error ? e.message : String(e));
    } finally {
      setCredsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!authLoading && authEnabled && authenticated) {
      loadCredentials();
    }
  }, [authLoading, authEnabled, authenticated, loadCredentials]);

  const handleRevoke = async (cred: CredentialInfo) => {
    try {
      const result = await revokeCredential(cred.id);
      setRevoking(null);
      if (result.last_credential) {
        // Session cookie was invalidated server-side; redirect to login.
        router.replace(routes.login);
        return;
      }
      await loadCredentials();
    } catch (e) {
      setCredsError(e instanceof Error ? e.message : String(e));
      setRevoking(null);
    }
  };

  if (authLoading) {
    return (
      <main id="main-content" className={s.page}>
        <p className={s.emptyState}>Loading…</p>
      </main>
    );
  }

  return (
    <main id="main-content" className={s.page}>
      <section className={s.section}>
        <h2 className={s.sectionTitle}>Your Passkeys</h2>

        {credsLoading ? (
          <p className={s.emptyState}>Loading passkeys…</p>
        ) : credsError ? (
          <p className={s.errorText}>{credsError}</p>
        ) : credentials.length === 0 ? (
          <p className={s.emptyState}>No passkeys registered.</p>
        ) : (
          <div className={s.credentialList}>
            {credentials.map((cred) => (
              <CredentialRow
                key={cred.id}
                cred={cred}
                isLast={credentials.length === 1}
                onRevoke={(c) => setRevoking(c)}
              />
            ))}
          </div>
        )}

        <div>
          <button className={s.primaryButton} onClick={() => setShowAddModal(true)}>
            Add New Device
          </button>
        </div>
      </section>

      {showAddModal && (
        <AddDeviceModal
          onClose={() => setShowAddModal(false)}
          onSuccess={loadCredentials}
        />
      )}

      {revoking && (
        <RevokeConfirmDialog
          credential={revoking}
          isLast={credentials.length === 1}
          onConfirm={() => handleRevoke(revoking)}
          onCancel={() => setRevoking(null)}
        />
      )}
    </main>
  );
}
