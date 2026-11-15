import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Trash2, TriangleAlert } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@groot/ui/alert";
import { Button } from "@groot/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@groot/ui/dialog";
import { Input } from "@groot/ui/input";
import { Label } from "@groot/ui/label";
import { monoClass } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { releaseHub, keys } from "@/lib/releaseHub";
import type { AppDetail } from "@/lib/types";

/**
 * Delete an app and everything it owns: both platforms, every release and
 * artifact, the stored signing keys and Play wiring. Requires typing the
 * slug — the signing key cannot be recovered once the app is deleted.
 */
export function DeleteAppDialog({ app }: { app: AppDetail }) {
  const [open, setOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");
  const navigate = useNavigate();
  const releaseCount = app.platforms.reduce((n, p) => n + p.releases.length, 0);
  const hasSigningKey = app.platforms.some((p) => p.signing.hasKey);

  const del = useAction(() => releaseHub.deleteApp(app.slug), {
    confirm: {
      title: `Delete ${app.slug}?`,
      description:
        "Every release, artifact and stored signing key for all its platforms is deleted from the release hub. This cannot be undone. Anything already published to Google Play stays on Play.",
      confirmLabel: "Delete app",
      destructive: true,
    },
    success: `Deleted ${app.slug}`,
    errorTitle: "Could not delete the app",
    fallback: "Unable to delete the app",
    invalidate: [keys.apps],
    onSuccess: () => navigate("/apps", { replace: true }),
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setConfirmText("");
      }}
    >
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          className="text-muted-foreground hover:text-destructive"
          aria-label={`Delete ${app.slug}`}
        >
          <Trash2 />
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete {app.slug}?</DialogTitle>
          <DialogDescription>
            This permanently deletes the app from the release hub: all {releaseCount}{" "}
            {releaseCount === 1 ? "release" : "releases"} and their artifacts, every platform's
            package, retention and Play wiring.
          </DialogDescription>
        </DialogHeader>
        {hasSigningKey && (
          <Alert variant="destructive">
            <TriangleAlert />
            <AlertTitle>The signing keystore is deleted with it</AlertTitle>
            <AlertDescription>
              Without an offline backup of the keystore, installed devices can never update to a
              build of this app again.
            </AlertDescription>
          </Alert>
        )}
        <div className="space-y-2">
          <Label htmlFor="delete-app-confirm">
            Type <span className={monoClass}>{app.slug}</span> to confirm
          </Label>
          <Input
            id="delete-app-confirm"
            autoComplete="off"
            spellCheck={false}
            placeholder={app.slug}
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)} disabled={del.isPending}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={confirmText.trim() !== app.slug || del.isPending}
            onClick={() => void del.run()}
          >
            {del.isPending ? <Loader2 className="animate-spin" /> : <Trash2 />}
            Delete app
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
