import { Link, useNavigate } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { Text } from "@groot/ui/typography";
import { cn } from "@groot/ui/lib/utils";
import { CommandPaletteTrigger } from "@groot/shell/components/CommandPalette";
import { PageLayout } from "@groot/shell/components/layout/PageLayout";
import { useShellConfig } from "@groot/shell/lib/shell-config";
import { SettingsSearch } from "./SettingsSearch";
import type { SettingsSection } from "./sections";

interface SettingsLayoutProps {
  sections: SettingsSection[];
  active: SettingsSection;
  /** The path the settings page is mounted at, e.g. `/settings` (`""` at the root). */
  basePath: string;
}

function groupSections(sections: SettingsSection[]) {
  const groups = new Map<string, SettingsSection[]>();
  for (const section of sections) {
    groups.set(section.group, [...(groups.get(section.group) ?? []), section]);
  }
  return [...groups];
}

/**
 * Full-page settings chrome: a slim top bar with a way back to the app, a
 * left sidebar (search + grouped sections) and the active section. Below
 * `lg` the sidebar becomes a search box over a scrolling row of sections.
 */
export function SettingsLayout({ sections, active, basePath }: SettingsLayoutProps) {
  const { brand } = useShellConfig();
  const navigate = useNavigate();
  const hrefFor = (id: string) => `${basePath}/${id}`;

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="sticky top-0 z-30 flex h-12 shrink-0 items-center gap-2 border-b border-border bg-background px-4 sm:px-6 lg:px-3">
        <Link
          to={brand.to}
          className="inline-flex h-8 items-center gap-1.5 rounded-xl px-2 text-sm text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring"
        >
          <ArrowLeft className="size-4" />
          Back to {brand.label}
        </Link>
        <div className="ml-auto">
          <CommandPaletteTrigger />
        </div>
      </header>

      <div className="flex flex-1 flex-col lg:flex-row">
        {/* z-20: the search dropdown is wider than the sidebar and must stay above the content. */}
        <aside className="border-b border-border px-4 py-3 sm:px-6 lg:sticky lg:top-12 lg:z-20 lg:flex lg:h-[calc(100dvh-3rem)] lg:w-60 lg:shrink-0 lg:flex-col lg:border-b-0 lg:border-r lg:px-3 lg:py-4">
          <SettingsSearch sections={sections} onSelect={(id) => navigate(hrefFor(id))} />
          <nav
            aria-label="Settings sections"
            className="-mx-1 mt-3 flex gap-1 overflow-x-auto px-1 lg:mx-0 lg:mt-5 lg:min-h-0 lg:flex-1 lg:flex-col lg:gap-5 lg:overflow-y-auto lg:px-0"
          >
            {groupSections(sections).map(([group, groupSections]) => (
              <div key={group} className="contents lg:block">
                <Text variant="meta" className="hidden px-2 pb-1.5 lg:block">
                  {group}
                </Text>
                <ul className="contents lg:flex lg:flex-col lg:gap-0.5">
                  {groupSections.map((section) => {
                    const current = section.id === active.id;
                    const Icon = section.icon;
                    return (
                      <li key={section.id} className="shrink-0">
                        <Link
                          to={hrefFor(section.id)}
                          aria-current={current ? "page" : undefined}
                          className={cn(
                            "group flex h-9 items-center gap-2 whitespace-nowrap rounded-xl px-2.5 text-sm outline-none focus-visible:ring-1 focus-visible:ring-ring lg:px-2",
                            current
                              ? "bg-muted text-foreground"
                              : "text-muted-foreground hover:bg-muted hover:text-foreground",
                          )}
                        >
                          {Icon && (
                            <Icon
                              className={cn(
                                "size-4 shrink-0",
                                current
                                  ? "text-foreground"
                                  : "text-muted-foreground group-hover:text-foreground",
                              )}
                            />
                          )}
                          {section.label}
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </div>
            ))}
          </nav>
        </aside>

        <main className="min-w-0 flex-1 px-4 pb-16 pt-6 sm:px-6 lg:px-10 lg:pt-10">
          {/* Left-aligned, so the title stays put when switching between narrow and wide sections. */}
          <div className={active.wide ? "max-w-5xl" : "max-w-3xl"}>
            <PageLayout title={active.label} description={active.description} maxWidth="full">
              {active.element}
            </PageLayout>
          </div>
        </main>
      </div>
    </div>
  );
}
