import * as React from "react";
import { useNavigate } from "react-router-dom";
import { LogOut, Search, type LucideIcon } from "lucide-react";
import { useCommandPaletteStore } from "@groot/shell/store/command-palette";
import { useShellConfig } from "@groot/shell/lib/shell-config";
import { THEME_OPTIONS, useTheme } from "@groot/shell/lib/theme";
import { useSignOut } from "@groot/shell/hooks/useSignOut";
import {
  CommandMenu,
  CommandMenuDialog,
  CommandMenuEmpty,
  CommandMenuFooter,
  CommandMenuInput,
  CommandMenuItem,
  CommandMenuList,
  CommandMenuTabs,
  type CommandMenuItemData,
  type CommandMenuTab,
} from "@groot/ui/command-menu";

/** A single selectable entry inside a command-palette group. */
export interface CommandItemEntry {
  label: string;
  icon?: LucideIcon;
  onSelect: () => void;
  variant?: "default" | "destructive";
  /** Stable React key. Defaults to `label`; set when labels may duplicate. */
  id?: string;
  /** What the footer names Enter while the row is highlighted, e.g.
   *  "Go to Settings". Defaults to the label. */
  action?: string;
  /** Muted text after the label. Also matched by the search. */
  description?: string;
  /** Keys shown at the row's trailing edge, e.g. `"mod+shift+n"`. Display
   *  only: bind the combo yourself with `useHotkey` (`@groot/ui/hotkeys`). */
  shortcut?: string;
  /** Extra search terms besides the label and description. */
  keywords?: string[];
  /** List the entry under "Suggestions" while nothing is typed. */
  suggested?: boolean;
}

/** A labeled group of commands in the palette. */
export interface CommandGroupEntry {
  heading: string;
  items: CommandItemEntry[];
  /** Stable React key. Defaults to `heading`; set when headings may duplicate. */
  id?: string;
}

const ALL_TAB = "all";
// Stable default so the item memos below hold when no groups are passed.
const NO_GROUPS: CommandGroupEntry[] = [];

/** Icon button that opens the shared command palette dialog. */
export function CommandPaletteTrigger() {
  const setOpen = useCommandPaletteStore((state) => state.setOpen);
  return (
    <button
      type="button"
      onClick={() => setOpen(true)}
      aria-label="Search (⌘K)"
      className="inline-flex size-8 items-center justify-center rounded-xl text-muted-foreground outline-none hover:bg-accent hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring"
    >
      <Search className="size-4" />
    </button>
  );
}

/**
 * The command palette dialog: render once at the app root. It lists the
 * `ShellConfig.nav` destinations, then the app's own `groups`, then the
 * built-in Theme and Account (log out) commands. ⌘K / Ctrl+K toggles it. An
 * app that builds its own Navigation, Theme and Account groups passes
 * `builtIns={false}` so they are not listed twice.
 */
export function CommandPaletteDialog({
  groups = NO_GROUPS,
  builtIns = true,
}: {
  groups?: CommandGroupEntry[];
  /** List the shell's Navigation, Theme and Account groups (default true). */
  builtIns?: boolean;
}) {
  const open = useCommandPaletteStore((state) => state.open);
  const setOpen = useCommandPaletteStore((state) => state.setOpen);
  const navigate = useNavigate();
  const { nav } = useShellConfig();
  const { setTheme } = useTheme();
  const signOut = useSignOut();
  const [tab, setTab] = React.useState(ALL_TAB);

  // Every time the palette opens it starts on "All".
  React.useEffect(() => {
    if (open) setTab(ALL_TAB);
  }, [open]);

  const resolvedGroups = React.useMemo<CommandGroupEntry[]>(
    () =>
      !builtIns
        ? groups
        : [
            ...(nav.length > 0
              ? [
                  {
                    id: "shell-navigation",
                    heading: "Navigation",
                    items: nav.map((item) => ({
                      id: item.to,
                      label: item.label,
                      icon: item.icon,
                      action: `Go to ${item.label}`,
                      shortcut: item.shortcut,
                      onSelect: () => navigate(item.to),
                    })),
                  },
                ]
              : []),
            ...groups,
            {
              id: "shell-theme",
              heading: "Theme",
              items: THEME_OPTIONS.map(({ value, label, icon }) => ({
                id: `theme-${value}`,
                label: `${label} theme`,
                icon,
                onSelect: () => setTheme(value),
              })),
            },
            {
              id: "shell-account",
              heading: "Account",
              items: [
                {
                  label: "Log out",
                  icon: LogOut,
                  onSelect: signOut,
                  variant: "destructive" as const,
                },
              ],
            },
          ],
    [builtIns, nav, groups, navigate, setTheme, signOut],
  );

  const { items, groupOf, destructive, suggestions, tabs } = React.useMemo(() => {
    const items: CommandMenuItemData[] = [];
    // Item value → its group's tab value. Tabs filter by group id, so two
    // groups that share a heading stay two tabs.
    const groupOf = new Map<string, string>();
    const destructive = new Set<string>();
    const suggestions: string[] = [];
    const tabs: CommandMenuTab[] = [{ value: ALL_TAB, label: "All" }];
    for (const group of resolvedGroups) {
      const groupKey = group.id ?? group.heading;
      tabs.push({ value: groupKey, label: group.heading });
      for (const entry of group.items) {
        const value = `${groupKey}::${entry.id ?? entry.label}`;
        groupOf.set(value, groupKey);
        items.push({
          value,
          label: entry.label,
          action: entry.action,
          description: entry.description,
          icon: entry.icon,
          shortcut: entry.shortcut,
          keywords: [group.heading, ...(entry.keywords ?? [])],
          group: group.heading,
          onSelect: entry.onSelect,
        });
        if (entry.variant === "destructive") destructive.add(value);
        if (entry.suggested) suggestions.push(value);
      }
    }
    return { items, groupOf, destructive, suggestions, tabs };
  }, [resolvedGroups]);

  const visibleItems = React.useMemo(() => {
    if (tab === ALL_TAB) return items;
    return items.filter((item) => groupOf.get(item.value) === tab);
  }, [items, groupOf, tab]);

  return (
    <CommandMenuDialog open={open} onOpenChange={setOpen} shortcut="mod+k">
      <CommandMenu
        items={visibleItems}
        suggestions={tab === ALL_TAB && suggestions.length > 0 ? suggestions : undefined}
      >
        <CommandMenuInput placeholder="Type a command or search…" />
        {tabs.length > 2 && <CommandMenuTabs tabs={tabs} value={tab} onValueChange={setTab} />}
        <CommandMenuList
          renderItem={(item) => (
            <CommandMenuItem
              value={item.value}
              className={destructive.has(item.value) ? "text-destructive" : undefined}
            />
          )}
        >
          <CommandMenuEmpty>No results found.</CommandMenuEmpty>
        </CommandMenuList>
        <CommandMenuFooter />
      </CommandMenu>
    </CommandMenuDialog>
  );
}
