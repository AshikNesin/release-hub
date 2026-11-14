/**
 * UI component barrel — single import surface for all primitives.
 *
 *   import { Button, Dialog, Input } from "@groot/ui";
 *
 * Design tokens live in packages/shell/src/index.css (Tailwind v4 CSS-first: oklch
 * `@theme inline` variables) and are surfaced via semantic Tailwind classes
 * (bg-primary, text-foreground, border-border, …). Do not use raw palette
 * colors (gray-*, red-*, …) outside index.css.
 *
 * This is the component **kit shipped to child projects**, not groot's own
 * app surface — some primitives (alert, pagination, select, tooltip) may have
 * no importers inside this repo. That is not dead code: they exist so child
 * apps can consume them from `@groot/ui` instead of vendoring. Removal
 * decisions for this package must weigh child-project demand (see
 * docs/guides/sync.md), not just the in-repo import graph.
 */

export * from "./badge";
export * from "./alert";
export * from "./breadcrumb";
export * from "./button";
export * from "./card";
export * from "./checkbox";
export * from "./command";
export * from "./dialog";
export * from "./dropdown-menu";
export { EmptyState, ErrorState } from "./empty-state";
export * from "./form";
export * from "./input";
export * from "./input-group";
export * from "./label";
export * from "./pagination";
export { Skeleton, SkeletonCard, SkeletonList, SkeletonTable } from "./loading-skeleton";
export { LoadingSpinner, LoadingState } from "./loading-spinner";
export * from "./select";
export { StatusBadge } from "./status-badge";
export type { StatusVariant } from "./status-badge";
export * from "./table";
export { Text, metaLabelClass, monoClass, h1Class, statClass, displayClass } from "./typography";
export { DataField, DataFields } from "./display";
export { listRowClass, listRowGridClass } from "./list-row";
export { StatTile } from "./stat-tile";
export * from "./tabs";
export * from "./textarea";
export { Toaster } from "./sonner";
export * from "./tooltip";
export { GrootUIProvider } from "./provider";
