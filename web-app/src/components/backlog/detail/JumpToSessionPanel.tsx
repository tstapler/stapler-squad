"use client";
// +feature: backlog:jump-to-session

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ChevronDown } from "lucide-react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import { SessionDetail } from "@/components/sessions/SessionDetail";
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
 * jumpToSession.ts), expanded in place in the page's own layout rather than
 * navigating away or opening a modal. A secondary caret menu lets the
 * operator pick a different role's most recent session when more than one
 * is available.
 */
export function JumpToSessionPanel({ linkedSessions, backlogItemId, getSession }: JumpToSessionPanelProps) {
  const { track } = useAnalytics();
  const defaultTarget = getMostRecentJumpableSession(linkedSessions);
  const roleOptions = listJumpableRoleOptions(linkedSessions);

  const [expanded, setExpanded] = useState(false);
  const [selectedRole, setSelectedRole] = useState<string | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [menuPos, setMenuPos] = useState({ top: 0, left: 0 });
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const menuTriggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuId = useId();
  const fetchSeqRef = useRef(0);

  useFocusTrap(menuRef, menuOpen, menuTriggerRef);

  const target = selectedRole
    ? getMostRecentJumpableSessionForRole(linkedSessions, selectedRole) ?? defaultTarget
    : defaultTarget;

  useEffect(() => {
    if (!expanded || !target) return;
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
  }, [expanded, target, getSession]);

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
          className={styles.jumpButton}
          aria-expanded={expanded}
          aria-controls={PANEL_ID}
          data-testid="jump-to-session-button"
          onClick={() => {
            const next = !expanded;
            setExpanded(next);
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
          {expanded ? "Hide" : "Jump to"} {roleLabel(target.role)} session
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
                  setExpanded(true);
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
      {expanded && (
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
