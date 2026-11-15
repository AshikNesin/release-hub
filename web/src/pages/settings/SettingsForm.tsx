import { useEffect, useState, type ReactNode } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@groot/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@groot/ui/card";
import { Input } from "@groot/ui/input";
import { Label } from "@groot/ui/label";
import { SkeletonCard } from "@groot/ui/loading-skeleton";
import { Textarea } from "@groot/ui/textarea";
import { Text } from "@groot/ui/typography";
import { cn } from "@groot/ui/lib/utils";
import { useAction } from "@groot/shell/lib/feedback";
import { releaseHub, keys, useSettings } from "@/lib/releaseHub";
import type { ReleaseHubSettings, SettingsPatch } from "@/lib/types";

export interface SettingField {
  key: keyof SettingsPatch;
  label: string;
  /** Read the current value out of the settings payload. */
  read: (s: ReleaseHubSettings) => string;
  placeholder?: string;
  hint?: ReactNode;
  multiline?: boolean;
  inputMode?: "numeric" | "text" | "email";
  mono?: boolean;
}

/**
 * A card of release hub settings with one Save: only this card's keys are sent
 * (PATCH /api/v1/settings saves just the keys present).
 */
export function SettingsForm({
  title,
  description,
  fields,
  columns = 1,
  children,
}: {
  title: string;
  description?: ReactNode;
  fields: SettingField[];
  columns?: 1 | 2;
  children?: ReactNode;
}) {
  const { data: settings, isLoading } = useSettings();
  const initial = (s: ReleaseHubSettings) => Object.fromEntries(fields.map((f) => [f.key, f.read(s)]));
  const [values, setValues] = useState<Record<string, string>>({});
  useEffect(() => {
    if (settings) setValues(initial(settings));
    // Reset the draft whenever the saved settings change.
  }, [settings]);

  const save = useAction((patch: SettingsPatch) => releaseHub.patchSettings(patch), {
    success: `${title} saved`,
    errorTitle: `Could not save ${title.toLowerCase()}`,
    fallback: "Unable to save the settings",
    invalidate: [keys.settings, keys.apps],
  });

  if (isLoading || !settings) return <SkeletonCard description />;
  const saved = initial(settings);
  const dirty = fields.some((f) => (values[f.key] ?? "") !== saved[f.key]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void save.run(Object.fromEntries(fields.map((f) => [f.key, values[f.key] ?? ""])));
        }}
      >
        <CardContent className={cn("grid gap-4", columns === 2 && "sm:grid-cols-2")}>
          {fields.map((f) => {
            const id = `setting-${f.key}`;
            const common = {
              id,
              value: values[f.key] ?? "",
              placeholder: f.placeholder,
              className: cn(f.mono && "font-mono text-xs"),
              spellCheck: false,
            };
            return (
              <div
                key={f.key}
                className={cn("space-y-2", f.multiline && columns === 2 && "sm:col-span-2")}
              >
                <Label htmlFor={id}>{f.label}</Label>
                {f.multiline ? (
                  <Textarea
                    {...common}
                    rows={3}
                    onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}
                  />
                ) : (
                  <Input
                    {...common}
                    inputMode={f.inputMode}
                    onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}
                  />
                )}
                {f.hint && (
                  <Text variant="meta" className="block">
                    {f.hint}
                  </Text>
                )}
              </div>
            );
          })}
          {children}
        </CardContent>
        <CardFooter className="mt-4 justify-end gap-2 border-t py-3">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={!dirty || save.isPending}
            onClick={() => setValues(saved)}
          >
            Reset
          </Button>
          <Button type="submit" size="sm" disabled={!dirty || save.isPending}>
            {save.isPending && <Loader2 className="animate-spin" />}
            Save
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}
