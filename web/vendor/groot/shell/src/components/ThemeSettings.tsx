import { Card, CardContent } from "@groot/ui/card";
import { Text } from "@groot/ui/typography";
import { cn } from "@groot/ui/lib/utils";
import { THEME_OPTIONS, useTheme } from "@groot/shell/lib/theme";

export function ThemeSettings() {
  const { theme, setTheme } = useTheme();

  return (
    <Card>
      <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <p className="font-medium">Theme</p>
          <Text variant="meta" className="mt-0.5 block">
            Choose how the app looks on this device.
          </Text>
        </div>
        <div
          role="group"
          aria-label="Theme"
          className="inline-flex h-9 w-fit shrink-0 items-center rounded-xl bg-muted p-[3px]"
        >
          {THEME_OPTIONS.map(({ value, label, icon: Icon }) => {
            const selected = theme === value;
            return (
              <button
                key={value}
                type="button"
                onClick={() => setTheme(value)}
                aria-pressed={selected}
                className={cn(
                  "inline-flex h-full items-center gap-1.5 rounded-lg px-2.5 font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                  selected
                    ? "bg-background text-foreground shadow-sm dark:bg-input/30"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                <Icon className="size-3.5" />
                {label}
              </button>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}
