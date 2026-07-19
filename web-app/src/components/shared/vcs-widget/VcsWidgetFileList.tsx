"use client";

import { useState } from "react";
import {
  AlertTriangle,
  Copy,
  EyeOff,
  FileEdit,
  FileMinus,
  FilePlus,
  FileQuestion,
  FileSymlink,
  File as FileIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { FileChangeSection, FileChangeStatus, FileChangeSummary } from "@/lib/vcs/types";
import * as styles from "./VcsWidgetFileList.css";

interface VcsWidgetFileListProps {
  fileChanges: FileChangeSummary[];
  onNavigateToFile?: (path: string) => void;
}

const STATUS_META: Record<FileChangeStatus, { icon: LucideIcon; label: string }> = {
  modified: { icon: FileEdit, label: "Modified" },
  added: { icon: FilePlus, label: "Added" },
  deleted: { icon: FileMinus, label: "Deleted" },
  renamed: { icon: FileSymlink, label: "Renamed" },
  copied: { icon: Copy, label: "Copied" },
  untracked: { icon: FileQuestion, label: "Untracked" },
  ignored: { icon: EyeOff, label: "Ignored" },
  conflict: { icon: AlertTriangle, label: "Conflict — resolve before merging" },
  unknown: { icon: FileIcon, label: "Unknown" },
};

const SECTION_ORDER: FileChangeSection[] = ["conflict", "staged", "unstaged", "untracked"];

const SECTION_TITLE: Record<FileChangeSection, string> = {
  conflict: "Conflicts",
  staged: "Staged Changes",
  unstaged: "Unstaged Changes",
  untracked: "Untracked Files",
};

const CAP = 20;

function FileRow({
  file,
  onNavigateToFile,
}: {
  file: FileChangeSummary;
  onNavigateToFile?: (path: string) => void;
}) {
  const meta = STATUS_META[file.status] ?? STATUS_META.unknown;
  const Icon = meta.icon;
  const hasStats = file.additions > 0 || file.deletions > 0;
  const pathLabel = file.oldPath ? `${file.oldPath} → ${file.path}` : file.path;

  const content = (
    <>
      <Icon aria-hidden="true" size={14} className={styles.statusIcon} />
      <span className={styles.filePath}>{pathLabel}</span>
      {hasStats && (
        <span className={styles.fileStats}>
          {file.additions > 0 && <span className={styles.additions}>+{file.additions}</span>}
          {file.deletions > 0 && <span className={styles.deletions}>-{file.deletions}</span>}
        </span>
      )}
    </>
  );

  if (onNavigateToFile) {
    return (
      <button
        type="button"
        className={styles.fileRow}
        onClick={() => onNavigateToFile(file.path)}
        title={meta.label}
      >
        {content}
      </button>
    );
  }

  return (
    <span className={styles.fileRowStatic} title={meta.label}>
      {content}
    </span>
  );
}

function FileSection({
  section,
  files,
  onNavigateToFile,
}: {
  section: FileChangeSection;
  files: FileChangeSummary[];
  onNavigateToFile?: (path: string) => void;
}) {
  const [showAll, setShowAll] = useState(false);
  if (files.length === 0) return null;

  const capped = section !== "conflict" && !showAll && files.length > CAP;
  const visible = capped ? files.slice(0, CAP) : files;

  return (
    <div className={styles.section}>
      <h4 className={styles.sectionTitle}>
        {SECTION_TITLE[section]} ({files.length})
      </h4>
      <ul className={styles.fileListContainer}>
        {visible.map((f, i) => (
          <li key={`${f.path}-${i}`}>
            <FileRow file={f} onNavigateToFile={onNavigateToFile} />
          </li>
        ))}
      </ul>
      {capped && (
        <button type="button" className={styles.showAllButton} onClick={() => setShowAll(true)}>
          Show all {files.length} files
        </button>
      )}
    </div>
  );
}

export function VcsWidgetFileList({ fileChanges, onNavigateToFile }: VcsWidgetFileListProps) {
  const bySection = new Map<FileChangeSection, FileChangeSummary[]>();
  for (const section of SECTION_ORDER) bySection.set(section, []);
  for (const f of fileChanges) {
    if (!bySection.has(f.section)) bySection.set(f.section, []);
    bySection.get(f.section)!.push(f);
  }

  return (
    <div className={styles.container}>
      {SECTION_ORDER.map((section) => (
        <FileSection
          key={section}
          section={section}
          files={bySection.get(section) ?? []}
          onNavigateToFile={onNavigateToFile}
        />
      ))}
    </div>
  );
}
