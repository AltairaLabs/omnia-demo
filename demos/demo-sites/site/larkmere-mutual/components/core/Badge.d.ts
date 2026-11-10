import * as React from "react";

/** Status marker on a policy, claim or transfer. Caps, 11px, 2px radius. */
export interface BadgeProps {
  children: React.ReactNode;
  tone?: "neutral" | "positive" | "caution" | "negative" | "info" | "accent";
  /** Adds a leading status dot. */
  dot?: boolean;
  style?: React.CSSProperties;
}
export declare function Badge(props: BadgeProps): React.ReactElement;
