"use client";

import * as React from "react";
import { Dialog as DialogPrimitive } from "radix-ui";
import { motion } from "motion/react";

import { cn } from "./lib/utils";
import { useModalPresence } from "./motion";
import { Button } from "./button";
import { XIcon } from "lucide-react";

/**
 * Modal dialog with Fluid Functionalism's motion
 * (https://www.fluidfunctionalism.com/docs/dialog): the overlay fades and the
 * panel springs in from 97% scale, then leaves on a quicker exit while it
 * stays mounted, so closing animates instead of vanishing. Radix still owns
 * focus trapping, scroll lock, dismissal and the ARIA wiring.
 */

/** The dimmed backdrop behind a modal panel. */
const overlayClass = "fixed inset-0 isolate z-50 bg-black/40 dark:bg-black/70";

/**
 * Panel widths (each capped at the viewport less a 1rem gutter per side):
 * `sm` 400px, the default; `md` 480px; `lg` 540px; `xl` 880px, the canvas for
 * composed layouts. A `sm:max-w-*` class still overrides it from 640px up.
 */
const dialogSizes = {
  sm: "max-w-[400px]",
  md: "max-w-[480px]",
  lg: "max-w-[540px]",
  xl: "max-w-[880px]",
} as const;

/** Base panel classes, shared by the dialog and the alert dialog. */
const dialogPanelClass =
  "fixed left-1/2 z-50 flex w-[calc(100%-2rem)] flex-col gap-5 rounded-3xl bg-popover p-6 text-sm text-popover-foreground shadow-2xl shadow-black/10 ring-1 ring-foreground/10 outline-none";

function panelPlacement(position: "center" | "top") {
  return position === "top"
    ? "top-[12dvh] max-h-[calc(88dvh-1rem)]"
    : "top-1/2 max-h-[calc(100dvh-2rem)]";
}

/** Motion targets for the panel: centered panels translate by half their height. */
function panelMotion(open: boolean, position: "center" | "top") {
  const y = position === "top" ? 0 : "-50%";
  return {
    initial: { opacity: 0, scale: 0.97, x: "-50%", y },
    animate: { opacity: open ? 1 : 0, scale: open ? 1 : 0.97, x: "-50%", y },
  };
}

/** Whether the enclosing Dialog is open: the content keeps itself mounted through the exit. */
const DialogOpenContext = React.createContext(false);

function Dialog({
  open: controlledOpen,
  defaultOpen,
  onOpenChange,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Root>) {
  // The Root is always controlled so the content can follow `open` through
  // its exit; an uncontrolled dialog keeps its state here, and the caller's
  // onOpenChange is told alongside.
  const [uncontrolledOpen, setUncontrolledOpen] = React.useState(defaultOpen ?? false);
  const open = controlledOpen ?? uncontrolledOpen;
  const handleOpenChange = (next: boolean) => {
    setUncontrolledOpen(next);
    onOpenChange?.(next);
  };
  return (
    <DialogOpenContext.Provider value={open}>
      <DialogPrimitive.Root
        data-slot="dialog"
        open={open}
        onOpenChange={handleOpenChange}
        {...props}
      />
    </DialogOpenContext.Provider>
  );
}

function DialogTrigger({ ...props }: React.ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />;
}

function DialogPortal({ ...props }: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />;
}

function DialogClose({ ...props }: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />;
}

function DialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        overlayClass,
        "duration-150 data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0",
        className,
      )}
      {...props}
    />
  );
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  size = "sm",
  position = "center",
  /**
   * Whether clicking outside the dialog (overlay / pointer-down) dismisses
   * it. Defaults to `true`, matching platform modal conventions. Set to
   * `false` for dialogs whose input is expensive to re-enter and would be
   * lost to a stray click; the dialog still closes via the X button, the
   * Escape key, or an explicit `DialogClose`.
   */
  dismissOnOutsideClick = true,
  onPointerDownOutside,
  onInteractOutside,
  ...props
}: Omit<React.ComponentProps<typeof DialogPrimitive.Content>, "asChild" | "forceMount"> & {
  showCloseButton?: boolean;
  dismissOnOutsideClick?: boolean;
  /** Panel width: sm 400px, md 480px, lg 540px, xl 880px (viewport-capped). A `sm:max-w-*` class still overrides it from 640px up. @default "sm" */
  size?: keyof typeof dialogSizes;
  /**
   * `center` (default), or `top`: anchored 12dvh from the top so a panel whose
   * height follows its content (a command menu) keeps its top edge still.
   */
  position?: "center" | "top";
}) {
  const open = React.useContext(DialogOpenContext);
  const { mounted, transition, onExitComplete } = useModalPresence(open);

  const handlePointerDownOutside = dismissOnOutsideClick
    ? onPointerDownOutside
    : (e: { preventDefault(): void }) => {
        e.preventDefault();
        onPointerDownOutside?.(e as Parameters<NonNullable<typeof onPointerDownOutside>>[0]);
      };

  const handleInteractOutside = dismissOnOutsideClick
    ? onInteractOutside
    : (e: { preventDefault(): void }) => {
        e.preventDefault();
        onInteractOutside?.(e as Parameters<NonNullable<typeof onInteractOutside>>[0]);
      };

  if (!mounted) return null;

  return (
    <DialogPortal forceMount>
      <DialogPrimitive.Overlay data-slot="dialog-overlay" asChild forceMount>
        <motion.div
          className={overlayClass}
          initial={{ opacity: 0 }}
          animate={{ opacity: open ? 1 : 0 }}
          transition={transition}
        />
      </DialogPrimitive.Overlay>
      <DialogPrimitive.Content
        data-slot="dialog-content"
        asChild
        forceMount
        onPointerDownOutside={handlePointerDownOutside}
        onInteractOutside={handleInteractOutside}
        {...props}
      >
        <motion.div
          className={cn(dialogPanelClass, panelPlacement(position), dialogSizes[size], className)}
          {...panelMotion(open, position)}
          transition={transition}
          onAnimationComplete={onExitComplete}
        >
          {children}
          {showCloseButton && (
            <DialogPrimitive.Close data-slot="dialog-close" asChild>
              <Button variant="ghost" className="absolute top-4 right-4" size="icon-sm">
                <XIcon />
                <span className="sr-only">Close</span>
              </Button>
            </DialogPrimitive.Close>
          )}
        </motion.div>
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}

/**
 * Scrollable middle of a tall dialog: header and footer stay pinned while
 * this region scrolls. The negative margins let the scrollbar sit at the
 * dialog edge and keep focus rings from being clipped.
 */
function DialogBody({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-body"
      className={cn("-mx-6 -my-1 min-h-0 flex-1 overflow-y-auto px-6 py-1", className)}
      {...props}
    />
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-header"
      className={cn("flex shrink-0 flex-col gap-2", className)}
      {...props}
    />
  );
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  showCloseButton?: boolean;
}) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn("flex shrink-0 flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)}
      {...props}
    >
      {children}
      {showCloseButton && (
        <DialogPrimitive.Close asChild>
          <Button variant="outline">Close</Button>
        </DialogPrimitive.Close>
      )}
    </div>
  );
}

function DialogTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn("font-heading text-base leading-none font-medium", className)}
      {...props}
    />
  );
}

function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn(
        "text-sm text-muted-foreground *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-foreground",
        className,
      )}
      {...props}
    />
  );
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
};
