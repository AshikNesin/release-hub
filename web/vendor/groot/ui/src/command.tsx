"use client";

import * as React from "react";
import { Command as CommandPrimitive } from "cmdk";

import { cn } from "./lib/utils";
import { FluidHighlight } from "./motion";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./dialog";
import { SearchIcon, CheckIcon } from "lucide-react";

/**
 * cmdk command menu in Fluid Functionalism's CommandMenu style
 * (https://www.fluidfunctionalism.com/docs/command-menu): a borderless search
 * line over the list, rows that brighten as they become active under one
 * gliding highlight, and an optional footer of keyboard hints.
 *
 * Deprecated: groot's own ⌘K palette is the command menu
 * (`@groot/ui/command-menu`), and nothing in the repo renders this module.
 * Every export is marked `@deprecated` and is removed in the next major.
 */

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function Command({ className, ...props }: React.ComponentProps<typeof CommandPrimitive>) {
  return (
    <CommandPrimitive
      data-slot="command"
      className={cn(
        "flex size-full flex-col overflow-hidden rounded-xl! bg-popover p-1 text-popover-foreground",
        className,
      )}
      {...props}
    />
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandDialog({
  title = "Command Palette",
  description = "Search for a command to run...",
  children,
  className,
  showCloseButton = false,
  ...props
}: React.ComponentProps<typeof Dialog> & {
  title?: string;
  description?: string;
  className?: string;
  showCloseButton?: boolean;
}) {
  return (
    <Dialog {...props}>
      <DialogContent
        size="lg"
        position="top"
        className={cn(
          "flex max-h-[min(440px,76dvh)] flex-col gap-0 overflow-hidden p-0",
          className,
        )}
        showCloseButton={showCloseButton}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandInput({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Input>) {
  return (
    <div
      data-slot="command-input-wrapper"
      className="group/command-input flex h-12 shrink-0 items-center gap-2.5 px-4"
    >
      <SearchIcon
        aria-hidden
        className="size-4 shrink-0 text-muted-foreground group-focus-within/command-input:text-foreground"
      />
      <CommandPrimitive.Input
        data-slot="command-input"
        className={cn(
          "h-full min-w-0 flex-1 bg-transparent text-sm text-foreground outline-hidden placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 pointer-coarse:text-base",
          className,
        )}
        {...props}
      />
    </div>
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandList({
  className,
  children,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.List>) {
  return (
    <CommandPrimitive.List
      data-slot="command-list"
      className={cn(
        "no-scrollbar max-h-72 scroll-py-1 overflow-x-hidden overflow-y-auto border-t border-border/60 outline-none *:[[cmdk-list-sizer]]:relative *:[[cmdk-list-sizer]]:isolate",
        className,
      )}
      {...props}
    >
      <FluidHighlight selector='[cmdk-item][data-selected="true"]' className="bg-muted" />
      {children}
    </CommandPrimitive.List>
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandEmpty({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Empty>) {
  return (
    <CommandPrimitive.Empty
      data-slot="command-empty"
      className={cn("py-6 text-center text-sm", className)}
      {...props}
    />
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandGroup({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Group>) {
  return (
    <CommandPrimitive.Group
      data-slot="command-group"
      className={cn(
        "overflow-hidden p-1 text-foreground **:[[cmdk-group-heading]]:px-2 **:[[cmdk-group-heading]]:py-1.5 **:[[cmdk-group-heading]]:text-xs **:[[cmdk-group-heading]]:font-medium **:[[cmdk-group-heading]]:text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandSeparator({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Separator>) {
  return (
    <CommandPrimitive.Separator
      data-slot="command-separator"
      className={cn("-mx-1 h-px bg-border", className)}
      {...props}
    />
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Item>) {
  return (
    <CommandPrimitive.Item
      data-slot="command-item"
      className={cn(
        "group/command-item relative flex h-9 shrink-0 cursor-pointer items-center gap-2.5 rounded-lg px-2.5 text-sm text-muted-foreground outline-hidden select-none data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 data-selected:text-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 data-selected:*:[svg]:text-foreground",
        className,
      )}
      {...props}
    >
      {children}
      <CheckIcon className="ml-auto opacity-0 group-has-data-[slot=command-shortcut]/command-item:hidden group-data-[checked=true]/command-item:opacity-100" />
    </CommandPrimitive.Item>
  );
}

/**
 * Keyboard hints under the list, Fluid CommandMenu style. Hidden on touch
 * screens, which have no keys to press.
 *
 * @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major.
 */
function CommandFooter({
  className,
  hints = [
    { label: "Navigate", keys: ["↑", "↓"] },
    { label: "Open", keys: ["↵"] },
    { label: "Close", keys: ["esc"] },
  ],
  ...props
}: React.ComponentProps<"div"> & { hints?: { label: string; keys: string[] }[] }) {
  return (
    <div
      data-slot="command-footer"
      className={cn(
        "flex h-10 shrink-0 items-center gap-4 overflow-hidden border-t border-border/60 px-4 text-xs text-muted-foreground pointer-coarse:hidden",
        className,
      )}
      {...props}
    >
      {hints.map((hint) => (
        <span key={hint.label} className="flex shrink-0 items-center gap-1.5">
          <span>{hint.label}</span>
          <kbd className="inline-flex items-center gap-0.5 font-sans">
            {hint.keys.map((key) => (
              <span
                key={key}
                className="flex h-5 min-w-5 items-center justify-center rounded-[5px] bg-muted px-1 text-[11px] leading-none text-muted-foreground"
              >
                {key}
              </span>
            ))}
          </kbd>
        </span>
      ))}
    </div>
  );
}

/** @deprecated Use the command menu (`@groot/ui/command-menu`). Removed in the next major. */
function CommandShortcut({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="command-shortcut"
      className={cn(
        "ml-auto text-xs font-medium text-muted-foreground group-data-selected/command-item:text-foreground",
        className,
      )}
      {...props}
    />
  );
}

export {
  Command,
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
  CommandSeparator,
  CommandFooter,
};
