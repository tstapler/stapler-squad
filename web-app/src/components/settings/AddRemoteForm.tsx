"use client";

import { useCallback, useRef, useState } from "react";
import { ConnectError } from "@connectrpc/connect";
import {
  useRemotesService,
  type RemoteConfigInfo,
  type GeneratedIdentityInfo,
} from "@/lib/hooks/useRemotesService";
import { HostKeyTrustDialog } from "./HostKeyTrustDialog";
import * as styles from "./AddRemoteForm.css";

/** Extracts a human-readable message from a failed RPC call. */
function errorMessage(err: unknown): string {
  if (err instanceof ConnectError) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

export interface AddRemoteFormProps {
  onSaved: (remote: RemoteConfigInfo) => void;
  onCancel: () => void;
}

/**
 * Add Remote form (Task 6.1.1b/e/f): name/host/user/port/base-path fields,
 * wired to TestRemoteConnection (draft mode) -> on unknown host key,
 * HostKeyTrustDialog -> TrustRemoteHostKey (draft mode) -> CreateRemote,
 * per ux.md Surface 2/3's interaction flow. The remote is never persisted
 * (CreateRemote is never called) until either an immediately-successful
 * test or an explicit "Trust and connect" click.
 */
export function AddRemoteForm({ onSaved, onCancel }: AddRemoteFormProps) {
  const {
    createRemote,
    deleteRemote,
    generateRemoteIdentity,
    testRemoteConnectionDraft,
    trustRemoteHostKeyDraft,
  } = useRemotesService();

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [user, setUser] = useState("");
  const [port, setPort] = useState("");
  const [basePath, setBasePath] = useState("");

  const [identity, setIdentity] = useState<GeneratedIdentityInfo | null>(null);
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  // Set only while a host_key_unknown response is pending user action --
  // the dialog is rendered whenever this is non-empty.
  const [pendingFingerprint, setPendingFingerprint] = useState<string | null>(null);
  const [trustBusy, setTrustBusy] = useState(false);

  // Guards against handleTrust acting on a stale result after the user has
  // already clicked Cancel while trustRemoteHostKeyDraft was still in
  // flight -- without this, the dialog visibly closes on Cancel but the
  // remote is still persisted (and onSaved still fires) once the pending
  // RPC resolves behind the user's back. A ref rather than a `cancelled`
  // local (useConfiguredRemotes.ts's equivalent idiom) because the
  // cancellation and the in-flight await live in two different callbacks
  // (handleTrustCancel vs. handleTrust), not one effect and its own cleanup.
  const trustCancelledRef = useRef(false);

  const canSubmit = Boolean(name.trim()) && Boolean(host.trim()) && Boolean(user.trim()) && Boolean(basePath.trim()) && !testing;

  // Once an identity has been generated, EVERY field locks (ux.md Surface 2:
  // "fields as above, now read-only until Cancel/retry") -- not just while
  // `testing` is true. Identity is generated once, keyed by `name`
  // (GenerateOrDescribeIdentity), and the displayed authorized_keys line is
  // that exact key. If fields (especially Name) stayed editable after
  // generation, a rename followed by Create would call CreateRemote with a
  // NEW name that has no stored identity -- its GenerateOrDescribeIdentity
  // fallback would then silently mint a FRESH, different keypair than the
  // one shown/pasted, and the saved remote's stored identity would never
  // match what's actually in the remote's authorized_keys. Fields stay
  // locked until the user explicitly cancels the whole form (which deletes
  // the orphaned identity) -- there is no "unlock to edit, then retry" path.
  const fieldsLocked = testing || Boolean(identity);

  const portNumber = port.trim() === "" ? 0 : Number(port);

  const currentDraft = useCallback(
    () => ({ name: name.trim(), host: host.trim(), user: user.trim(), port: portNumber }),
    [name, host, user, portNumber]
  );

  const persistRemote = useCallback(async () => {
    const saved = await createRemote({
      name: name.trim(),
      host: host.trim(),
      user: user.trim(),
      port: portNumber,
      basePath: basePath.trim(),
    });
    onSaved(saved);
  }, [createRemote, name, host, user, portNumber, basePath, onSaved]);

  const handleTestConnection = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!canSubmit) return;
      setError(null);
      setTesting(true);
      try {
        let currentIdentity = identity;
        if (!currentIdentity) {
          currentIdentity = await generateRemoteIdentity(name.trim());
          setIdentity(currentIdentity);
        }

        const result = await testRemoteConnectionDraft(currentDraft());
        if (result.hostKeyUnknown) {
          setPendingFingerprint(result.fingerprint);
        } else if (result.success) {
          await persistRemote();
        } else {
          setError(result.errorMessage || `Couldn't reach ${name.trim()}. Check that the host is up, or pick a different remote.`);
        }
      } catch (err) {
        setError(errorMessage(err));
      } finally {
        setTesting(false);
      }
    },
    [canSubmit, identity, name, generateRemoteIdentity, testRemoteConnectionDraft, currentDraft, persistRemote]
  );

  const handleTrust = useCallback(async () => {
    if (!pendingFingerprint) return;
    trustCancelledRef.current = false;
    setTrustBusy(true);
    setError(null);
    try {
      const trustResult = await trustRemoteHostKeyDraft(currentDraft(), pendingFingerprint);
      // Bail out if the user clicked Cancel while this RPC was still in
      // flight -- otherwise a resolve-after-cancel would silently persist
      // the remote (and fire onSaved) even though the dialog already closed.
      // See trustCancelledRef's declaration above.
      if (trustCancelledRef.current) return;
      if (trustResult.success) {
        setPendingFingerprint(null);
        await persistRemote();
      } else {
        setPendingFingerprint(null);
        setError(trustResult.errorMessage || "Trusting the host key failed.");
      }
    } catch (err) {
      if (trustCancelledRef.current) return;
      setPendingFingerprint(null);
      setError(errorMessage(err));
    } finally {
      setTrustBusy(false);
    }
  }, [pendingFingerprint, trustRemoteHostKeyDraft, currentDraft, persistRemote]);

  const handleTrustCancel = useCallback(() => {
    // Signal any in-flight handleTrust call to bail out once its await
    // resolves, instead of silently persisting the remote after the user
    // already closed the dialog -- see handleTrust's trustCancelledRef check.
    trustCancelledRef.current = true;
    // Remote was never saved (CreateRemote is only called after a
    // successful Trust) -- closing the dialog just returns to the still-
    // editable Add Remote form. Per ux.md Surface 3 step 4, this is not a
    // dead end: the user can fix things on the remote host and retry Test
    // connection, which reuses the SAME generated identity (see
    // GenerateRemoteIdentity's idempotency).
    setPendingFingerprint(null);
  }, []);

  const handleFormCancel = useCallback(() => {
    // If an identity was generated but the remote was never saved, clean up
    // the orphaned keychain entry (ux.md Surface 2, interaction flow step 3)
    // -- best-effort, a failure here shouldn't block closing the form.
    if (identity && name.trim()) {
      void deleteRemote(name.trim()).catch(() => {});
    }
    onCancel();
  }, [identity, name, deleteRemote, onCancel]);

  const handleCopyAuthorizedKeys = useCallback(() => {
    if (!identity) return;
    void navigator.clipboard.writeText(identity.authorizedKeysLine).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    });
  }, [identity]);

  return (
    <>
      <form className={styles.form} onSubmit={handleTestConnection} data-testid="add-remote-form">
        {error && (
          <div className={styles.errorMessage} role="alert" data-testid="add-remote-error">
            {error}
          </div>
        )}

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="add-remote-name">
            Name<span aria-hidden="true">*</span>
          </label>
          <input
            id="add-remote-name"
            type="text"
            className={styles.input}
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={fieldsLocked}
            required
            aria-required="true"
            placeholder="prod-box"
            data-testid="add-remote-name"
          />
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="add-remote-host">
            Host<span aria-hidden="true">*</span>
          </label>
          <input
            id="add-remote-host"
            type="text"
            className={styles.input}
            value={host}
            onChange={(e) => setHost(e.target.value)}
            disabled={fieldsLocked}
            required
            aria-required="true"
            placeholder="prod.example.com"
            data-testid="add-remote-host"
          />
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="add-remote-user">
            User<span aria-hidden="true">*</span>
          </label>
          <input
            id="add-remote-user"
            type="text"
            className={styles.input}
            value={user}
            onChange={(e) => setUser(e.target.value)}
            disabled={fieldsLocked}
            required
            aria-required="true"
            placeholder="tyler"
            data-testid="add-remote-user"
          />
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="add-remote-port">
            Port
          </label>
          <input
            id="add-remote-port"
            type="number"
            inputMode="numeric"
            className={styles.input}
            value={port}
            onChange={(e) => setPort(e.target.value)}
            disabled={fieldsLocked}
            placeholder="22"
            data-testid="add-remote-port"
          />
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="add-remote-base-path">
            Base path<span aria-hidden="true">*</span>
          </label>
          <input
            id="add-remote-base-path"
            type="text"
            className={styles.input}
            value={basePath}
            onChange={(e) => setBasePath(e.target.value)}
            disabled={fieldsLocked}
            required
            aria-required="true"
            placeholder="/srv/workspaces"
            data-testid="add-remote-base-path"
          />
        </div>

        {!identity && (
          <p className={styles.hint}>
            A new SSH keypair will be generated for this remote and stored in your OS keychain. You&apos;ll need to
            add the public key below to the remote&apos;s authorized_keys.
          </p>
        )}

        {identity && (
          <div className={styles.authorizedKeysBlock} data-testid="add-remote-authorized-keys">
            <span className={styles.label}>Add this to ~/.ssh/authorized_keys on {host.trim() || "the remote"}:</span>
            <div className={styles.authorizedKeysRow}>
              <code className={styles.authorizedKeysLine}>{identity.authorizedKeysLine}</code>
              <button
                type="button"
                className={styles.copyBtn}
                onClick={handleCopyAuthorizedKeys}
                data-testid="add-remote-copy-authorized-keys"
              >
                {copied ? "Copied!" : "Copy"}
              </button>
            </div>
            <p className={styles.caveat}>
              Stapler Squad cannot verify this line was applied — test the connection once it&apos;s in place.
            </p>
          </div>
        )}

        <div className={styles.actionRow}>
          <button type="button" className={styles.cancelBtn} onClick={handleFormCancel} data-testid="add-remote-cancel">
            Cancel
          </button>
          <button type="submit" className={styles.submitBtn} disabled={!canSubmit} data-testid="add-remote-submit">
            {testing ? "Testing…" : "Test connection"}
          </button>
        </div>
      </form>

      {pendingFingerprint && (
        <HostKeyTrustDialog
          host={host.trim()}
          port={portNumber}
          fingerprint={pendingFingerprint}
          onTrust={() => {
            if (!trustBusy) void handleTrust();
          }}
          onCancel={handleTrustCancel}
        />
      )}
    </>
  );
}
