"use client";

import { forwardRef } from "react";
import { Slot } from "@radix-ui/react-slot";
import { button, type ButtonVariants } from "./Button.css";

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> &
  ButtonVariants & {
    asChild?: boolean;
  };

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ asChild, intent, size, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return <Comp ref={ref} className={button({ intent, size })} {...props} />;
  }
);

Button.displayName = "Button";
