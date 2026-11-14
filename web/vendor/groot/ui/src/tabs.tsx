import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { Tabs as TabsPrimitive } from "radix-ui";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";

import { cn } from "./lib/utils";
import { spring } from "./motion";

/**
 * Tabs with Fluid Functionalism's motion
 * (https://www.fluidfunctionalism.com/docs/tabs): the selected pill (or, in
 * the `line` variant, the underline) springs from tab to tab instead of
 * jumping, and a fainter pill follows the pointer across the list. Radix owns
 * roles, roving focus and activation; the indicators are measured overlays.
 */

function Tabs({
  className,
  orientation = "horizontal",
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Root>) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      data-orientation={orientation}
      orientation={orientation}
      className={cn("group/tabs flex gap-2 data-horizontal:flex-col", className)}
      {...props}
    />
  );
}

const tabsListVariants = cva(
  "group/tabs-list inline-flex w-fit items-center justify-center rounded-xl p-[3px] text-muted-foreground group-data-horizontal/tabs:h-9 group-data-vertical/tabs:h-fit group-data-vertical/tabs:flex-col data-[variant=line]:rounded-none",
  {
    variants: {
      variant: {
        default: "bg-muted",
        line: "gap-1 bg-transparent",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);

type Rect = {
  left: number;
  top: number;
  width: number;
  height: number;
};

function rectWithin(el: HTMLElement, parent: HTMLElement): Rect {
  const r = el.getBoundingClientRect();
  const p = parent.getBoundingClientRect();
  return {
    left: r.left - p.left + parent.scrollLeft,
    top: r.top - p.top + parent.scrollTop,
    width: r.width,
    height: r.height,
  };
}

const sameRect = (a: Rect | null, b: Rect | null) =>
  a === b ||
  (!!a &&
    !!b &&
    a.left === b.left &&
    a.top === b.top &&
    a.width === b.width &&
    a.height === b.height);

/** Tracks the active trigger's box inside the list, through resizes and tab changes. */
function useActiveRect(listRef: React.RefObject<HTMLDivElement | null>) {
  const [rect, setRect] = React.useState<Rect | null>(null);
  React.useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const measure = () => {
      const active = list.querySelector<HTMLElement>(
        '[data-slot="tabs-trigger"][data-state="active"]',
      );
      const next = active && active.offsetWidth > 0 ? rectWithin(active, list) : null;
      setRect((prev) => (sameRect(prev, next) ? prev : next));
    };
    measure();
    const mutations = new MutationObserver(measure);
    mutations.observe(list, {
      subtree: true,
      attributes: true,
      attributeFilter: ["data-state"],
      childList: true,
    });
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    resize?.observe(list);
    for (const child of Array.from(list.children)) resize?.observe(child);
    return () => {
      mutations.disconnect();
      resize?.disconnect();
    };
  }, [listRef]);
  return rect;
}

function TabsList({
  className,
  variant = "default",
  children,
  onPointerMove,
  onPointerLeave,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.List> & VariantProps<typeof tabsListVariants>) {
  const listRef = React.useRef<HTMLDivElement>(null);
  const active = useActiveRect(listRef);
  const [hover, setHover] = React.useState<Rect | null>(null);
  const reduceMotion = useReducedMotion() ?? false;
  const line = variant === "line";
  const move = reduceMotion ? { duration: 0 } : spring.moderate;

  return (
    <TabsPrimitive.List
      ref={listRef}
      data-slot="tabs-list"
      data-variant={variant}
      className={cn(tabsListVariants({ variant }), "relative isolate", className)}
      onPointerMove={(e) => {
        onPointerMove?.(e);
        if (e.pointerType !== "mouse" || !listRef.current) return;
        const trigger = (e.target as HTMLElement).closest<HTMLElement>(
          '[data-slot="tabs-trigger"]',
        );
        const next =
          trigger && trigger.dataset.state !== "active" && !trigger.hasAttribute("disabled")
            ? rectWithin(trigger, listRef.current)
            : null;
        setHover((prev) => (sameRect(prev, next) ? prev : next));
      }}
      onPointerLeave={(e) => {
        onPointerLeave?.(e);
        setHover(null);
      }}
      {...props}
    >
      {active && (
        <motion.span
          aria-hidden
          data-slot="tabs-indicator"
          className={cn(
            "pointer-events-none absolute -z-10",
            line
              ? "bg-foreground"
              : "rounded-lg bg-background shadow-sm dark:border dark:border-input dark:bg-input/30",
          )}
          initial={false}
          animate={
            line
              ? {
                  left: active.left,
                  width: active.width,
                  top: active.top + active.height + 1,
                  height: 2,
                }
              : active
          }
          transition={move}
        />
      )}
      <AnimatePresence>
        {hover && !line && (
          <motion.span
            aria-hidden
            className="pointer-events-none absolute -z-10 rounded-lg bg-foreground/5"
            initial={{ ...hover, opacity: 0 }}
            animate={{ ...hover, opacity: 1 }}
            exit={{ opacity: 0, transition: { duration: 0.08 } }}
            transition={
              reduceMotion ? { duration: 0 } : { ...spring.moderate, opacity: { duration: 0.08 } }
            }
          />
        )}
      </AnimatePresence>
      {children}
    </TabsPrimitive.List>
  );
}

function TabsTrigger({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      className={cn(
        "relative inline-flex h-[calc(100%-1px)] flex-1 items-center justify-center gap-1.5 rounded-lg border border-transparent px-1.5 py-0.5 text-sm font-medium whitespace-nowrap text-foreground/60 group-data-vertical/tabs:w-full group-data-vertical/tabs:justify-start hover:text-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-1 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-50 has-data-[icon=inline-end]:pr-1 has-data-[icon=inline-start]:pl-1 dark:text-muted-foreground dark:hover:text-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        // The selected look is the list's sliding indicator, not the trigger's own background.
        "data-[state=active]:text-foreground dark:data-[state=active]:text-foreground",
        className,
      )}
      {...props}
    />
  );
}

function TabsContent({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return (
    <TabsPrimitive.Content
      data-slot="tabs-content"
      className={cn("flex-1 text-sm outline-none", className)}
      {...props}
    />
  );
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
