import * as React from "react";

/**
 * Text field. 2px radius, hairline border, green focus ring. Set `mono` for any figure.
 *
 * @startingPoint section="Forms" subtitle="Text, currency and search fields with states" viewport="700x260"
 */
export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  size?: "sm" | "md" | "lg";
  /** Leading Lucide glyph, e.g. "search". */
  icon?: string;
  /** Static leading text, e.g. "$". */
  prefix?: string;
  /** Static trailing text, e.g. "%" or "APY". */
  suffix?: string;
  /** Tabular mono figures — use for money, rates, account and claim numbers. */
  mono?: boolean;
  invalid?: boolean;
}
export declare function Input(props: InputProps): React.ReactElement;
