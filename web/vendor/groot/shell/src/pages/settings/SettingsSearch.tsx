import { useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Search } from "lucide-react";
import { useHotkey } from "@groot/ui/hotkeys";
import { Input } from "@groot/ui/input";
import { cn } from "@groot/ui/lib/utils";
import type { SettingsSection } from "./sections";

interface SearchEntry {
  key: string;
  sectionId: string;
  title: string;
  /** "Group" for a section, "Group › Section" for an item. */
  trail: string;
  description?: string;
  /** Lowercased label + keywords, matched by substring. */
  terms: string[];
}

function buildIndex(sections: SettingsSection[]): SearchEntry[] {
  const lower = (values: string[]) => values.map((v) => v.toLowerCase());
  return sections.flatMap((section) => [
    {
      key: section.id,
      sectionId: section.id,
      title: section.label,
      trail: section.group,
      description: section.description,
      terms: lower([section.label, ...(section.keywords ?? [])]),
    },
    ...(section.items ?? []).map((item) => ({
      key: `${section.id}::${item.label}`,
      sectionId: section.id,
      title: item.label,
      trail: `${section.group} › ${section.label}`,
      description: item.description,
      terms: lower([item.label, ...(item.keywords ?? [])]),
    })),
  ]);
}

function Highlight({ text, query }: { text: string; query: string }) {
  const at = text.toLowerCase().indexOf(query);
  if (!query || at < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, at)}
      <span className="font-semibold">{text.slice(at, at + query.length)}</span>
      {text.slice(at + query.length)}
    </>
  );
}

/**
 * The settings sidebar search: matches section labels, keywords and item
 * labels, and lists them in a dropdown. `/` focuses it, ↑/↓ move, Enter
 * opens the highlighted section, Esc clears.
 */
export function SettingsSearch({
  sections,
  onSelect,
}: {
  sections: SettingsSection[];
  onSelect: (sectionId: string) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const listId = useId();
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);

  const index = useMemo(() => buildIndex(sections), [sections]);
  const q = query.trim().toLowerCase();
  const results = q ? index.filter((entry) => entry.terms.some((term) => term.includes(q))) : [];
  const activeIndex = Math.min(active, results.length - 1);
  const showList = open && q.length > 0;

  // A bare key, so it stays out of text fields; like every global hotkey it
  // does nothing while a dialog is open.
  useHotkey("/", () => inputRef.current?.focus());

  const close = () => {
    setQuery("");
    setOpen(false);
    inputRef.current?.blur();
  };

  const pick = (entry: SearchEntry) => {
    onSelect(entry.sectionId);
    close();
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
      return;
    }
    if (results.length === 0) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setOpen(true);
      setActive((activeIndex + 1) % results.length);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setOpen(true);
      setActive((activeIndex - 1 + results.length) % results.length);
    } else if (event.key === "Enter") {
      event.preventDefault();
      const entry = results[activeIndex];
      if (entry) pick(entry);
    }
  };

  const optionId = (i: number) => `${listId}-${i}`;

  return (
    <div className="relative">
      <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        ref={inputRef}
        type="text"
        role="combobox"
        aria-label="Search settings"
        aria-autocomplete="list"
        aria-expanded={showList}
        aria-controls={listId}
        aria-activedescendant={showList && results.length > 0 ? optionId(activeIndex) : undefined}
        autoComplete="off"
        spellCheck={false}
        placeholder="Search settings"
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setActive(0);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={onKeyDown}
        className="pl-9 pr-9"
      />
      {!query && (
        <kbd
          aria-hidden
          className="pointer-events-none absolute right-2 top-1/2 flex h-5 min-w-5 -translate-y-1/2 items-center justify-center rounded-md bg-muted px-1 font-sans text-xs text-muted-foreground"
        >
          /
        </kbd>
      )}
      {showList && (
        <div className="absolute left-0 top-full z-50 mt-1 w-full overflow-hidden rounded-xl bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10 lg:w-80">
          {results.length === 0 ? (
            <p className="px-2.5 py-2 text-sm text-muted-foreground">No matching settings</p>
          ) : (
            <ul id={listId} role="listbox" aria-label="Matching settings">
              {results.map((entry, i) => (
                <li
                  key={entry.key}
                  id={optionId(i)}
                  role="option"
                  aria-selected={i === activeIndex}
                  // Keep focus in the input so the blur doesn't close the list before the click.
                  onMouseDown={(e) => e.preventDefault()}
                  onMouseMove={() => setActive(i)}
                  onClick={() => pick(entry)}
                  className={cn(
                    "cursor-default rounded-lg px-2.5 py-1.5",
                    i === activeIndex && "bg-accent text-accent-foreground",
                  )}
                >
                  <div className="truncate text-sm">
                    <Highlight text={entry.title} query={q} />
                  </div>
                  <div className="truncate text-xs text-muted-foreground">
                    {entry.trail}
                    {entry.description ? ` · ${entry.description}` : ""}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
