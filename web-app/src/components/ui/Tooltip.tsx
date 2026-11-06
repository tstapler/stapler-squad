"use client";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { Link } from "@/lib/nav";
import { tooltipContent, tooltipArrow, tooltipLink } from "./Tooltip.css";

type Side = "top" | "right" | "bottom" | "left";

export function Tooltip({
  children,
  label,
  side = "top",
  helpSlug,
}: {
  children: React.ReactNode;
  label: string;
  side?: Side;
  /** Slug of a doc in public/docs/; adds a "Learn more" link to /help?doc=<slug>. */
  helpSlug?: string;
}) {
  return (
    <TooltipPrimitive.Provider delayDuration={400}>
      <TooltipPrimitive.Root>
        <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
        <TooltipPrimitive.Portal>
          <TooltipPrimitive.Content className={tooltipContent} sideOffset={4} side={side}>
            {label}
            {helpSlug && (
              <Link className={tooltipLink} href={`/help?doc=${encodeURIComponent(helpSlug)}`}>
                Learn more →
              </Link>
            )}
            <TooltipPrimitive.Arrow className={tooltipArrow} />
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  );
}
