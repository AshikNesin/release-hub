import type { ReactNode } from "react";

import { tableColumnHeaderClass } from "./table";
import { cn } from "./lib/utils";

/**
 * Read-only display field: the `dt` label + `dd` value idiom.
 *
 * This is the *display* counterpart to form-`Field` (`@groot/ui/form`) —
 * that one wraps a control inside a form; this one renders a read-only
 * label/value pair. Label typography is `tableColumnHeaderClass` (the one
 * meta-label treatment); values default to `text-sm` body size.
 *
 * Use `DataFields` for the responsive grid these live in, so spacing stays
 * identical wherever key-value data is shown (detail pages, overview cards,
 * metadata panels).
 */
interface DataFieldProps {
  label: ReactNode;
  children: ReactNode;
  className?: string;
  /** Extra classes for the value element (e.g. `monoClass`). */
  valueClassName?: string;
}

export function DataField({ label, children, className, valueClassName }: DataFieldProps) {
  return (
    <div className={className}>
      <dt className={tableColumnHeaderClass}>{label}</dt>
      <dd className={cn("mt-1 text-sm text-foreground", valueClassName)}>{children}</dd>
    </div>
  );
}

/**
 * Grid wrapper for `DataField`s. Responsive by default (2 cols → 4 on lg),
 * matching the JobOverview/JobDetail detail-page rhythm.
 */
interface DataFieldsProps {
  children: ReactNode;
  className?: string;
  /** Fixed column count instead of the responsive default. */
  columns?: 2 | 3 | 4;
}

export function DataFields({ children, className, columns }: DataFieldsProps) {
  const cols = columns
    ? {
        2: "grid-cols-2",
        3: "grid-cols-2 md:grid-cols-3",
        4: "grid-cols-2 md:grid-cols-3 lg:grid-cols-4",
      }[columns]
    : "grid-cols-2 md:grid-cols-3 lg:grid-cols-4";

  return <dl className={cn("grid gap-x-6 gap-y-4", cols, className)}>{children}</dl>;
}
