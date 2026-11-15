import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Plus } from "lucide-react";
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
import { useAction } from "@groot/shell/lib/feedback";
import { releaseHub, keys } from "@/lib/releaseHub";

// Mirrors slugRe in srv/api_releases.go.
const schema = z.object({
  slug: z.string().regex(/^[a-z0-9][a-z0-9-]{0,63}$/, "Lowercase letters, digits and dashes"),
});

export function RegisterAppDialog({ trigger }: { trigger?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const create = useAction((slug: string) => releaseHub.createApp(slug), {
    success: (_, slug) => ({
      title: "App registered",
      description: `Add the ${slug} platform next.`,
    }),
    errorTitle: "Could not register the app",
    fallback: "Unable to register the app",
    invalidate: [keys.apps],
    onSuccess: (_, slug) => {
      setOpen(false);
      navigate(`/apps/${slug}?tab=configuration`);
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {trigger ?? (
          <Button>
            <Plus />
            Register app
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Register app</DialogTitle>
          <DialogDescription>
            Just a slug, one per product. Android and iOS platforms are added from the app's page.
          </DialogDescription>
        </DialogHeader>
        <Form
          schema={schema}
          defaultValues={{ slug: "" }}
          onSubmit={async ({ slug }) => {
            await create.run(slug);
          }}
          className="space-y-4"
        >
          <FormField
            name="slug"
            label="Slug"
            description="Used in URLs, e.g. /api/apps/tinyfirewall/manifest."
          >
            <Input placeholder="tinyfirewall" autoFocus autoComplete="off" spellCheck={false} />
          </FormField>
          <Button type="submit" size="lg" className="w-full" disabled={create.isPending}>
            {create.isPending ? <Loader2 className="animate-spin" /> : <Plus />}
            {create.isPending ? "Registering…" : "Register"}
          </Button>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
