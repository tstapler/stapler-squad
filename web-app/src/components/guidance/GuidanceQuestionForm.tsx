"use client";
// +feature: session:guidance-structured

import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { QUESTION_FORM_COPY as FORM_COPY } from "@/lib/copy/notificationCopy";
import { clearDraft, readDraft, writeDraft } from "@/lib/utils/sessionDrafts";
import { ActionButton } from "@/components/ui/notification-actions/ActionButton";
import { actionButton, secondaryVariant } from "@/components/ui/notification-actions/ActionRow.css";
import * as styles from "./GuidanceQuestionForm.css";

export interface FormOption {
  label: string;
  description?: string;
}

export interface FormQuestion {
  text: string;
  header?: string;
  multiSelect: boolean;
  options: FormOption[];
}

/** What the operator chose for one question: option labels and/or a typed "Other". */
export interface QuestionAnswer {
  question: string;
  header?: string;
  selected: string[];
}

/** Past this many options a single question becomes a native select (design/ux.md 3.1). */
export const SELECT_THRESHOLD = 8;

/**
 * The answer text a durable-record route stores: the labels of a single question, or one
 * "question: labels" line per question for several.
 */
export function formatAnswers(answers: QuestionAnswer[]): string {
  if (answers.length === 1) return answers[0].selected.join(", ");
  return answers.map((a) => `${a.header || a.question}: ${a.selected.join(", ")}`).join("\n");
}

export interface GuidanceQuestionFormProps {
  /** The request id: scopes drafts and test ids. */
  id: string;
  questions: FormQuestion[];
  /** "readonly": question and options as text with Open session and no control (design/ux.md 3.1). */
  mode: "answer" | "readonly";
  /** The route can carry typed text (an agent-created durable-record row), so "Other" and Send exist. */
  canTypeText?: boolean;
  /** The payload could not be rendered or measured; adds "View full prompt in session". */
  unrenderable?: boolean;
  /** A prior send settled or is in flight: every control is `aria-disabled`. */
  locked?: boolean;
  /** Why controls are disabled right now ("Offline", "Reconnecting"); shown as their description. */
  disabledReason?: string;
  /** Called once per send. */
  onSubmit: (answers: QuestionAnswer[]) => void;
  /** Renders Open session (the row supplies its own link). */
  openControl: ReactNode;
  /** The delivery-state line, rendered below the controls. */
  delivery?: ReactNode;
  minHeight?: number;
}

type Picks = string[][];

function loadPicks(id: string, count: number): Picks {
  try {
    const parsed = JSON.parse(readDraft(`guidance:${id}`) || "null") as { picks?: Picks } | null;
    if (parsed?.picks?.length === count) return parsed.picks;
  } catch {
    // a corrupt draft is no draft
  }
  return Array.from({ length: count }, () => [] as string[]);
}

const isSingleCard = (qs: FormQuestion[]) => qs.length === 1 && !qs[0].multiSelect && qs[0].options.length > 0 && qs[0].options.length <= SELECT_THRESHOLD;

const optionText = (o: FormOption) => (o.description ? `${o.label} - ${o.description}` : o.label);

const answerOf = (q: FormQuestion, selected: string[]): QuestionAnswer => ({ question: q.text, header: q.header, selected: selected.filter(Boolean) });

/** Controls shared by every shape: the disabled state, its reason, Open session and the delivery line. */
interface Shared {
  id: string;
  questions: FormQuestion[];
  canTypeText: boolean;
  blocked: boolean;
  disabledReason?: string;
  reasonId?: string;
  minHeight?: number;
  openControl: ReactNode;
  delivery?: ReactNode;
  send: (answers: QuestionAnswer[]) => void;
}

function Footer({ s }: { s: Shared }) {
  return (
    <>
      {s.disabledReason && (
        <span id={s.reasonId} className={styles.helper}>
          {s.disabledReason}
        </span>
      )}
      {s.delivery ?? <p className={styles.helper}>{FORM_COPY.noDeliveryYet}</p>}
    </>
  );
}

/** Read-only shape: text list, no control (unmeasured harness, archived, canary, S1c fail). */
function ReadOnlyForm({ id, questions, unrenderable, openControl, delivery }: Pick<Shared, "id" | "questions" | "openControl" | "delivery"> & { unrenderable: boolean }) {
  return (
    <div className={styles.form} data-testid={`guidance-form-${id}`}>
      {questions.map((q, qi) => (
        <div key={qi} className={styles.group} data-testid="question-text">
          <p className={styles.groupLabel}>{q.text}</p>
          <ul className={styles.textList}>
            {q.options.map((o) => (
              <li key={o.label} className={styles.textOption}>
                {optionText(o)}
              </li>
            ))}
          </ul>
        </div>
      ))}
      <p className={styles.helper}>{FORM_COPY.answerInSession}</p>
      <div className={styles.optionList}>
        {unrenderable && <span className={styles.textOption}>{FORM_COPY.viewFullPrompt}</span>}
        {!delivery && openControl}
      </div>
      {delivery}
    </div>
  );
}

/** One question of up to eight options: option buttons in a labelled group; one activation sends. */
function SingleQuestionCard({ s }: { s: Shared }) {
  const uid = useId();
  const q = s.questions[0];
  const [other, setOther] = useState("");
  const labelId = `${uid}-q0`;
  return (
    <form className={styles.form} aria-label={FORM_COPY.answerFor(q.text)} data-testid={`guidance-form-${s.id}`} onSubmit={(e) => e.preventDefault()}>
      <p id={labelId} className={styles.groupLabel}>
        {q.text}
      </p>
      <div role="group" aria-labelledby={labelId} className={styles.optionList}>
        {q.options.map((o, oi) => (
          <ActionButton
            key={o.label}
            id={`option-${oi}`}
            label={optionText(o)}
            variant={secondaryVariant}
            disabled={s.blocked}
            reasonId={s.reasonId}
            minHeight={s.minHeight}
            option
            onClick={() => s.send([answerOf(q, [o.label])])}
          />
        ))}
      </div>
      {s.canTypeText && (
        <div className={styles.otherRow}>
          <label htmlFor={`${uid}-other`}>{FORM_COPY.otherLabel}</label>
          <input
            id={`${uid}-other`}
            type="text"
            className={styles.otherField}
            value={other}
            data-testid="guidance-other"
            enterKeyHint="send"
            autoComplete="off"
            onFocus={(e) => e.currentTarget.scrollIntoView?.({ block: "nearest" })}
            onChange={(e) => setOther(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.nativeEvent.isComposing && other.trim()) {
                e.preventDefault();
                s.send([answerOf(q, [other.trim()])]);
              }
            }}
          />
          <ActionButton id="other-send" label={FORM_COPY.send} minHeight={s.minHeight} disabled={s.blocked || other.trim() === ""} reasonId={s.reasonId} onClick={() => s.send([answerOf(q, [other.trim()])])} />
        </div>
      )}
      {!s.delivery && <div className={styles.optionList}>{s.openControl}</div>}
      <Footer s={s} />
    </form>
  );
}

interface GroupProps {
  q: FormQuestion;
  qi: number;
  total: number;
  picked: string[];
  other: string;
  error?: string;
  s: Shared;
  groupRef: (el: HTMLElement | null) => void;
  onToggle: (label: string) => void;
  onOther: (text: string) => void;
}

function Choices({ q, picked, s, onToggle, name }: Pick<GroupProps, "q" | "picked" | "s" | "onToggle"> & { name: string }) {
  if (!q.multiSelect && q.options.length > SELECT_THRESHOLD) {
    return (
      <select
        className={styles.selectField}
        aria-label={q.text}
        value={picked[0] ?? ""}
        aria-disabled={s.blocked || undefined}
        aria-describedby={s.reasonId}
        onChange={(e) => onToggle(e.target.value)}
      >
        <option value="" />
        {q.options.map((o) => (
          <option key={o.label} value={o.label}>
            {o.label}
          </option>
        ))}
      </select>
    );
  }
  if (!q.multiSelect) {
    // Never a radiogroup: arrow keys on radios would change the selection while someone browses (design/ux.md 3.1).
    return (
      <div className={styles.optionList}>
        {q.options.map((o) => (
          <button
            key={o.label}
            type="button"
            className={`${actionButton} ${secondaryVariant} ${styles.toggleButton}`}
            aria-pressed={picked.includes(o.label)}
            aria-disabled={s.blocked || undefined}
            aria-describedby={s.reasonId}
            style={{ minHeight: s.minHeight }}
            onClick={() => onToggle(o.label)}
          >
            {optionText(o)}
          </button>
        ))}
      </div>
    );
  }
  return (
    <>
      {q.options.map((o) => (
        <label key={o.label} className={styles.choiceRow}>
          <input
            className={styles.choiceInput}
            type="checkbox"
            name={name}
            checked={picked.includes(o.label)}
            aria-disabled={s.blocked || undefined}
            aria-describedby={s.reasonId}
            onChange={() => onToggle(o.label)}
          />
          <span>
            {o.label}
            {o.description && <span className={styles.description}> - {o.description}</span>}
          </span>
        </label>
      ))}
    </>
  );
}

function QuestionGroup({ q, qi, total, picked, other, error, s, groupRef, onToggle, onOther }: GroupProps) {
  const uid = useId();
  const errId = `${uid}-err`;
  const legend = total > 1 ? `${FORM_COPY.questionOf(qi + 1, total)}: ${q.text}` : q.text;
  return (
    <fieldset
      ref={groupRef}
      className={styles.group}
      data-testid={`question-group-${qi}`}
      aria-invalid={error ? true : undefined}
      aria-describedby={error ? errId : undefined}
    >
      <legend className={styles.groupLabel}>
        {legend} {q.multiSelect && FORM_COPY.chooseAny}
      </legend>
      <Choices q={q} picked={picked} s={s} onToggle={onToggle} name={`${uid}-choice`} />
      {s.canTypeText && (
        <div className={styles.otherRow}>
          <label htmlFor={`${uid}-other`}>{FORM_COPY.otherLabel}</label>
          <input id={`${uid}-other`} type="text" className={styles.otherField} value={other} autoComplete="off" onChange={(e) => onOther(e.target.value)} />
        </div>
      )}
      {error && (
        <p id={errId} className={styles.fieldError} data-testid={`question-error-${qi}`}>
          {error}
        </p>
      )}
    </fieldset>
  );
}

/** Multi-select, multi-question and long-list shapes: stacked groups and one explicit Submit. */
function SelectionForm({ s }: { s: Shared }) {
  const { id, questions } = s;
  const [picks, setPicks] = useState<Picks>(() => loadPicks(id, questions.length));
  const [others, setOthers] = useState<string[]>(() => questions.map(() => ""));
  const [errors, setErrors] = useState<Record<number, string>>({});
  const groupRefs = useRef<(HTMLElement | null)[]>([]);

  const persist = (next: Picks) => {
    setPicks(next);
    writeDraft(`guidance:${id}`, next.some((p) => p.length > 0) ? JSON.stringify({ picks: next }) : "");
  };
  const collect = () => questions.map((q, qi) => answerOf(q, [...picks[qi], ...(s.canTypeText ? [others[qi].trim()] : [])]));
  const firstEmpty = collect().findIndex((a) => a.selected.length === 0);

  const toggle = (qi: number, label: string) => {
    if (s.blocked) return;
    const cur = picks[qi];
    const next = questions[qi].multiSelect ? (cur.includes(label) ? cur.filter((l) => l !== label) : [...cur, label]) : [label];
    persist(picks.map((p, i) => (i === qi ? next : p)));
    setErrors(({ [qi]: _cleared, ...rest }) => rest);
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const answers = collect();
    const flagged: Record<number, string> = {};
    answers.forEach((a, i) => {
      if (a.selected.length === 0) flagged[i] = FORM_COPY.chooseAtLeastOne;
    });
    setErrors(flagged);
    const first = answers.findIndex((a) => a.selected.length === 0);
    if (first >= 0) {
      const group = groupRefs.current[first];
      (group?.querySelector<HTMLElement>("input, select, button") ?? group)?.focus();
      return;
    }
    s.send(answers);
  };

  const clear = () => {
    if (s.blocked) return;
    persist(questions.map(() => []));
    setOthers(questions.map(() => ""));
    setErrors({});
  };

  return (
    <form className={styles.form} aria-label={FORM_COPY.answerFor(questions[0].text)} data-testid={`guidance-form-${id}`} onSubmit={submit}>
      {questions.map((q, qi) => (
        <QuestionGroup
          key={qi}
          q={q}
          qi={qi}
          total={questions.length}
          picked={picks[qi]}
          other={others[qi]}
          error={errors[qi]}
          s={s}
          groupRef={(el) => {
            groupRefs.current[qi] = el;
          }}
          onToggle={(label) => toggle(qi, label)}
          onOther={(text) => setOthers(others.map((v, i) => (i === qi ? text : v)))}
        />
      ))}
      {questions.some((q) => q.multiSelect) && (
        <p className={styles.helper} data-testid="selected-count">
          {FORM_COPY.selectedCount(picks.reduce((n, p) => n + p.length, 0))}
        </p>
      )}
      {firstEmpty >= 0 && questions.length > 1 && <p className={styles.helper}>{FORM_COPY.answerToContinue(firstEmpty + 1)}</p>}
      <div className={styles.submitRow}>
        <button
          type="submit"
          className={`${actionButton} ${secondaryVariant}`}
          data-testid="submit-answers"
          aria-disabled={s.blocked || firstEmpty >= 0 ? true : undefined}
          aria-describedby={s.reasonId}
          style={{ minHeight: s.minHeight }}
        >
          {FORM_COPY.submitAnswers}
        </button>
        <button type="button" className={actionButton} data-testid="clear-answers" aria-disabled={s.blocked || undefined} style={{ minHeight: s.minHeight }} onClick={clear}>
          {FORM_COPY.clear}
        </button>
        {!s.delivery && s.openControl}
      </div>
      <Footer s={s} />
    </form>
  );
}

/**
 * The question UI shared by GuidanceRequestPanel and ActionRow (design/ux.md 3). A single
 * select question is a group of option buttons where one activation sends; multi-select and
 * multi-question are stacked groups with one explicit Submit; more than eight options is a
 * native select. "Other" free text exists only on routes that can carry text.
 */
export function GuidanceQuestionForm(props: GuidanceQuestionFormProps) {
  const { id, questions, mode, canTypeText = false, unrenderable = false, locked = false, disabledReason, onSubmit, openControl, delivery, minHeight } = props;
  const uid = useId();
  const blocked = locked || !!disabledReason;
  // One activation sends: a second tap before the parent locks the card must not send again.
  const sentRef = useRef(false);
  useEffect(() => {
    if (!locked) sentRef.current = false;
  }, [locked]);
  if (mode === "readonly" || questions.length === 0) {
    return <ReadOnlyForm id={id} questions={questions} unrenderable={unrenderable} openControl={openControl} delivery={delivery} />;
  }
  const shared: Shared = {
    id,
    questions,
    canTypeText,
    blocked,
    disabledReason,
    reasonId: disabledReason ? `${uid}-reason` : undefined,
    minHeight,
    openControl,
    delivery,
    send: (answers) => {
      if (blocked || sentRef.current) return;
      sentRef.current = true;
      clearDraft(`guidance:${id}`);
      onSubmit(answers);
    },
  };
  return isSingleCard(questions) ? <SingleQuestionCard s={shared} /> : <SelectionForm s={shared} />;
}
