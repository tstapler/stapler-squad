"use client";
// +feature: ui:owner-inbox-page

import { useState } from "react";
import { Link } from "@/lib/nav";
import { useAnswerGuidanceRequestMutation, useListAllPendingGuidanceRequestsQuery } from "@/lib/api/guidanceApi";
import { isOwnerActionRequest, ownerActionQuestion } from "@/lib/guidance/ownerAction";
import { getErrorMessage } from "@/lib/utils/connectError";
import * as styles from "./page.css";

/** Global list of request_owner_action requests still waiting on the owner. */
export function OwnerInboxPage() {
  const { data, isLoading, error } = useListAllPendingGuidanceRequestsQuery(undefined, { pollingInterval: 5000 });
  const [answer] = useAnswerGuidanceRequestMutation();
  const [busyId, setBusyId] = useState<string | null>(null);
  const [errorText, setErrorText] = useState<string | null>(null);

  const requests = (data ?? []).filter((r) => isOwnerActionRequest(r) && r.scope === "backlog-item" && r.itemId);

  const respond = async (id: string, response: string) => {
    setBusyId(id);
    setErrorText(null);
    try {
      const result = await answer({ id, answer: response }).unwrap();
      if (!result.applied) setErrorText("That request was already answered or cancelled.");
    } catch (err) {
      setErrorText(getErrorMessage(err, "Failed to submit the response."));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <main className={styles.page} data-testid="owner-inbox">
      <h1 className={styles.title}>
        Owner Inbox <span className={styles.count}>{requests.length}</span>
      </h1>
      {errorText && (
        <p className={styles.error} role="alert">
          {errorText}
        </p>
      )}
      {isLoading && <p className={styles.muted}>Loading…</p>}
      {!isLoading && error && <p className={styles.error}>Could not load requests.</p>}
      {!isLoading && !error && requests.length === 0 && (
        <p className={styles.muted} data-testid="owner-inbox-empty">
          Nothing is waiting on you.
        </p>
      )}
      <ul className={styles.list}>
        {requests.map((r) => (
          <li key={r.id} className={styles.card} data-testid={`owner-inbox-item-${r.id}`}>
            <p className={styles.question}>{ownerActionQuestion(r)}</p>
            <Link href={`/backlog?item=${r.itemId}`} className={styles.itemLink}>
              Open backlog item
            </Link>
            <div className={styles.actions}>
              {r.options.map((option) => (
                <button
                  key={option}
                  type="button"
                  className={styles.actionButton}
                  disabled={busyId === r.id}
                  onClick={() => void respond(r.id, option)}
                  data-testid={`owner-inbox-respond-${r.id}-${option}`}
                >{/* analytics-exempt */}
                  {option}
                </button>
              ))}
            </div>
          </li>
        ))}
      </ul>
    </main>
  );
}
