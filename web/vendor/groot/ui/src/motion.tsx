import * as React from "react";
import { AnimatePresence, motion, useReducedMotion, type Transition } from "motion/react";

import { cn } from "./lib/utils";

/**
 * Shared motion for @groot/ui, adapted from Fluid Functionalism
 * (https://www.fluidfunctionalism.com/docs/motion): the spring tiers, the
 * deferred-unmount presence modal surfaces animate out with, and the fluid
 * hover highlight menus, selects and the command menu share.
 */

/**
 * Spring tiers: critically damped `moderate` for short travel, a slightly
 * bouncy `slow` for panels. Exits run faster than entrances so a closing
 * surface clears before the next one lands.
 */
export const spring = {
  moderate: {
    type: "spring",
    duration: 0.16,
    bounce: 0,
    exit: { duration: 0.12 },
  },
  slow: {
    type: "spring",
    duration: 0.24,
    bounce: 0.12,
    exit: { duration: 0.16 },
  },
} as const;

type Tier = (typeof spring)[keyof typeof spring];

/** Safety buffer past a tier's exit before the portal is force-unmounted. */
const exitFallbackMs = (tier: Tier) => Math.round(tier.exit.duration * 1000) + 100;

/**
 * Keeps a modal surface mounted through its exit animation. `mounted` turns
 * true in the render that opens it (so the portal and `open` land in one
 * commit) and false once the exit completes — on `onExitComplete`, or after a
 * timer, because rAF-driven animations stall in throttled background tabs and
 * would otherwise leave an invisible overlay (and the scroll lock) in place.
 * Under reduced motion the transition is instant.
 */
export function useModalPresence(open: boolean, tier: Tier = spring.slow) {
  const reduceMotion = useReducedMotion() ?? false;
  const [mounted, setMounted] = React.useState(open);
  if (open && !mounted) setMounted(true);

  React.useEffect(() => {
    if (open) return;
    const id = setTimeout(() => setMounted(false), reduceMotion ? 0 : exitFallbackMs(tier));
    return () => clearTimeout(id);
  }, [open, reduceMotion, tier]);

  const transition: Transition = reduceMotion
    ? { duration: 0 }
    : open
      ? { type: tier.type, duration: tier.duration, bounce: tier.bounce }
      : { duration: tier.exit.duration, ease: [0.4, 0, 1, 1] };

  return {
    mounted,
    transition,
    onExitComplete: () => {
      if (!open) setMounted(false);
    },
  };
}

type HighlightRect = { top: number; left: number; width: number; height: number };

/**
 * Fluid Functionalism's fluid hover for Radix menus and selects
 * (https://www.fluidfunctionalism.com/docs/dropdown): one highlight that
 * springs from row to row instead of each row lighting up on its own. It
 * follows the row Radix marks `data-highlighted` (pointer and keyboard
 * alike), so item behaviour is untouched. Render it as the first child of
 * the element that holds the rows; that element must be `relative isolate`.
 */
export function FluidHighlight({
  className,
  selector = "[data-highlighted]",
}: {
  className?: string;
  /** The active row: Radix marks `data-highlighted`, cmdk `data-selected="true"`. */
  selector?: string;
}) {
  const ref = React.useRef<HTMLSpanElement>(null);
  const [rect, setRect] = React.useState<HighlightRect | null>(null);
  const reduceMotion = useReducedMotion() ?? false;

  React.useLayoutEffect(() => {
    const container = ref.current?.parentElement;
    if (!container) return;
    const measure = () => {
      // Only rows of this container: a submenu's rows live in their own portal.
      const row = container.querySelector<HTMLElement>(`:scope ${selector}`);
      if (!row || row.offsetHeight === 0) {
        setRect(null);
        return;
      }
      // offsetParent-relative geometry: scrolling moves rows and highlight together.
      let top = 0;
      let left = 0;
      let el: HTMLElement | null = row;
      while (el && el !== container) {
        top += el.offsetTop;
        left += el.offsetLeft;
        el = el.offsetParent as HTMLElement | null;
      }
      const next = { top, left, width: row.offsetWidth, height: row.offsetHeight };
      setRect((prev) =>
        prev &&
        prev.top === next.top &&
        prev.left === next.left &&
        prev.width === next.width &&
        prev.height === next.height
          ? prev
          : next,
      );
    };
    measure();
    const observer = new MutationObserver(measure);
    observer.observe(container, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ["data-highlighted", "data-selected", "hidden"],
    });
    return () => observer.disconnect();
  }, [selector]);

  return (
    <span ref={ref} aria-hidden className="contents">
      <AnimatePresence>
        {rect && (
          <motion.span
            data-slot="fluid-highlight"
            className={cn("pointer-events-none absolute -z-10 rounded-lg bg-accent", className)}
            initial={{ ...rect, opacity: 0 }}
            animate={{ ...rect, opacity: 1 }}
            exit={{ opacity: 0, transition: { duration: 0.06 } }}
            transition={
              reduceMotion ? { duration: 0 } : { ...spring.moderate, opacity: { duration: 0.08 } }
            }
          />
        )}
      </AnimatePresence>
    </span>
  );
}
