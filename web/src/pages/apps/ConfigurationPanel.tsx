import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  CircleCheck,
  CircleX,
  ExternalLink,
  KeyRound,
  ListTree,
  Loader2,
  PlugZap,
  Power,
  Store,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@groot/ui/alert";
import { Badge } from "@groot/ui/badge";
import { Button } from "@groot/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@groot/ui/card";
import { DataField, DataFields } from "@groot/ui/display";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@groot/ui/dialog";
import { Input } from "@groot/ui/input";
import { Label } from "@groot/ui/label";
import { LoadingSpinner } from "@groot/ui/loading-spinner";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@groot/ui/select";
import { StatusBadge } from "@groot/ui/status-badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@groot/ui/table";
import { monoClass, Text } from "@groot/ui/typography";
import { describeError, useAction } from "@groot/shell/lib/feedback";
import { CHANNELS, ChannelBadge } from "@/components/badges";
import { CopyField } from "@/components/copy";
import { releaseHub, keys } from "@/lib/releaseHub";
import type { AppDetail, PlatformDetail, Preflight } from "@/lib/types";
import { uploadCommand } from "./ReleasesPanel";

export function ConfigurationPanel({
  app,
  platform,
}: {
  app: AppDetail;
  platform: PlatformDetail;
}) {
  return (
    <>
      <IntegrationCard platform={platform} />
      <div className="grid gap-6 lg:grid-cols-2">
        <SigningCard slug={app.slug} platform={platform} />
        <RetentionCard slug={app.slug} platform={platform} />
      </div>
      {platform.platform === "android" && <PlayCard app={app} platform={platform} />}
    </>
  );
}

function IntegrationCard({ platform }: { platform: PlatformDetail }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Integration</CardTitle>
        <CardDescription>What CI and installed apps talk to.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <DataFields>
          <DataField label="Package" valueClassName={monoClass}>
            {platform.packageName}
          </DataField>
        </DataFields>
        <div className="space-y-2">
          <Label>Update manifest (public, for in-app updaters)</Label>
          <CopyField value={`${platform.manifestUrl}?channel=direct`} what="Manifest URL" />
        </div>
        <div className="space-y-2">
          <Label>Upload from CI</Label>
          <CopyField value={uploadCommand(platform)} what="Upload command" />
          {platform.platform === "android" && (
            <Text variant="meta" className="block">
              Send an <code className={monoClass}>.aab</code> with{" "}
              <code className={monoClass}>channel=internal</code>,{" "}
              <code className={monoClass}>alpha</code>, <code className={monoClass}>beta</code> or{" "}
              <code className={monoClass}>public</code> to also publish to Google Play.
            </Text>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function SigningCard({ slug, platform }: { slug: string; platform: PlatformDetail }) {
  const { signing } = platform;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Signing key</CardTitle>
        <CardDescription>Encrypted at rest; CI fetches it with an API token.</CardDescription>
        <CardAction>
          {signing.hasKey ? (
            <StatusBadge status="success" label="Stored" />
          ) : (
            <StatusBadge status="warning" label="None" />
          )}
        </CardAction>
      </CardHeader>
      <CardContent>
        {signing.hasKey ? (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Keystore SHA-256</Label>
              <CopyField value={signing.sha256} what="Fingerprint" />
            </div>
            {signing.alias && (
              <DataFields>
                <DataField label="Key alias" valueClassName={monoClass}>
                  {signing.alias}
                </DataField>
              </DataFields>
            )}
          </div>
        ) : (
          <Text variant="meta" className="block">
            No keystore stored. Upload one with{" "}
            <code className={monoClass}>
              POST /api/apps/{slug}/{platform.platform}/signing
            </code>
            .
          </Text>
        )}
      </CardContent>
    </Card>
  );
}

function RetentionCard({ slug, platform }: { slug: string; platform: PlatformDetail }) {
  const { retention } = platform;
  const [value, setValue] = useState(retention.override === null ? "" : String(retention.override));
  const save = useAction((keep: number | null) => releaseHub.setRetention(slug, platform.platform, keep), {
    success: (res) => ({ title: "Retention saved", description: `Keeping ${res.label}.` }),
    errorTitle: "Could not save retention",
    fallback: "Unable to save retention",
    invalidate: [keys.app(slug)],
  });
  const dirty = value !== (retention.override === null ? "" : String(retention.override));

  return (
    <Card>
      <CardHeader>
        <CardTitle>Release retention</CardTitle>
        <CardDescription>
          After a direct upload, older direct releases beyond the keep count are deleted. Play
          channels are never pruned.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void save.run(value.trim() === "" ? null : Number(value));
          }}
        >
          <div className="space-y-2">
            <Label htmlFor={`keep-${platform.platform}`}>Keep newest</Label>
            <Input
              id={`keep-${platform.platform}`}
              type="number"
              min={0}
              inputMode="numeric"
              className="w-36"
              placeholder="Release hub setting"
              value={value}
              onChange={(e) => setValue(e.target.value)}
            />
          </div>
          <Button type="submit" variant="outline" disabled={!dirty || save.isPending}>
            {save.isPending && <Loader2 className="animate-spin" />}
            Save
          </Button>
        </form>
        <Text variant="meta" className="mt-3 block">
          Now keeping {retention.label}
          {retention.override === null ? " (from the release hub setting)" : ""}. Blank inherits Settings →
          General; 0 keeps everything.
        </Text>
      </CardContent>
    </Card>
  );
}

function PlayCard({ app, platform }: { app: AppDetail; platform: PlatformDetail }) {
  const { play } = platform;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Store className="size-4 text-muted-foreground" />
          Google Play
        </CardTitle>
        <CardDescription>
          {play.enabled
            ? "Uploads of .aab files on Play channels are pushed to the matching track."
            : "Push .aab uploads to Play tracks with a shared service account."}
        </CardDescription>
        <CardAction>
          {play.enabled ? (
            <StatusBadge status="success" label="Publishing on" />
          ) : (
            <StatusBadge status="pending" label="Off" />
          )}
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-5">
        {play.enabled ? (
          <PlayEnabled app={app} platform={platform} />
        ) : (
          <PlayEnable app={app} platform={platform} />
        )}
        <div>
          <Text variant="meta" className="mb-2 block">
            Channel → Play track
          </Text>
          <div className="flex flex-wrap gap-x-4 gap-y-2">
            {(["internal", "alpha", "beta", "public"] as const).map((c) => (
              <span key={c} className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <ChannelBadge channel={c} />
                {CHANNELS[c].where.replace("Play ", "")}
              </span>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function PlayEnabled({ app, platform }: { app: AppDetail; platform: PlatformDetail }) {
  const [check, setCheck] = useState<Preflight | "running" | null>(null);
  const disable = useAction(() => releaseHub.setPlay(app.slug, platform.platform, { enable: false }), {
    confirm: {
      title: "Disable Google Play publishing?",
      description:
        "Uploads keep landing on the release hub but stop being pushed to Play. The shared service account stays in Settings.",
      confirmLabel: "Disable",
      destructive: true,
    },
    success: "Google Play publishing disabled",
    errorTitle: "Could not disable Play",
    fallback: "Unable to disable Play publishing",
    invalidate: [keys.app(app.slug), keys.apps, keys.playAccounts],
  });

  const runCheck = async () => {
    setCheck("running");
    try {
      setCheck(await releaseHub.preflight(app.slug, platform.platform));
    } catch (error) {
      setCheck({ ok: false, detail: describeError(error, "The connection check failed.") });
    }
  };

  return (
    <div className="space-y-4">
      <DataFields>
        <DataField label="Service account" valueClassName={monoClass}>
          {platform.play.email || `#${platform.play.accountId}`}
        </DataField>
      </DataFields>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => void runCheck()}
          disabled={check === "running"}
        >
          {check === "running" ? <Loader2 className="animate-spin" /> : <PlugZap />}
          Test connection
        </Button>
        <TracksDialog slug={app.slug} platform={platform} />
        <Button variant="outline" size="sm" asChild>
          <a href="https://play.google.com/console/u/0/developers" target="_blank" rel="noreferrer">
            Play Console
            <ExternalLink />
          </a>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => void disable.run()}
          disabled={disable.isPending}
          className="text-muted-foreground hover:text-destructive"
        >
          <Power />
          Disable
        </Button>
      </div>
      {check && check !== "running" && (
        <Alert variant={check.ok ? "default" : "destructive"}>
          {check.ok ? <CircleCheck className="text-success" /> : <CircleX />}
          <AlertTitle>{check.ok ? "Connected" : "Connection check failed"}</AlertTitle>
          <AlertDescription>{check.detail}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}

function TracksDialog({ slug, platform }: { slug: string; platform: PlatformDetail }) {
  const [open, setOpen] = useState(false);
  const { data, isLoading, error } = useQuery({
    queryKey: keys.tracks(slug, platform.platform),
    queryFn: () => releaseHub.tracks(slug, platform.platform),
    enabled: open,
    staleTime: 30_000,
    retry: false,
  });
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <ListTree />
          Play tracks
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Play tracks</DialogTitle>
          <DialogDescription>
            What is live on each track of <span className={monoClass}>{platform.packageName}</span>,
            read from Google Play.
          </DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <div className="flex justify-center py-10">
            <LoadingSpinner size="md" />
          </div>
        ) : error ? (
          <Alert variant="destructive">
            <CircleX />
            <AlertTitle>Could not read the tracks</AlertTitle>
            <AlertDescription>
              {describeError(error, "Google Play did not answer.")}
            </AlertDescription>
          </Alert>
        ) : (
          <div className="overflow-hidden rounded-2xl ring-1 ring-foreground/10">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-4">Track</TableHead>
                  <TableHead>Release hub channel</TableHead>
                  <TableHead className="pr-4">Releases</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.map((t) => (
                  <TableRow key={t.track}>
                    <TableCell className={`pl-4 ${monoClass}`}>{t.track}</TableCell>
                    <TableCell>
                      <ChannelBadge channel={t.channel} />
                    </TableCell>
                    <TableCell className="pr-4 whitespace-normal">
                      {t.releases?.length ? (
                        <div className="flex flex-wrap gap-1">
                          {t.releases.map((r) => (
                            <Badge key={r} variant="secondary">
                              {r}
                            </Badge>
                          ))}
                        </div>
                      ) : (
                        <span className="text-muted-foreground">None</span>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

const NEW_KEY = "new";

function PlayEnable({ app, platform }: { app: AppDetail; platform: PlatformDetail }) {
  const [account, setAccount] = useState(
    app.playAccounts[0] ? String(app.playAccounts[0].id) : NEW_KEY,
  );
  const [file, setFile] = useState<File | null>(null);
  const enable = useAction(
    () =>
      releaseHub.setPlay(
        app.slug,
        platform.platform,
        account === NEW_KEY ? { file: file as File } : { account: Number(account) },
      ),
    {
      success: (res) => ({
        title: "Google Play publishing enabled",
        description: res.serviceAccount ? `Using ${res.serviceAccount}.` : undefined,
      }),
      errorTitle: "Could not enable Play",
      fallback: "Unable to enable Play publishing",
      invalidate: [keys.app(app.slug), keys.apps, keys.playAccounts],
    },
  );
  const ready = account !== NEW_KEY || file !== null;

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) void enable.run();
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor={`acct-${platform.platform}`}>Service account</Label>
          <Select value={account} onValueChange={setAccount}>
            <SelectTrigger id={`acct-${platform.platform}`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {app.playAccounts.map((a) => (
                <SelectItem key={a.id} value={String(a.id)}>
                  <KeyRound className="text-muted-foreground" />
                  {a.email}
                </SelectItem>
              ))}
              <SelectItem value={NEW_KEY}>Upload a new JSON key…</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {account === NEW_KEY && (
          <div className="space-y-2">
            <Label htmlFor={`key-${platform.platform}`}>Service-account JSON key</Label>
            <Input
              id={`key-${platform.platform}`}
              type="file"
              accept=".json,application/json"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </div>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" disabled={!ready || enable.isPending}>
          {enable.isPending ? <Loader2 className="animate-spin" /> : <Store />}
          Enable Play publishing
        </Button>
        <Text variant="meta">
          App not on Play yet? Create it in{" "}
          <a
            href="https://play.google.com/console/u/0/developers"
            target="_blank"
            rel="noreferrer"
            className="underline underline-offset-2 hover:text-foreground"
          >
            Play Console
          </a>{" "}
          with package <span className={monoClass}>{platform.packageName}</span> first; the API
          can't create apps.
        </Text>
      </div>
    </form>
  );
}
