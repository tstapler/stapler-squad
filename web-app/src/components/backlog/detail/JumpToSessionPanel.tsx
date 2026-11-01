"use client";
// +feature: backlog:jump-to-session

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ChevronDown } from "lucide-react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import { SessionDetail } from "@/components/sessions/SessionDetail";
import { SessionPeekModal } from "@/components/sessions/SessionPeekModal";
import { useViewport } from "@/components/providers/ViewportProvider";
import { useAnalytics } from "@/lib/analytics";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { formatDate } from "@/lib/backlog/formatDate";
import {
  getMostRecentJumpableSession,
  getMostRecentJumpableSessionForRole,
  listJumpableRoleOptions,
} from "@/lib/backlog/jumpToSession";
import * as styles from "./JumpToSessionPanel.css";

export interface JumpToSessionPanelProps {
  linkedSessions: LinkedSession[];
  backlogItemId: string;
  /** Fetches the full Session proto by id — the real-time terminal needs more than the lightweight LinkedSession the backlog item already carries (useSessionService's getSession). */
  getSession: (id: string) => Promise<Session | null>;
}

function roleLabel(role: string): string {
  return role.charAt(0).toUpperCase() + role.slice(1);
}

const PANEL_ID = "jump-to-session-panel";

/**
 * A single button that opens a real, interactive terminal for the backlog
 * item's most recently active jumpable session (work/review — see
 * jumpToSession.ts), expanded in place in the page's own layout on desktop, or in a
 * full-screen SessionPeekModal on mobile (<600px). A secondary caret menu lets the
 * operator pick a different role's most recent session when more than one
 * is available.
 */
export function JumpToSessionPanel({ linkedSessions, backlogItemId, getSession }: JumpToSessionPanelProps) {
  const { track } = useAnalytics();
  const { isMobile } = useViewport();
  const defaultTarget = getMostRecentJumpableSession(linkedSessions);
  const roleOptions = listJumpableRoleOptions(linkedSessions);

  const [expanded, setExpanded] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const isOpen = expanded || modalOpen;
  const [selectedRole, setSelectedRole] = useState<string | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [menuPos, setMenuPos] = useState({ top: 0, left: 0 });
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const jumpButtonRef = useRef<HTMLButtonElement>(null);
  const menuTriggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuId = useId();
  const fetchSeqRef = useRef(0);

  useFocusTrap(menuRef, menuOpen, menuTriggerRef);

  const target = selectedRole
    ? getMostRecentJumpableSessionForRole(linkedSessions, selectedRole) ?? defaultTarget
    : defaultTarget;

  useEffect(() => {
    if (!isOpen || !target) return;
    const seq = ++fetchSeqRef.current;
    setLoading(true);
    setError(null);
    // View-only fetch to populate the inline terminal, not a user-initiated
    // mutation — the click that triggers it is tracked separately below.
    // analytics-exempt
    getSession(target.sessionId)
      .then((result) => {
        if (seq !== fetchSeqRef.current) return;
        if (!result) {
          setError("Session not found.");
          setSession(null);
        } else {
          setSession(result);
        }
      })
      .catch(() => {
        if (seq !== fetchSeqRef.current) return;
        setError("Failed to load session.");
        setSession(null);
      })
      .finally(() => {
        if (seq === fetchSeqRef.current) setLoading(false);
      });
  }, [isOpen, target, getSession]);

  const closeModal = useCallback(() => {
    setModalOpen(false);
    // Return focus to the control that opened the modal (the portal unmount drops it to <body>).
    requestAnimationFrame(() => jumpButtonRef.current?.focus());
  }, []);

  const closeMenu = useCallback(() => setMenuOpen(false), []);

  const openMenu = useCallback(() => {
    const rect = menuTriggerRef.current?.getBoundingClientRect();
    if (rect) setMenuPos({ top: rect.bottom + 4, left: rect.left });
    setMenuOpen(true);
  }, []);

  useEffect(() => {
    if (!menuOpen) return;
    const handler = (e: MouseEvent) => {
      const node = e.target as Node;
      if (menuTriggerRef.current?.contains(node) || menuRef.current?.contains(node)) return;
      closeMenu();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [menuOpen, closeMenu]);

  if (!defaultTarget || !target) return null;

  return (
    <div>
      <div className={styles.controls}>
        <button
          type="button"
          ref={jumpButtonRef}
          className={styles.jumpButton}
          aria-expanded={isMobile ? modalOpen : expanded}
          aria-controls={isMobile ? undefined : PANEL_ID}
          data-testid="jump-to-session-button"
          onClick={() => {
            const next = isMobile ? true : !expanded;
            if (isMobile) setModalOpen(true);
            else setExpanded(next);
            if (next) {
              track({
                name: "backlog_jump_to_session",
                category: "user_action",
                component: "JumpToSessionPanel",
                labels: { role: target.role },
              });
            }
          }}
        >
          {!isMobile && expanded ? "Hide" : "Jump to"} {roleLabel(target.role)} session
        </button>
        {roleOptions.length > 1 && (
          <button
            type="button"
            ref={menuTriggerRef}
            className={styles.roleMenuTrigger}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            aria-controls={menuId}
            aria-label="Choose which session role to jump to"
            data-testid="jump-to-session-role-menu-trigger"
            // Opens/closes a disclosure menu, not an action — mirrors
            // MoveToMenu.tsx's identical untracked trigger.
            // analytics-exempt
            onClick={() => (menuOpen ? closeMenu() : openMenu())}
          >
            <ChevronDown size={16} aria-hidden="true" />
          </button>
        )}
      </div>
      {menuOpen &&
        createPortal(
          <div
            ref={menuRef}
            id={menuId}
            role="menu"
            aria-label="Session roles"
            className={styles.roleMenu}
            style={{ top: menuPos.top, left: menuPos.left }}
            data-testid="jump-to-session-role-menu"
            onKeyDown={(e) => {
              if (e.key === "Escape") closeMenu();
            }}
          >
            {roleOptions.map((s) => (
              <button
                key={s.role}
                type="button"
                role="menuitem"
                className={styles.roleMenuItem}
                data-testid={`jump-to-session-role-item-${s.role}`}
                onClick={() => {
                  setSelectedRole(s.role);
                  if (isMobile) setModalOpen(true);
                  else setExpanded(true);
                  closeMenu();
                  track({
                    name: "backlog_jump_to_session",
                    category: "user_action",
                    component: "JumpToSessionPanel",
                    labels: { role: s.role },
                  });
                }}
              >
                <span className={styles.roleMenuItemRole}>{roleLabel(s.role)}</span>
                <span className={styles.roleMenuItemDate}>{formatDate(s.startedAt)}</span>
              </button>
            ))}
          </div>,
          document.body
        )}
      {modalOpen && !loading && !error && session && (
        <SessionPeekModal session={session} onClose={closeModal} />
      )}
      {modalOpen && (loading || error) && createPortal(
        <div className={styles.panel} role="status" data-testid="jump-to-session-modal-status">
          {loading ? "Loading terminal…" : error}
          {error && (
            <button type="button" onClick={closeModal}>Close</button>
          )}
        </div>,
        document.body
      )}
      {!isMobile && expanded && (
        <div id={PANEL_ID} className={styles.panel} data-testid={PANEL_ID}>
          {loading && <div className={styles.panelLoading}>Loading terminal…</div>}
          {!loading && error && <div className={styles.panelError}>{error}</div>}
          {!loading && !error && session && (
            <SessionDetail
              session={session}
              onClose={() => setExpanded(false)}
              embedded={true}
              initialTab="terminal"
              backlogItemId={backlogItemId}
            />
          )}
        </div>
      )}
    </div>
  );
}
