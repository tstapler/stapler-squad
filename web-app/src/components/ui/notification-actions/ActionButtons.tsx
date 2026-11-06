"use client";

import type { ReactNode } from "react";
import { ActionKind, type NotificationActionDescriptor } from "@/gen/session/v1/notification_action_pb";
import { cancelPrompt, pendingLabel, type HoldKey } from "@/lib/copy/notificationCopy";
import { ActionButton } from "./ActionButton";
import { CANCEL_KINDS, OPTION_KINDS, kindSlug, pendingVerb, type Pick } from "./actionSpec";
import { APPROVE_EXTRA_GAP_PX } from "./layout";
import { approveVariant, buttons, denyVariant, secondaryVariant } from "./ActionRow.css";

export interface ButtonsUi {
  /** Pending, rate-limited or offline: every action is `aria-disabled`. */
  blocked: boolean;
  mobile: boolean;
  minHeight?: number;
  /** Kind currently in flight, for its "Approving..." label. */
  pendingKind?: ActionKind;
  /** Id of the element describing why `d` cannot run, if any. */
  reasonFor: (d: NotificationActionDescriptor) => string | undefined;
  onPress: (p: Pick) => void;
  /** A commit delay is running: the pressed button reads "Approving in 3s". */
  holding?: { key: HoldKey; text: string };
}

function PrimaryButton({ d, ui }: { d: NotificationActionDescriptor; ui: ButtonsUi }) {
  const isApprove = d.kind === ActionKind.APPROVE;
  const holdText = isApprove && ui.holding?.key === "approve" ? ui.holding.text : undefined;
  const label = holdText ?? (ui.pendingKind === d.kind ? pendingLabel(pendingVerb(d.kind)) : d.label);
  return (
    <ActionButton
      id={kindSlug(d.kind)}
      label={label}
      variant={isApprove ? approveVariant : denyVariant}
      disabled={ui.blocked || !d.available}
      reasonId={ui.reasonFor(d)}
      minHeight={ui.minHeight}
      style={ui.mobile && isApprove ? { marginBlockStart: APPROVE_EXTRA_GAP_PX } : undefined}
      onClick={() => ui.onPress({ descriptor: d })}
    />
  );
}

/** Desktop: Approve, Deny. Phone: Deny first, so a thumb's first target is the safe one. */
export function ApprovalButtons({ approve, deny, ui }: { approve?: NotificationActionDescriptor; deny?: NotificationActionDescriptor; ui: ButtonsUi }) {
  const a = approve && <PrimaryButton key="a" d={approve} ui={ui} />;
  const d = deny && <PrimaryButton key="d" d={deny} ui={ui} />;
  return <>{ui.mobile ? [d, a] : [a, d]}</>;
}

/** Answer and decision options, one button each inside a labelled group. */
export function OptionGroup({ d, ui }: { d: NotificationActionDescriptor; ui: ButtonsUi }) {
  return (
    <div role="group" aria-label={d.label} className={buttons}>
      {d.options.map((o, i) => (
        <ActionButton
          key={`${o.value}-${o.label}`}
          id={`${kindSlug(d.kind)}-${o.value || i}`}
          label={d.kind === ActionKind.BACKLOG_DECISION && ui.holding?.key === o.value ? ui.holding.text : o.label}
          variant={secondaryVariant}
          disabled={ui.blocked || !d.available}
          reasonId={ui.reasonFor(d)}
          minHeight={ui.minHeight}
          option
          onClick={() => ui.onPress({ descriptor: d, option: o })}
        />
      ))}
    </div>
  );
}

export function CancelButton({ d, ui }: { d: NotificationActionDescriptor; ui: ButtonsUi }) {
  return (
    <ActionButton
      id={kindSlug(d.kind)}
      label={cancelPrompt(d.kind === ActionKind.CANCEL_PEER_HELP ? "request" : "prompt")}
      variant={secondaryVariant}
      disabled={ui.blocked || !d.available}
      reasonId={ui.reasonFor(d)}
      minHeight={ui.minHeight}
      onClick={() => ui.onPress({ descriptor: d })}
    />
  );
}

/** Every descriptor other than the approval pair, in server order. */
export function OtherButtons({ descriptors, ui }: { descriptors: NotificationActionDescriptor[]; ui: ButtonsUi }): ReactNode {
  return descriptors.map((d) => {
    if (OPTION_KINDS.has(d.kind)) return <OptionGroup key={`${d.kind}-${d.targetKey}`} d={d} ui={ui} />;
    if (CANCEL_KINDS.has(d.kind)) return <CancelButton key={`${d.kind}-${d.targetKey}`} d={d} ui={ui} />;
    return null;
  });
}
