import * as React from "react";

/** A single-glyph action. Hover fills a circular bone field; `label` is required for a11y. */
export interface IconButtonProps extends React.HTMLAttributes<HTMLButtonElement> {
  /** Lucide name. */
  icon: string;
  /** Accessible label and tooltip — always supply one. */
  label: string;
  size?: "sm" | "md" | "lg";
  variant?: "ghost" | "solid";
  disabled?: boolean;
}
export declare function IconButton(props: IconButtonProps): React.ReactElement;
