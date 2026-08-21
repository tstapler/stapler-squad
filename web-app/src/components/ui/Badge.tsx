import { badge, type BadgeVariants } from "./Badge.css";

type BadgeProps = React.HTMLAttributes<HTMLSpanElement> & BadgeVariants;

export function Badge({ intent, size, ...props }: BadgeProps) {
  return <span className={badge({ intent, size })} {...props} />;
}
