import { useState } from "react";
import { Loader2, Plus } from "lucide-react";
import { Controller, useFormContext } from "react-hook-form";
import { z } from "zod";
import { Button } from "@groot/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@groot/ui/dialog";
import { Form, FormField } from "@groot/ui/form";
import { Input } from "@groot/ui/input";
import { Label } from "@groot/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@groot/ui/select";
import { useAction } from "@groot/shell/lib/feedback";
import { hub, keys } from "@/lib/hub";
import { platformLabel } from "@/lib/format";
import type { AppDetail, Platform } from "@/lib/types";

const schema = z.object({
  platform: z.enum(["android", "ios"]),
  packageName: z
    .string()
    .min(1, "Required")
    .regex(/^[A-Za-z][\w-]*(\.[A-Za-z][\w-]*)+$/, "A reverse-DNS name, e.g. com.example.app"),
});
type Values = z.infer<typeof schema>;

function PlatformSelect({ options }: { options: Platform[] }) {
  const { control } = useFormContext<Values>();
  return (
    <div className="space-y-2">
      <Label htmlFor="platform">Platform</Label>
      <Controller
        control={control}
        name="platform"
        render={({ field }) => (
          <Select value={field.value} onValueChange={field.onChange}>
            <SelectTrigger id="platform">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map((p) => (
                <SelectItem key={p} value={p}>
                  {platformLabel(p)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
    </div>
  );
}

export function AddPlatformDialog({ app }: { app: AppDetail }) {
  const [open, setOpen] = useState(false);
  const taken = new Set(app.platforms.map((p) => p.platform));
  const options = (["android", "ios"] as const).filter((p) => !taken.has(p));
  const add = useAction((v: Values) => hub.addPlatform(app.slug, v), {
    success: (res, v) => ({
      title: `${platformLabel(v.platform)} platform added`,
      description: res.signingKey
        ? "A signing keystore was generated and stored encrypted."
        : undefined,
    }),
    errorTitle: "Could not add the platform",
    fallback: "Unable to add the platform",
    invalidate: [keys.app(app.slug), keys.apps],
    onSuccess: () => setOpen(false),
  });
  if (options.length === 0) return null;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant={app.platforms.length ? "outline" : "default"}>
          <Plus />
          Add platform
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add platform</DialogTitle>
          <DialogDescription>
            Each platform has its own package name, signing key, Play wiring and releases.
          </DialogDescription>
        </DialogHeader>
        <Form
          schema={schema}
          defaultValues={{ platform: options[0], packageName: app.suggestedPackage }}
          onSubmit={async (v) => {
            await add.run(v);
          }}
          className="space-y-4"
        >
          <PlatformSelect options={options} />
          <FormField
            name="packageName"
            label="Package / bundle name"
            description={
              app.suggestedPackage
                ? "Prefilled from the bundle prefix in Settings."
                : "Tip: set a bundle prefix in Settings to prefill this."
            }
          >
            <Input
              placeholder="com.example.app"
              autoComplete="off"
              spellCheck={false}
              className="font-mono"
            />
          </FormField>
          <Button type="submit" size="lg" className="w-full" disabled={add.isPending}>
            {add.isPending ? <Loader2 className="animate-spin" /> : <Plus />}
            {add.isPending ? "Adding…" : "Add platform"}
          </Button>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
