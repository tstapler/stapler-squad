import {
  hint as hintClass,
  keys as keysClass,
  key as keyClass,
  separator,
  description as descriptionClass,
  hintsContainer,
  title as titleClass,
  hints as hintsClass,
} from "./KeyboardHint.css";

interface KeyboardHintProps {
  keys: string | string[];
  description: string;
  className?: string;
}

export function KeyboardHint({ keys, description, className = "" }: KeyboardHintProps) {
  const keyArray = Array.isArray(keys) ? keys : [keys];

  return (
    <div className={`${hintClass} ${className}`}>
      <div className={keysClass}>
        {keyArray.map((key, index) => (
          <span key={index}>
            <kbd className={keyClass}>{key}</kbd>
            {index < keyArray.length - 1 && (
              <span className={separator}>+</span>
            )}
          </span>
        ))}
      </div>
      <span className={descriptionClass}>{description}</span>
    </div>
  );
}

interface KeyboardHintsProps {
  hints: Array<{
    keys: string | string[];
    description: string;
  }>;
  title?: string;
  className?: string;
}

export function KeyboardHints({ hints, title, className = "" }: KeyboardHintsProps) {
  return (
    <div className={`${hintsContainer} ${className}`}>
      {title && <h3 className={titleClass}>{title}</h3>}
      <div className={hintsClass}>
        {hints.map((hint, index) => (
          <KeyboardHint
            key={index}
            keys={hint.keys}
            description={hint.description}
          />
        ))}
      </div>
    </div>
  );
}
