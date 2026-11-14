import { Link } from "react-router-dom";
import { Box, ChevronRight, Layers, Package, RotateCcw, Store } from "lucide-react";
import { Button } from "@groot/ui/button";
import { Card } from "@groot/ui/card";
import { EmptyState, ErrorState } from "@groot/ui/empty-state";
import { listRowClass } from "@groot/ui/list-row";
import { SkeletonList, Skeleton } from "@groot/ui/loading-skeleton";
import { StatTile } from "@groot/ui/stat-tile";
import { StatusBadge } from "@groot/ui/status-badge";
import { Text } from "@groot/ui/typography";
import { cn } from "@groot/ui/lib/utils";
import { PageLayout } from "@groot/shell/components/layout/PageLayout";
import { ChannelBadge, PlatformBadge } from "@/components/badges";
import { useApps } from "@/lib/hub";
import { fromNow, formatDateTime } from "@/lib/format";
import type { AppSummary } from "@/lib/types";
import { RegisterAppDialog } from "./RegisterAppDialog";

export function Apps() {
  const { data: apps, isLoading, isError, refetch } = useApps();

  const platforms = apps?.flatMap((a) => a.platforms) ?? [];
  const stats = [
    { label: "Apps", value: apps?.length ?? 0, icon: <Box className="size-3.5" /> },
    { label: "Platforms", value: platforms.length, icon: <Layers className="size-3.5" /> },
    {
      label: "Releases",
      value: platforms.reduce((n, p) => n + p.releaseCount, 0),
      icon: <Package className="size-3.5" />,
    },
    {
      label: "On Google Play",
      value: platforms.filter((p) => p.playEnabled).length,
      icon: <Store className="size-3.5" />,
    },
  ];

  return (
    <PageLayout
      title="Apps"
      description="One slug per product. Platforms carry the package name, signing key and releases."
      actions={<RegisterAppDialog />}
      maxWidth="6xl"
    >
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {stats.map((s) =>
          isLoading ? (
            <Skeleton key={s.label} className="h-[74px] rounded-2xl" />
          ) : (
            <StatTile key={s.label} label={s.label} value={s.value} icon={s.icon} />
          ),
        )}
      </div>

      <Card className="overflow-hidden p-0">
        {isLoading ? (
          <SkeletonList rows={4} />
        ) : isError ? (
          <ErrorState
            title="Failed to load apps"
            description="The hub did not answer. Check it is running, then retry."
            action={
              <Button variant="outline" size="sm" onClick={() => void refetch()}>
                <RotateCcw />
                Retry
              </Button>
            }
          />
        ) : !apps?.length ? (
          <EmptyState
            icon={Box}
            title="No apps yet"
            description="Register an app, add its Android or iOS platform, then point CI at its upload URL."
            action={<RegisterAppDialog />}
          />
        ) : (
          <ul className="divide-y divide-border">
            {apps.map((app) => (
              <AppRow key={app.slug} app={app} />
            ))}
          </ul>
        )}
      </Card>
    </PageLayout>
  );
}

function AppRow({ app }: { app: AppSummary }) {
  // The newest release across the app's platforms.
  const latest = app.platforms
    .map((p) => p.latest)
    .filter((r) => r !== null)
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0];
  const needsAction = app.platforms.some((p) => p.latest?.playStatus === "error");

  return (
    <li>
      <Link
        to={`/apps/${app.slug}`}
        className={cn(listRowClass, "group outline-none focus-visible:bg-muted/40")}
      >
        <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-muted text-sm font-semibold text-muted-foreground uppercase">
          {app.slug[0]}
        </span>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate text-sm font-medium text-foreground">{app.slug}</span>
            {app.platforms.map((p) => (
              <PlatformBadge key={p.platform} platform={p.platform} />
            ))}
            {needsAction && <StatusBadge status="error" label="Play: action needed" />}
          </div>
          <Text variant="meta" className="mt-0.5 block truncate font-mono">
            {app.platforms.length
              ? [...new Set(app.platforms.map((p) => p.packageName))].join(" · ")
              : "No platform yet — open to add Android or iOS"}
          </Text>
        </div>

        <div className="hidden shrink-0 flex-col items-end gap-1 sm:flex">
          {latest ? (
            <>
              <div className="flex items-center gap-2">
                <span className="text-sm tabular-nums">
                  {latest.versionName}{" "}
                  <span className="text-muted-foreground">({latest.versionCode})</span>
                </span>
                <ChannelBadge channel={latest.channel} />
              </div>
              <Text variant="meta" title={formatDateTime(latest.createdAt)}>
                {fromNow(latest.createdAt)}
              </Text>
            </>
          ) : (
            <Text variant="meta">No releases</Text>
          )}
        </div>

        <ChevronRight className="size-4 shrink-0 text-muted-foreground group-hover:text-foreground" />
      </Link>
    </li>
  );
}
