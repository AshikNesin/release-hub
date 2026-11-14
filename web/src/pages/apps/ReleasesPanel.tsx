import { Download, Trash2, TriangleAlert, Upload } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@groot/ui/alert";
import { Button } from "@groot/ui/button";
import { Card } from "@groot/ui/card";
import { EmptyState } from "@groot/ui/empty-state";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@groot/ui/table";
import { Text } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { ChannelBadge, PlayStatus } from "@/components/badges";
import { CopyField } from "@/components/copy";
import { hub, keys } from "@/lib/hub";
import { formatBytes, formatDateTime, fromNow } from "@/lib/format";
import type { PlatformDetail, Release } from "@/lib/types";

/** The curl line CI uses to publish a build (bearer token from Settings → API tokens). */
export const uploadCommand = (platform: PlatformDetail, channel = "direct") =>
  `curl -H "Authorization: Bearer $HUB_TOKEN" -F file=@${platform.platform === "ios" ? "App.ipa" : "app-release.apk"} -F channel=${channel} ${platform.uploadUrl}`;

export function ReleasesPanel({ slug, platform }: { slug: string; platform: PlatformDetail }) {
  const remove = useAction(
    (r: Release) => hub.deleteRelease(slug, platform.platform, r.versionCode),
    {
      confirm: (r) => ({
        title: `Delete ${r.versionName} (${r.versionCode})?`,
        description:
          "The release and its artifact file are removed. Devices on the direct channel fall back to the previous release.",
        confirmLabel: "Delete release",
        destructive: true,
      }),
      success: (_, r) => `Deleted ${r.versionName} (${r.versionCode})`,
      errorTitle: "Delete failed",
      fallback: "Unable to delete the release",
      invalidate: [keys.app(slug), keys.apps],
    },
  );

  if (platform.releases.length === 0) {
    return (
      <Card className="p-0">
        <EmptyState
          icon={Upload}
          title="No releases yet"
          description="Publish the first build from CI with an API token (Settings → API tokens):"
          action={
            <CopyField
              value={uploadCommand(platform)}
              what="Upload command"
              className="w-[min(40rem,80vw)]"
            />
          }
        />
      </Card>
    );
  }

  return (
    <Card className="overflow-hidden p-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="pl-5">Version</TableHead>
            <TableHead>Channel</TableHead>
            <TableHead className="hidden md:table-cell">Size</TableHead>
            <TableHead className="hidden sm:table-cell">Uploaded</TableHead>
            <TableHead className="w-0 pr-5 text-right">
              <span className="sr-only">Actions</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {platform.releases.map((r) => (
            <ReleaseRows
              key={r.versionCode}
              release={r}
              onDelete={r.channel === "direct" ? () => void remove.run(r) : undefined}
              deleting={remove.isPending}
            />
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

function ReleaseRows({
  release: r,
  onDelete,
  deleting,
}: {
  release: Release;
  onDelete?: () => void;
  deleting: boolean;
}) {
  const failed = r.playStatus === "error" && r.channel !== "direct";
  return (
    <>
      <TableRow className={failed ? "border-b-0" : undefined}>
        <TableCell className="pl-5">
          <div className="flex items-baseline gap-1.5">
            <span className="font-medium tabular-nums">{r.versionName}</span>
            <span className="text-xs text-muted-foreground tabular-nums">({r.versionCode})</span>
          </div>
          {r.notes && (
            <Text variant="meta" className="mt-0.5 line-clamp-1 max-w-md" title={r.notes}>
              {r.notes}
            </Text>
          )}
        </TableCell>
        <TableCell>
          <div className="flex flex-wrap items-center gap-1.5">
            <ChannelBadge channel={r.channel} />
            <span title={r.playRelease ? `On Play as ${r.playRelease}` : undefined}>
              <PlayStatus release={r} />
            </span>
          </div>
        </TableCell>
        <TableCell className="hidden text-muted-foreground tabular-nums md:table-cell">
          {formatBytes(r.sizeBytes)}
        </TableCell>
        <TableCell
          className="hidden text-muted-foreground sm:table-cell"
          title={formatDateTime(r.createdAt)}
        >
          {fromNow(r.createdAt)}
        </TableCell>
        <TableCell className="pr-5">
          <div className="flex justify-end gap-0.5">
            <Button
              variant="ghost"
              size="icon-sm"
              asChild
              className="text-muted-foreground hover:text-foreground"
            >
              <a
                href={r.url}
                download
                aria-label={`Download ${r.versionName}`}
                title={`Download · sha256 ${r.sha256.slice(0, 12)}…`}
              >
                <Download />
              </a>
            </Button>
            {onDelete && (
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={onDelete}
                disabled={deleting}
                aria-label={`Delete ${r.versionName}`}
                title="Delete release"
                className="text-muted-foreground hover:text-destructive"
              >
                <Trash2 />
              </Button>
            )}
          </div>
        </TableCell>
      </TableRow>
      {failed && (
        <TableRow className="hover:bg-transparent">
          <TableCell colSpan={5} className="px-5 pt-0 pb-4 whitespace-normal">
            <Alert variant="destructive">
              <TriangleAlert />
              <AlertTitle>Play push failed — manual action needed</AlertTitle>
              <AlertDescription>
                <p>The build is stored on the hub but is not on Google Play: {r.playError}</p>
                <p>
                  Fix it (usually in Play Console), then re-upload with a higher versionCode to
                  retry.
                </p>
              </AlertDescription>
            </Alert>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}
