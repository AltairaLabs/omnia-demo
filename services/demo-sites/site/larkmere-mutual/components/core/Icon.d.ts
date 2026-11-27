import * as React from "react";

/** Lucide glyph at Larkmere's fixed 1.5px stroke. Always inherits currentColor. */
export interface IconProps {
  /** Lucide icon name, kebab-case: "shield", "file-text", "chevron-right". */
  name: string;
  /** 16 inline with text, 20 in buttons/nav, 24 standalone. Default 20. */
  size?: 16 | 20 | 24 | number;
  /** Leave at 1.5 — Larkmere does not scale stroke weight. */
  strokeWidth?: number;
  style?: React.CSSProperties;
  /** Set only when the icon carries meaning on its own; otherwise it is aria-hidden. */
  title?: string;
}
export declare function Icon(props: IconProps): React.ReactElement;
