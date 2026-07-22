"use client";
import { createContext, useContext, type ReactNode } from "react";
import * as Accordion from "@radix-ui/react-accordion";
import * as styles from "./Collapsible.css";

// Signals to a descendant CollapsibleSection that it's rendered inside a
// CollapsibleGroup's shared Accordion.Root, so it should render only its
// Item/Trigger/Content (no Root of its own) — this is what makes Radix's
// Home/End/Arrow roving-tabindex keyboard nav span all sibling headers
// (ADR-027; Radix's roving tabindex is scoped per Root, not global).
const CollapsibleGroupContext = createContext(false);

interface CollapsibleGroupProps {
  children: ReactNode;
  className?: string;
  /** Initially-open section keys. Uncontrolled; omit to start with all sections closed. */
  defaultValue?: string[];
  /** Controlled open-section keys, paired with onValueChange. */
  value?: string[];
  onValueChange?: (value: string[]) => void;
}

/**
 * Wraps a single Accordion.Root (type="multiple") shared by every child
 * CollapsibleSection — render once around a group of sibling sections so
 * their headers share one Radix roving-tabindex context (Task 1.1.1c).
 */
export function CollapsibleGroup({
  children,
  className,
  defaultValue,
  value,
  onValueChange,
}: CollapsibleGroupProps) {
  const rootClassName = className ? `${styles.root} ${className}` : styles.root;
  return (
    <CollapsibleGroupContext.Provider value={true}>
      {value !== undefined ? (
        <Accordion.Root
          type="multiple"
          className={rootClassName}
          value={value}
          onValueChange={onValueChange}
        >
          {children}
        </Accordion.Root>
      ) : (
        <Accordion.Root
          type="multiple"
          className={rootClassName}
          defaultValue={defaultValue ?? []}
          onValueChange={onValueChange}
        >
          {children}
        </Accordion.Root>
      )}
    </CollapsibleGroupContext.Provider>
  );
}

interface CollapsibleSectionProps {
  /** Unique key within the section's CollapsibleGroup (or item, when standalone); also the localStorage suffix (see useSectionExpandState). */
  sectionKey: string;
  title: ReactNode;
  /** Initial open state. Only used for standalone (non-grouped) usage — inside a CollapsibleGroup, initial open state is controlled by the group's defaultValue. */
  defaultExpanded?: boolean;
  /** Fires with the section's new open state. Only used for standalone (non-grouped) usage. */
  onExpandedChange?: (expanded: boolean) => void;
  children: ReactNode;
  className?: string;
}

function CollapsibleItem({
  sectionKey,
  title,
  children,
  className,
}: Pick<CollapsibleSectionProps, "sectionKey" | "title" | "children" | "className">) {
  return (
    <Accordion.Item value={sectionKey} className={className ? `${styles.item} ${className}` : styles.item}>
      <Accordion.Header>
        <Accordion.Trigger
          className={styles.header}
          data-testid={`collapsible-header-${sectionKey}`}
        >
          <span>{title}</span>
          <span className={styles.chevron} aria-hidden="true">
            ▸
          </span>
        </Accordion.Trigger>
      </Accordion.Header>
      <Accordion.Content className={styles.content}>
        <div className={styles.contentInner}>{children}</div>
      </Accordion.Content>
    </Accordion.Item>
  );
}

/**
 * A single progressive-disclosure section: a real `<button aria-expanded>`
 * header (never a `<div onClick>`) whose body is removed from the DOM (not
 * just visually hidden) while collapsed.
 *
 * Must be rendered either as a descendant of a `CollapsibleGroup` (to share
 * that group's single Radix Root and its cross-header keyboard nav), or
 * standalone — in which case it mounts its own implicit single-item
 * `Accordion.Root` so existing single-section call sites need no changes.
 */
export function CollapsibleSection({
  sectionKey,
  title,
  defaultExpanded = false,
  onExpandedChange,
  children,
  className,
}: CollapsibleSectionProps) {
  const insideGroup = useContext(CollapsibleGroupContext);

  if (insideGroup) {
    if (process.env.NODE_ENV !== "production" && (defaultExpanded || onExpandedChange)) {
      console.warn(
        `CollapsibleSection "${sectionKey}": defaultExpanded/onExpandedChange are ignored inside a CollapsibleGroup; use the group's defaultValue/onValueChange instead.`,
      );
    }
    return (
      <CollapsibleItem sectionKey={sectionKey} title={title} className={className}>
        {children}
      </CollapsibleItem>
    );
  }

  return (
    <Accordion.Root
      type="multiple"
      className={styles.root}
      defaultValue={defaultExpanded ? [sectionKey] : []}
      onValueChange={(next) => onExpandedChange?.(next.includes(sectionKey))}
    >
      <CollapsibleItem sectionKey={sectionKey} title={title} className={className}>
        {children}
      </CollapsibleItem>
    </Accordion.Root>
  );
}
