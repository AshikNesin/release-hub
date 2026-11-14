import * as React from "react";
import { Tooltip as TooltipPrimitive } from "radix-ui";
import { motion, useReducedMotion } from "motion/react";
import { cn } from "./lib/utils";

/**
 * Tooltip with Fluid Functionalism's look and motion
 * (https://www.fluidfunctionalism.com/docs/tooltip): an inverted chip that
 * fades in while sliding 4px off its side, and fades out (staying mounted
 * through the exit) instead of vanishing.
 */

/**
 * Minimal tooltip provider. Mount once near the app root; required for any
 * `Tooltip` below it to function.
 */
function TooltipProvider({
  delayDuration = 200,
  skipDelayDuration = 300,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipPrimitive.Provider
      delayDuration={delayDuration}
      skipDelayDuration={skipDelayDuration}
      {...props}
    />
  );
}

const SLIDE = { top: { y: 4 }, bottom: { y: -4 }, left: { x: 4 }, right: { x: -4 } } as const;
const ENTER = { type: "spring", duration: 0.08, bounce: 0 } as const;
const EXIT = { duration: 0.06 } as const;

/**
 * Convenience wrapper: `Provider` + `Root` + `Trigger` + `Content` in one
 * component for the common single-anchor case. For multi-trigger or controlled
 * usage, compose the primitives directly.
 *
 * @example
 * <Tooltip content="Save">
 *   <Button>Save</Button>
 * </Tooltip>
 */
function Tooltip({
  children,
  content,
  side = "right",
  className,
  open: openProp,
  defaultOpen,
  onOpenChange,
  delayDuration,
  disableHoverableContent,
  sideOffset = 6,
  ...props
}: {
  children: React.ReactNode;
  content: React.ReactNode;
  side?: "top" | "right" | "bottom" | "left";
} & Omit<React.ComponentProps<typeof TooltipPrimitive.Root>, "children"> &
  Omit<React.ComponentProps<typeof TooltipPrimitive.Content>, "content" | "side">) {
  const [uncontrolled, setUncontrolled] = React.useState(defaultOpen ?? false);
  const open = openProp ?? uncontrolled;
  const [mounted, setMounted] = React.useState(open);
  if (open && !mounted) setMounted(true);
  const reduceMotion = useReducedMotion() ?? false;

  React.useEffect(() => {
    if (open) return;
    const id = setTimeout(() => setMounted(false), 160);
    return () => clearTimeout(id);
  }, [open]);

  return (
    <TooltipPrimitive.Root
      open={open}
      onOpenChange={(next) => {
        setUncontrolled(next);
        onOpenChange?.(next);
      }}
      delayDuration={delayDuration}
      disableHoverableContent={disableHoverableContent}
    >
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      {mounted && (
        <TooltipPrimitive.Portal forceMount>
          <TooltipPrimitive.Content
            side={side}
            sideOffset={sideOffset}
            forceMount
            className="z-50"
            {...props}
          >
            <motion.div
              className={cn(
                "rounded-lg bg-foreground px-2.5 py-1.5 text-xs font-medium text-background shadow-md",
                className,
              )}
              initial={{ opacity: 0, ...SLIDE[side] }}
              animate={{ opacity: open ? 1 : 0, x: 0, y: 0 }}
              transition={reduceMotion ? { duration: 0 } : open ? ENTER : EXIT}
              onAnimationComplete={() => {
                if (!open) setMounted(false);
              }}
            >
              {content}
            </motion.div>
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      )}
    </TooltipPrimitive.Root>
  );
}

export { Tooltip, TooltipProvider };
