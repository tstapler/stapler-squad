"use client";

import { forwardRef } from "react";
import {
  input,
  inputLabel,
  inputError,
  inputWrapper,
} from "./Input.css";

type InputSize = "sm" | "md" | "lg";
type InputState = "default" | "error" | "disabled";

// Omit the native HTML `size` (a number attribute) so our string size variant wins.
type InputProps = Omit<React.InputHTMLAttributes<HTMLInputElement>, "size"> & {
  inputSize?: InputSize;
  state?: InputState;
  label?: string;
  error?: string;
};

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ inputSize, state: stateProp, label, error, id, ...props }, ref) => {
    const derivedState: InputState = error ? "error" : (stateProp ?? "default");
    return (
      <div className={inputWrapper({})}>
        {label && (
          <label htmlFor={id} className={inputLabel}>
            {label}
          </label>
        )}
        <input
          ref={ref}
          id={id}
          className={input({ size: inputSize, state: derivedState })}
          aria-invalid={!!error}
          aria-describedby={error ? `${id}-error` : undefined}
          {...props}
        />
        {error && (
          <span id={`${id}-error`} className={inputError} role="alert">
            {error}
          </span>
        )}
      </div>
    );
  }
);

Input.displayName = "Input";
