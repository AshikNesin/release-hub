import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { cn } from "@groot/ui/lib/utils";
import { Circle, PanelLeftClose, PanelLeft } from "lucide-react";
import { isNavActive, type NavDestination, type ShellBrand } from "@groot/shell/lib/shell-config";
import { CommandPaletteTrigger } from "./CommandPalette";

interface SidebarNavProps {
  items: NavDestination[];
  pathname: string;
  /** Mobile drawer open state. */
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Desktop collapsed (icon-rail) state. */
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  footer?: ReactNode;
  brand: Required<ShellBrand>;
}

const ICON_BTN =
  "inline-flex size-8 items-center justify-center rounded-xl text-muted-foreground outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-1 focus-visible:ring-sidebar-ring";

/** Nav row: quiet text, a faint tint for hover and the active page. */
const NAV_ROW =
  "group flex items-center rounded-xl text-sm outline-none focus-visible:ring-1 focus-visible:ring-sidebar-ring";

/**
 * Collapsible left sidebar.
 *
 * Two independent states:
 *  - `open` (mobile only): slides the drawer in/out as an overlay.
 *  - `collapsed` (desktop only): collapses to a minimal icon rail.
 *
 * Mobile and desktop render in separate containers so resizing from an open
 * mobile drawer to desktop never leaks the mobile drawer state into the
 * desktop layout.
 */
export function SidebarNav({
  items,
  pathname,
  open,
  onOpenChange,
  collapsed,
  onCollapsedChange,
  footer,
  brand,
}: SidebarNavProps) {
  const isActive = (item: NavDestination) => isNavActive(pathname, item.to);

  const { icon: BrandMark, to: brandTo, label: brandLabel } = brand;

  const navLinks = items.map((item) => {
    const Icon = item.icon ?? Circle;
    const active = isActive(item);
    return (
      <Link
        key={item.to}
        to={item.to}
        onClick={() => onOpenChange(false)}
        className={cn(
          NAV_ROW,
          "h-9 gap-2 px-2",
          active ? "bg-sidebar-accent text-sidebar-accent-foreground" : "hover:bg-sidebar-accent",
        )}
      >
        <Icon
          className={cn(
            "size-4 shrink-0",
            active ? "text-foreground" : "text-muted-foreground group-hover:text-foreground",
          )}
        />
        <span className="flex-1 whitespace-nowrap">{item.label}</span>
      </Link>
    );
  });

  const iconNavLinks = items.map((item) => {
    const Icon = item.icon ?? Circle;
    const active = isActive(item);
    return (
      <Link
        key={item.to}
        to={item.to}
        onClick={() => onOpenChange(false)}
        title={item.label}
        aria-label={item.label}
        className={cn(
          NAV_ROW,
          "size-9 justify-center",
          active ? "bg-sidebar-accent text-sidebar-accent-foreground" : "hover:bg-sidebar-accent",
        )}
      >
        <Icon
          className={cn(
            "size-4 shrink-0",
            active ? "text-foreground" : "text-muted-foreground group-hover:text-foreground",
          )}
        />
      </Link>
    );
  });

  return (
    <>
      {/* Mobile backdrop. */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/30 lg:hidden"
          onClick={() => onOpenChange(false)}
          aria-hidden
        />
      )}

      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-50 flex flex-col overflow-hidden border-r-[0.5px] border-sidebar-border bg-sidebar text-sidebar-foreground",
          // Mobile: fixed width, slide via translate.
          "w-56 transition-transform duration-300 ease-in-out",
          open ? "translate-x-0" : "-translate-x-full",
          // Desktop: always visible, animate width between rail and full.
          "lg:translate-x-0 lg:transition-[width] lg:duration-300 lg:ease-in-out",
          collapsed ? "lg:w-16" : "lg:w-56",
        )}
      >
        {/* Mobile drawer — always expanded content. Hidden on desktop. */}
        <div className={cn("flex h-full flex-col", open ? "flex" : "hidden", "lg:hidden")}>
          <div className="flex h-14 shrink-0 items-center justify-between gap-1 px-3">
            <Link
              to={brandTo}
              onClick={() => onOpenChange(false)}
              className="flex min-w-0 items-center gap-2 text-sm font-semibold"
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-sidebar-primary text-sidebar-primary-foreground">
                <BrandMark className="size-4" />
              </span>
              <span className="truncate">{brandLabel}</span>
            </Link>

            <div className="flex items-center gap-0.5">
              <CommandPaletteTrigger />
              <button
                type="button"
                onClick={() => onOpenChange(false)}
                aria-label="Close sidebar"
                className={ICON_BTN}
              >
                <PanelLeftClose className="size-4" />
              </button>
            </div>
          </div>

          <nav className="flex-1 space-y-0.5 px-2.5 py-2">{navLinks}</nav>

          {footer && <div className="shrink-0 p-2.5">{footer}</div>}
        </div>

        {/* Desktop collapsed rail — only on desktop when collapsed. */}
        <div className={cn("hidden h-full flex-col", collapsed ? "lg:flex" : "lg:hidden")}>
          <div className="flex shrink-0 flex-col items-center gap-1 px-3 py-3">
            <button
              type="button"
              onClick={() => onCollapsedChange(false)}
              aria-label="Expand sidebar"
              className={ICON_BTN}
            >
              <PanelLeft className="size-4" />
            </button>
            <CommandPaletteTrigger />
          </div>
          <nav className="flex flex-1 flex-col items-center gap-0.5 px-2 py-2">{iconNavLinks}</nav>
          {/* The account menu stays reachable on the rail (Layout's footer
              collapses to the avatar when `collapsed`). */}
          {footer && <div className="flex shrink-0 justify-center p-2.5">{footer}</div>}
        </div>

        {/* Desktop expanded — only on desktop when not collapsed. */}
        <div className={cn("hidden h-full flex-col", !collapsed ? "lg:flex" : "lg:hidden")}>
          <div className="flex h-14 shrink-0 items-center justify-between gap-1 px-3">
            <Link to={brandTo} className="flex min-w-0 items-center gap-2 text-sm font-semibold">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-sidebar-primary text-sidebar-primary-foreground">
                <BrandMark className="size-4" />
              </span>
              <span className="truncate transition-opacity duration-200">{brandLabel}</span>
            </Link>

            <div className="flex items-center gap-0.5">
              <CommandPaletteTrigger />
              <button
                type="button"
                onClick={() => onCollapsedChange(true)}
                aria-label="Collapse sidebar"
                className={ICON_BTN}
              >
                <PanelLeftClose className="size-4" />
              </button>
            </div>
          </div>

          <nav className="flex-1 space-y-0.5 px-2.5 py-2">{navLinks}</nav>

          {footer && <div className="shrink-0 p-2.5">{footer}</div>}
        </div>
      </aside>
    </>
  );
}
