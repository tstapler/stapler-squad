"use client";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { tooltipContent, tooltipArrow } from "./Tooltip.css";

type Side = "top" | "right" | "bottom" | "left";

export function Tooltip({
  children,
  label,
  side = "top",
}: {
  children: React.ReactNode;
  label: string;
  side?: Side;
}) {
  return (
    <TooltipPrimitive.Provider delayDuration={400}>
      <TooltipPrimitive.Root>
        <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
        <TooltipPrimitive.Portal>
          <TooltipPrimitive.Content className={tooltipContent} sideOffset={4} side={side}>
            {label}
            <TooltipPrimitive.Arrow className={tooltipArrow} />
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  );
}
