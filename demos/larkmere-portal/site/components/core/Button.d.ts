import * as React from "react";

/**
 * Larkmere's action control. Verb-first labels in sentence case ("File a claim").
 * Darkens on hover, drops 1px on press. Gold is never a button.
 *
 * @startingPoint section="Core" subtitle="Primary, secondary, ghost and danger actions" viewport="700x220"
 */
export interface ButtonProps extends React.HTMLAttributes<HTMLElement> {
  children: React.ReactNode;
  /** primary = the one committing action per view. danger names its object. */
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  /** Lucide name, e.g. "shield". */
  icon?: string;
  iconPosition?: "left" | "right";
  fullWidth?: boolean;
  disabled?: boolean;
  type?: "button" | "submit" | "reset";
  /** Renders an <a> instead of a <button>. */
  href?: string;
}
export declare function Button(props: ButtonProps): React.ReactElement;
