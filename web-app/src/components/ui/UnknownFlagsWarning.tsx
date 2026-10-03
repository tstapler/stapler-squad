// +feature: settings-programs
import { Icon } from "./ProbeStatusBadge";
import * as styles from "./FlagNotes.css";

const MAX_NAMES = 3;
const MAX_NAME_LENGTH = 40;

interface UnknownFlagsWarningProps {
  id: string;
  testId: string;
  /** First token of the probed command, shown as "<program> --help". */
  program: string;
  unknown: readonly string[];
}

const clip = (name: string) =>
  name.length > MAX_NAME_LENGTH ? `${name.slice(0, MAX_NAME_LENGTH)}…` : name;

/** Soft warning: plain text referenced by aria-describedby, never role=alert or aria-invalid. */
export function UnknownFlagsWarning({ id, testId, program, unknown }: UnknownFlagsWarningProps) {
  if (unknown.length === 0) return null;
  const shown = unknown.slice(0, MAX_NAMES).map(clip).join(", ");
  const more = unknown.length - MAX_NAMES;
  const verb = unknown.length === 1 ? "is" : "are";
  const programName = program.split("/").pop() || program;

  return (
    <div id={id} className={styles.warning} data-testid={testId}>
      <Icon name="warning" />
      <span>
        <code>{shown}</code>
        {more > 0 ? ` and ${more} more` : ""} {verb} not listed in <code>{programName} --help</code>. It may still work
        (hidden or subcommand flags).
      </span>
    </div>
  );
}
