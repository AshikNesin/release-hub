import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import type { ReactNode } from "react";
import { useRef, useState } from "react";
import { useAuthStore } from "@groot/shell/store/auth";
import { useCommandPaletteStore } from "@groot/shell/store/command-palette";
import {
  CommandPaletteTrigger,
  CommandPaletteDialog,
  type CommandGroupEntry,
} from "./CommandPalette";
import { SidebarNav } from "./SidebarNav";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@groot/ui/dropdown-menu";
import { Button } from "@groot/ui/button";
import { formatShortcut, useHotkey, useIsMac } from "@groot/ui/hotkeys";
import { cn } from "@groot/ui/lib/utils";
import { isNavActive, useShellConfig } from "@groot/shell/lib/shell-config";
import { isTheme, THEME_OPTIONS, useTheme } from "@groot/shell/lib/theme";
import { useSignOut } from "@groot/shell/hooks/useSignOut";
import {
  MoreHorizontal,
  LogOut,
  PanelLeftClose,
  PanelLeft,
  Monitor,
  SquareTerminal,
} from "lucide-react";

const SIDEBAR_COLLAPSED_KEY = "groot.sidebar.collapsed";

export interface LayoutProps {
  /**
   * Custom header / nav. When set, the shell's sidebar, its offset and the
   * mobile top bar are not rendered, so the header owns the top of the page;
   * the ⌘K palette and the routed `<Outlet/>` stay.
   */
  header?: ReactNode;
  /** Extra ⌘K groups, listed after the `ShellConfig.nav` destinations and before Theme and Account. */
  commandGroups?: CommandGroupEntry[];
  /** Also list the palette's built-in Navigation, Theme and Account groups (default true). */
  commandBuiltIns?: boolean;
  /**
   * Whether `<main>` gets the shell's default horizontal + vertical padding.
   * Defaults to `true`. Set `false` when pages own their own padding (e.g. via
   * `PageContainer`).
   */
  padded?: boolean;
  /** Extra className merged onto `<main>` (e.g. `flex flex-col` for full-height pages). */
  mainClassName?: string;
  /** Extra className merged onto the outermost wrapper div. */
  className?: string;
}

/**
 * App shell: a dub.sh-style collapsible sidebar on the left, routed `<Outlet/>`
 * on the right. The sidebar collapses (desktop) into an icon rail with a
 * 300ms width animation; on mobile it slides in as an overlay drawer.
 */
export function Layout({
  header,
  commandGroups,
  commandBuiltIns,
  padded = true,
  mainClassName,
  className,
}: LayoutProps) {
  const user = useAuthStore((state) => state.user);
  const navigate = useNavigate();
  const location = useLocation();
  const { brand, nav } = useShellConfig();
  const signOut = useSignOut();
  const setPaletteOpen = useCommandPaletteStore((state) => state.setOpen);
  const openingPalette = useRef(false);

  // When a custom header is supplied the default sidebar (and its offset /
  // mobile bar) is disabled so the header owns the full top of the page.
  // Uses a nullish check so both `undefined` (omitted) and `null` (explicitly
  // cleared) render the default sidebar — matching the `{header ?? ...}` JSX.
  const useDefaultSidebar = header == null;

  // Mobile drawer state.
  const [sidebarOpen, setSidebarOpen] = useState(false);
  // Desktop collapse state, persisted across reloads.
  // Lazy initializer reads localStorage synchronously to prevent hydration flash.
  const [collapsed, setCollapsed] = useState(() => {
    return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "true";
  });

  const handleCollapsedChange = (next: boolean) => {
    setCollapsed(next);
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(next));
  };

  const menuNav = nav.filter((item) => !item.primary);

  const fullPage = nav.some((item) => item.fullPage && isNavActive(location.pathname, item.to));
  const showSidebar = useDefaultSidebar && !fullPage;

  const userMenuFooter = (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          title={collapsed ? (user?.name ?? user?.email ?? "Account") : undefined}
          className={cn(
            "flex h-10 w-full items-center gap-2 rounded-xl px-1.5 text-left text-sm outline-none hover:bg-sidebar-accent focus-visible:ring-1 focus-visible:ring-sidebar-ring",
            collapsed && "lg:size-10 lg:justify-center lg:px-0",
          )}
        >
          <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
            {user?.name?.[0]?.toUpperCase() ?? user?.email?.[0]?.toUpperCase() ?? "?"}
          </span>
          <span
            className={cn(
              "flex min-w-0 flex-1 items-center leading-tight",
              collapsed && "lg:hidden",
            )}
          >
            <span className="truncate">
              {user?.name?.trim() || user?.email?.toLowerCase() || "Account"}
            </span>
          </span>
          <MoreHorizontal
            className={cn("size-4 shrink-0 text-muted-foreground", collapsed && "lg:hidden")}
          />
        </button>
      </DropdownMenuTrigger>
      {/* Opens upward, so the last group sits next to the trigger and Log out
          is the row farthest from it. */}
      <DropdownMenuContent
        align="start"
        side="top"
        sideOffset={8}
        className="w-64"
        onCloseAutoFocus={(event) => {
          // The palette takes focus; handing it back to the trigger would
          // pull it out of the dialog.
          if (openingPalette.current) {
            openingPalette.current = false;
            event.preventDefault();
          }
        }}
      >
        <DropdownMenuItem onSelect={signOut} className={MENU_ROW}>
          <LogOut />
          <span>Log out</span>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onSelect={() => {
            openingPalette.current = true;
            setPaletteOpen(true);
          }}
          className={MENU_ROW}
        >
          <SquareTerminal />
          <span>Command palette</span>
          <ShortcutHint shortcut="mod+k" />
        </DropdownMenuItem>
        {menuNav.length > 0 && <DropdownMenuSeparator />}
        {menuNav.map(({ to, label, icon: Icon, shortcut }) => (
          <DropdownMenuItem key={to} onSelect={() => navigate(to)} className={MENU_ROW}>
            {Icon ? <Icon /> : null}
            <span>{label}</span>
            {shortcut ? <ShortcutHint shortcut={shortcut} /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
  return (
    <div className={cn("min-h-dvh bg-background text-foreground", className)}>
      {/* Single shared command palette dialog — mounted once. */}
      <CommandPaletteDialog groups={commandGroups} builtIns={commandBuiltIns} />
      {nav.map((item) =>
        item.shortcut ? <NavShortcut key={item.to} shortcut={item.shortcut} to={item.to} /> : null,
      )}

      {fullPage
        ? null
        : (header ?? (
            <SidebarNav
              items={nav.filter((item) => item.primary)}
              brand={brand}
              pathname={location.pathname}
              open={sidebarOpen}
              onOpenChange={setSidebarOpen}
              collapsed={collapsed}
              onCollapsedChange={handleCollapsedChange}
              footer={userMenuFooter}
            />
          ))}

      {/* Main column: sidebar offset on desktop (animated with the sidebar),
          full-width on mobile. Only applies when the default sidebar renders. */}
      <div
        className={cn(
          "transition-[padding] duration-300 ease-in-out",
          showSidebar && (collapsed ? "lg:pl-16" : "lg:pl-56"),
        )}
      >
        {/* Top bar — mobile (sidebar toggle + brand + search icon). */}
        {showSidebar && (
          <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-border bg-background px-4 lg:hidden">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setSidebarOpen((o) => !o)}
              aria-label="Toggle sidebar"
            >
              {sidebarOpen ? (
                <PanelLeftClose className="size-4" />
              ) : (
                <PanelLeft className="size-4" />
              )}
            </Button>
            <Link to={brand.to} className="text-sm font-semibold">
              {brand.label}
            </Link>
            <div className="ml-auto">
              <CommandPaletteTrigger />
            </div>
          </header>
        )}

        <main
          className={cn(
            // dvh, not vh: on mobile the URL bar eats into 100vh and the
            // min-height would overflow the visible area (100dvh tracks it).
            // The 3.5rem offset only exists where the mobile top bar renders.
            "w-full",
            showSidebar && "min-h-[calc(100dvh-3.5rem)]",
            padded && !fullPage && "px-4 pb-10 pt-6 sm:px-6 lg:px-8",
            mainClassName,
          )}
        >
          <Outlet />
        </main>
      </div>
    </div>
  );
}

/** Account-menu row: Amp-like spacing between the icon and the label. */
const MENU_ROW = "gap-2.5";

/** Plain muted key glyphs ("⌘ ,"), not key caps, at a menu row's trailing edge. */
function ShortcutHint({ shortcut }: { shortcut: string }) {
  const mac = useIsMac();
  return (
    <DropdownMenuShortcut aria-hidden className="font-normal">
      {formatShortcut(shortcut, mac).join(" ")}
    </DropdownMenuShortcut>
  );
}

/**
 * Binds a `NavDestination.shortcut` to navigating there: a global hotkey, so
 * it does not fire while a dialog (the ⌘K palette included) is open, and a
 * combo without a modifier does not fire in a text field.
 */
function NavShortcut({ shortcut, to }: { shortcut: string; to: string }) {
  const navigate = useNavigate();
  useHotkey(shortcut, () => navigate(to));
  return null;
}

/**
 * A Theme submenu for a `DropdownMenu`.
 *
 * @deprecated Nothing renders it: Theme lives in Settings › Appearance and
 * the ⌘K Theme group. Removed in the next major.
 */
export function ThemeMenu() {
  const { theme, setTheme } = useTheme();

  return (
    <>
      <DropdownMenuSub>
        <DropdownMenuSubTrigger>
          <Monitor className="size-4" />
          <span>Theme</span>
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent>
          <DropdownMenuRadioGroup
            value={theme}
            onValueChange={(value) => {
              if (isTheme(value)) setTheme(value);
            }}
          >
            {THEME_OPTIONS.map(({ value, label, icon: Icon }) => (
              <DropdownMenuRadioItem key={value} value={value}>
                <Icon className="size-4" />
                <span>{label}</span>
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuSubContent>
      </DropdownMenuSub>
      <DropdownMenuSeparator />
    </>
  );
}
