"use client";

import * as React from "react";
import { AlertDialog as AlertDialogPrimitive } from "radix-ui";
import { motion } from "motion/react";

import { cn } from "../lib/utils";
import { useModalPresence } from "../motion";

// Same backdrop, panel and motion as dialog.tsx, kept local like the rest of
// this port's styling.
const overlayClass = "fixed inset-0 isolate z-50 bg-black/40 dark:bg-black/70";
const panelClass =
  "fixed top-1/2 left-1/2 z-50 flex max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-[400px] flex-col gap-5 rounded-3xl bg-popover p-6 text-sm text-popover-foreground shadow-2xl shadow-black/10 ring-1 ring-foreground/10 outline-none";
const panelInitial = { opacity: 0, scale: 0.97, x: "-50%", y: "-50%" };
const panelAnimate = (open: boolean) => ({
  opacity: open ? 1 : 0,
  scale: open ? 1 : 0.97,
  x: "-50%",
  y: "-50%",
});
import { Button, type ButtonProps } from "../button";

/**
 * Radix AlertDialog-based confirm primitive — a faithful port of the shadcn
 * `alert-dialog` component, restyled to match this package's `dialog.tsx`
 * (rounded-3xl, ring, viewport-capped, plain footer, and the same spring
 * enter/exit motion). Lives under `@groot/ui/primitives` so
 * callers know it is a composed/Radix-backed primitive rather than a bespoke
 * widget. The declarative {@link AlertDialog} parts are the building blocks
 * for the imperative {@link ConfirmProvider} (see ./confirm).
 */

/** Whether the enclosing AlertDialog is open: the content keeps itself mounted through the exit. */
const AlertDialogOpenContext = React.createContext(false);

function AlertDialog({
  open: controlledOpen,
  defaultOpen,
  onOpenChange,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Root>) {
  const [uncontrolledOpen, setUncontrolledOpen] = React.useState(defaultOpen ?? false);
  const open = controlledOpen ?? uncontrolledOpen;
  const handleOpenChange = (next: boolean) => {
    setUncontrolledOpen(next);
    onOpenChange?.(next);
  };
  return (
    <AlertDialogOpenContext.Provider value={open}>
      <AlertDialogPrimitive.Root
        data-slot="alert-dialog"
        open={open}
        onOpenChange={handleOpenChange}
        {...props}
      />
    </AlertDialogOpenContext.Provider>
  );
}

function AlertDialogTrigger({
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Trigger>) {
  return <AlertDialogPrimitive.Trigger data-slot="alert-dialog-trigger" {...props} />;
}

function AlertDialogPortal({ ...props }: React.ComponentProps<typeof AlertDialogPrimitive.Portal>) {
  return <AlertDialogPrimitive.Portal data-slot="alert-dialog-portal" {...props} />;
}

function AlertDialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Overlay>) {
  return (
    <AlertDialogPrimitive.Overlay
      data-slot="alert-dialog-overlay"
      className={cn(
        overlayClass,
        "duration-150 data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0",
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogContent({
  className,
  children,
  ...props
}: Omit<React.ComponentProps<typeof AlertDialogPrimitive.Content>, "asChild" | "forceMount">) {
  const open = React.useContext(AlertDialogOpenContext);
  const { mounted, transition, onExitComplete } = useModalPresence(open);
  if (!mounted) return null;
  return (
    <AlertDialogPortal forceMount>
      <AlertDialogPrimitive.Overlay data-slot="alert-dialog-overlay" asChild forceMount>
        <motion.div
          className={overlayClass}
          initial={{ opacity: 0 }}
          animate={{ opacity: open ? 1 : 0 }}
          transition={transition}
        />
      </AlertDialogPrimitive.Overlay>
      <AlertDialogPrimitive.Content data-slot="alert-dialog-content" asChild forceMount {...props}>
        <motion.div
          className={cn(panelClass, className)}
          initial={panelInitial}
          animate={panelAnimate(open)}
          transition={transition}
          onAnimationComplete={onExitComplete}
        >
          {children}
        </motion.div>
      </AlertDialogPrimitive.Content>
    </AlertDialogPortal>
  );
}

function AlertDialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-header"
      className={cn("flex shrink-0 flex-col gap-2", className)}
      {...props}
    />
  );
}

function AlertDialogFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-footer"
      className={cn("flex shrink-0 flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)}
      {...props}
    />
  );
}

function AlertDialogTitle({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Title>) {
  return (
    <AlertDialogPrimitive.Title
      data-slot="alert-dialog-title"
      className={cn("font-heading text-base leading-none font-medium", className)}
      {...props}
    />
  );
}

function AlertDialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Description>) {
  return (
    <AlertDialogPrimitive.Description
      data-slot="alert-dialog-description"
      className={cn(
        "text-sm text-muted-foreground *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-foreground",
        className,
      )}
      {...props}
    />
  );
}

// AlertDialogAction/Cancel are buttons with an AlertDialog role + auto-close.
// shadcn renders them via buttonVariants + Radix's Close/Action slot. We render
// the real Button (variant + size kept in sync with the rest of the app) inside
// Radix's slot, so they accept the full ButtonProps surface (variant, size…).
type AlertDialogActionProps = React.ComponentProps<typeof AlertDialogPrimitive.Action> &
  ButtonProps;

function AlertDialogAction({ className, ...props }: AlertDialogActionProps) {
  return (
    <AlertDialogPrimitive.Action asChild>
      <Button className={cn(className)} {...props} />
    </AlertDialogPrimitive.Action>
  );
}

type AlertDialogCancelProps = React.ComponentProps<typeof AlertDialogPrimitive.Cancel> &
  ButtonProps;

function AlertDialogCancel({ className, ...props }: AlertDialogCancelProps) {
  return (
    <AlertDialogPrimitive.Cancel asChild>
      <Button variant="outline" className={cn(className)} {...props} />
    </AlertDialogPrimitive.Cancel>
  );
}

export {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogTitle,
  AlertDialogTrigger,
};
