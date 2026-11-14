import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "./lib/utils";

/**
 * The type scale is CLOSED. These sizes (and PageLayout's h1) are the only
 * text sizes in the app; pages never pick a `text-*` size ad hoc. Rules and
 * rationale: docs/features/dls.md (adapted from Cloudflare Kumo's design
 * skill, MIT).
 */
export const metaLabelClass =
  "font-medium text-[11px] uppercase tracking-wider text-muted-foreground";

/** Inline monospaced text renders slightly smaller than the prose around it. */
export const monoClass = "font-mono text-[0.9em]";

/** In-page heading (a `Section` title, card title, or standalone label). */
const titleClass = "font-heading text-base leading-snug font-medium";
/** Routed-page title — the one sanctioned `<h1>` treatment (`PageLayout`). */
export const h1Class = "text-xl font-semibold text-foreground sm:text-2xl";
/** Standalone numeric value (a metric, not a heading). */
export const statClass = "text-xl font-semibold tabular-nums";
/** Marketing/display moments (auth brand panels); never for content. */
export const displayClass = "text-3xl font-semibold leading-tight";

const textVariants = cva("text-foreground", {
  variants: {
    variant: {
      /** In-page heading (a `Section` title, card title, or standalone label). */
      title: titleClass,
      /** Default reading size — body, labels, buttons, data. */
      body: "text-sm",
      /** Support text: timestamps, counts, helper copy. */
      meta: "text-xs text-muted-foreground",
      /** Inline monospace (ids, cron expressions, tokens) at optical size. */
      code: monoClass,
      /** Standalone numeric value (a metric, not a heading). */
      stat: statClass,
      /** Marketing/display moments (auth brand panels); never for content. */
      display: displayClass,
    },
    tone: {
      default: "",
      muted: "text-muted-foreground",
      primary: "text-primary",
      destructive: "text-destructive",
      success: "text-success",
      warning: "text-warning",
      info: "text-info",
      inverted: "text-primary-foreground",
    },
  },
  defaultVariants: { variant: "body", tone: "default" },
});

interface TextProps extends React.ComponentProps<"span">, VariantProps<typeof textVariants> {}

/**
 * Text owns the type scale. Semantic markup stays the caller's job: wrap it
 * in (or render inside) an `h1`–`h6`, `p`, `dt`, … as appropriate — `Text`
 * only locks the size/weight so pages can't drift.
 *
 * @example
 * ```tsx
 * <Text variant="title">Recent activity</Text>
 * <Text variant="meta">Updated 2 minutes ago</Text>
 * <Text variant="code" tone="muted">{job.id}</Text>
 * ```
 */
export function Text({ className, variant, tone, ...props }: TextProps) {
  return <span className={cn(textVariants({ variant, tone }), className)} {...props} />;
}
