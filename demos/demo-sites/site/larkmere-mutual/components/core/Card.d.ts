import * as React from "react";

/**
 * White card stock on bone. 6px radius, hairline border, whisper of a shadow.
 *
 * @startingPoint section="Core" subtitle="Resting, featured and interactive card stock" viewport="700x240"
 */
export interface CardProps extends React.HTMLAttributes<HTMLElement> {
  children: React.ReactNode;
  /** Swaps the resting shadow for a 3px gold top rule. One per view, at most. */
  featured?: boolean;
  /** Clickable: lifts to --shadow-2 on hover. */
  interactive?: boolean;
  /** Default 24px; use 16px on mobile. */
  padding?: string;
  as?: keyof JSX.IntrinsicElements;
}
export declare function Card(props: CardProps): React.ReactElement;
export declare function CardDivider(props: { style?: React.CSSProperties }): React.ReactElement;
