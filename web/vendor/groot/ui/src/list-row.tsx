/**
 * The interactive list-row idiom, as a class (not a component).
 *
 * Rows stay semantic (`<li>`, `<button>`, `<div role="row">`) — this constant
 * supplies only the shared treatment: hover tint, focus ring, padding rhythm.
 * Same precedent as `tableColumnHeaderClass`: the DLS centralizes the *class*,
 * pages keep their own markup.
 *
 * Usage: `<li className={cn(listRowClass, "grid grid-cols-12")}>`
 */
export const listRowClass = "flex items-center gap-3 px-4 py-3 hover:bg-muted/40 sm:px-5";

/** Grid variant for 12-col rows (JobsTable-style), keeps the same hover/padding. */
export const listRowGridClass =
  "grid grid-cols-12 items-center gap-4 px-4 py-3 text-sm hover:bg-muted/40";
