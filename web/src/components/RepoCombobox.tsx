import { useMemo, useState } from "react";
import { Popover } from "radix-ui";
import { Command } from "cmdk";
import { Check, ChevronsUpDown, Lock, Search, Sparkles } from "lucide-react";
import { cn } from "@groot/ui/lib/utils";
import { monoClass } from "@groot/ui/typography";
import { SUGGEST_THRESHOLD, queryScore, rankByRelevance } from "@/lib/repoMatch";
import type { GithubRepository } from "@/lib/types";

/**
 * Searchable repository picker. With no query it lists the repos most
 * likely to publish this app first ("Suggested", ranked by repoMatch), then
 * the rest; typing filters fuzzily (prefix, substring, or letters in order:
 * "gv" finds goldview). Repos that already publish another app are shown
 * but disabled, since one repo maps to one app.
 */
export function RepoCombobox({
  id,
  repos,
  appNames,
  value,
  onChange,
  suggested,
  disabled,
  placeholder,
}: {
  id?: string;
  repos: GithubRepository[];
  /** The app's names (slug, package tails), for relevance. */
  appNames: string[];
  value: string;
  onChange: (fullName: string) => void;
  /** The pre-selected best match, marked in the trigger and the list. */
  suggested: string | null;
  disabled?: boolean;
  placeholder: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const ranked = useMemo(() => rankByRelevance(repos, (r) => r.fullName, appNames), [repos, appNames]);
  const relevance = useMemo(() => new Map(ranked.map((r) => [r.item.fullName, r.score])), [ranked]);

  const { groups, total } = useMemo(() => {
    const free = ranked.filter((r) => !r.item.app);
    const taken = ranked.filter((r) => r.item.app).map((r) => r.item);
    if (query.trim()) {
      const hits = ranked
        .map((r) => ({ item: r.item, q: queryScore(query, r.item.fullName), rel: r.score }))
        .filter((r) => r.q > 0)
        .sort((a, b) => Number(!!a.item.app) - Number(!!b.item.app) || b.q - a.q || b.rel - a.rel)
        .map((r) => r.item);
      return { groups: [{ heading: "", items: hits }], total: hits.length };
    }
    const top = free.filter((r) => r.score >= SUGGEST_THRESHOLD).slice(0, 3).map((r) => r.item);
    const topSet = new Set(top);
    const rest = free
      .map((r) => r.item)
      .filter((r) => !topSet.has(r))
      .sort((a, b) => a.fullName.localeCompare(b.fullName));
    return {
      groups: [
        { heading: "Suggested for this app", items: top },
        { heading: top.length ? "All repositories" : "", items: rest },
        { heading: "Already publishing another app", items: taken },
      ],
      total: repos.length,
    };
  }, [ranked, query, repos.length]);

  return (
    <Popover.Root
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (!o) setQuery("");
      }}
    >
      <Popover.Trigger asChild disabled={disabled}>
        <button
          id={id}
          type="button"
          role="combobox"
          aria-expanded={open}
          className={cn(
            "flex h-9 w-full items-center gap-2 rounded-full border bg-transparent px-4 text-left text-sm outline-none",
            "focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
          )}
        >
          {value ? (
            <>
              <span className={cn(monoClass, "truncate")}>{value}</span>
              {value === suggested ? (
                <span className="inline-flex shrink-0 items-center gap-1 text-xs text-primary">
                  <Sparkles className="size-3" /> Best match
                </span>
              ) : null}
            </>
          ) : (
            <span className="truncate text-muted-foreground">{placeholder}</span>
          )}
          <ChevronsUpDown className="ml-auto size-4 shrink-0 text-muted-foreground" />
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="start"
          sideOffset={6}
          className="z-50 w-(--radix-popover-trigger-width) min-w-72 overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-lg"
        >
          <Command shouldFilter={false} loop>
            <div className="flex items-center gap-2 border-b px-3">
              <Search className="size-4 shrink-0 text-muted-foreground" />
              <Command.Input
                autoFocus
                value={query}
                onValueChange={setQuery}
                placeholder={`Search ${total} repositor${total === 1 ? "y" : "ies"}…`}
                className="h-10 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
              />
            </div>
            <Command.List className="max-h-72 overflow-y-auto p-1">
              <Command.Empty className="px-3 py-6 text-center text-sm text-muted-foreground">
                No repository matches “{query}”.
              </Command.Empty>
              {groups.map((g, gi) =>
                g.items.length ? (
                  <Command.Group
                    key={gi}
                    heading={g.heading || undefined}
                    className="[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:text-muted-foreground"
                  >
                    {g.items.map((r) => (
                      <Command.Item
                        key={r.id}
                        value={r.fullName}
                        disabled={!!r.app}
                        onSelect={() => {
                          onChange(r.fullName);
                          setOpen(false);
                          setQuery("");
                        }}
                        className={cn(
                          "flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm",
                          "data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground",
                          "data-[disabled=true]:cursor-not-allowed data-[disabled=true]:opacity-50",
                        )}
                      >
                        <Check className={cn("size-4 shrink-0", value === r.fullName ? "opacity-100" : "opacity-0")} />
                        <span className={cn(monoClass, "truncate")}>
                          <span className="text-muted-foreground">{r.fullName.split("/")[0]}/</span>
                          {r.fullName.split("/").slice(1).join("/")}
                        </span>
                        {r.private ? <Lock className="size-3 shrink-0 text-muted-foreground" /> : null}
                        <span className="ml-auto shrink-0 text-xs text-muted-foreground">
                          {r.app
                            ? `publishes ${r.app}`
                            : r.fullName === suggested
                              ? "best match"
                              : (relevance.get(r.fullName) ?? 0) >= SUGGEST_THRESHOLD && !query
                                ? "likely"
                                : ""}
                        </span>
                      </Command.Item>
                    ))}
                  </Command.Group>
                ) : null,
              )}
            </Command.List>
          </Command>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
