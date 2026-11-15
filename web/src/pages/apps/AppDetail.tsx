import { Link, useParams, useSearchParams } from "react-router-dom";
import { Layers, RotateCcw } from "lucide-react";
import { Breadcrumbs } from "@groot/ui/breadcrumb";
import { Button } from "@groot/ui/button";
import { Card } from "@groot/ui/card";
import { EmptyState, ErrorState } from "@groot/ui/empty-state";
import { Skeleton, SkeletonCard } from "@groot/ui/loading-skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@groot/ui/tabs";
import { PageLayout } from "@groot/shell/components/layout/PageLayout";
import { Section } from "@groot/shell/components/layout/Section";
import { ApiError } from "@groot/shell/lib/api";
import { PlatformBadge } from "@/components/badges";
import { useApp } from "@/lib/hub";
import { fromNow, platformLabel } from "@/lib/format";
import type { AppDetail, PlatformDetail } from "@/lib/types";
import { AddPlatformDialog } from "./AddPlatformDialog";
import { ConfigurationPanel } from "./ConfigurationPanel";
import { DeleteAppDialog } from "./DeleteAppDialog";
import { DistributionPanel } from "./DistributionPanel";
import { ReleasesPanel } from "./ReleasesPanel";

const TABS = [
  { id: "releases", label: "Releases" },
  { id: "distribution", label: "Distribution" },
  { id: "configuration", label: "Configuration" },
] as const;
type TabId = (typeof TABS)[number]["id"];

const isTab = (v: string | null): v is TabId => TABS.some((t) => t.id === v);

export function AppDetailPage() {
  const { slug = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const tab: TabId = isTab(params.get("tab")) ? (params.get("tab") as TabId) : "releases";
  const { data: app, isLoading, isError, error, refetch } = useApp(slug);

  const breadcrumb = (
    <Breadcrumbs page={slug}>
      <Link to="/apps">Apps</Link>
    </Breadcrumbs>
  );

  if (isLoading) {
    return (
      <PageLayout title={slug} breadcrumb={breadcrumb} maxWidth="6xl">
        <Skeleton className="h-9 w-80 rounded-xl" />
        <SkeletonCard />
      </PageLayout>
    );
  }
  if (isError || !app) {
    const missing = error instanceof ApiError && error.status === 404;
    return (
      <PageLayout title={slug} breadcrumb={breadcrumb} maxWidth="6xl">
        <Card className="p-0">
          <ErrorState
            title={missing ? "No such app" : "Failed to load the app"}
            description={
              missing
                ? `No app is registered as “${slug}”.`
                : "The hub did not answer. Retry in a moment."
            }
            action={
              missing ? (
                <Button variant="outline" size="sm" asChild>
                  <Link to="/apps">Back to apps</Link>
                </Button>
              ) : (
                <Button variant="outline" size="sm" onClick={() => void refetch()}>
                  <RotateCcw />
                  Retry
                </Button>
              )
            }
          />
        </Card>
      </PageLayout>
    );
  }

  const canAddPlatform = app.platforms.length < 2;
  return (
    <PageLayout
      title={app.slug}
      breadcrumb={breadcrumb}
      titleAdornment={
        <span className="flex gap-1.5">
          {app.platforms.map((p) => (
            <PlatformBadge key={p.platform} platform={p.platform} />
          ))}
        </span>
      }
      description={`Registered ${fromNow(app.createdAt)}`}
      actions={
        <span className="flex gap-1.5">
          {canAddPlatform && app.platforms.length > 0 ? <AddPlatformDialog app={app} /> : undefined}
          <DeleteAppDialog app={app} />
        </span>
      }
      maxWidth="6xl"
    >
      {app.platforms.length === 0 ? (
        <Card className="p-0">
          <EmptyState
            icon={Layers}
            title="No platforms yet"
            description="Add the Android or iOS variant. Android platforms get a signing keystore generated on the spot."
            action={<AddPlatformDialog app={app} />}
          />
        </Card>
      ) : (
        <Tabs
          value={tab}
          onValueChange={(next) =>
            setParams(
              (p) => {
                if (next === "releases") p.delete("tab");
                else p.set("tab", next);
                return p;
              },
              { replace: true },
            )
          }
        >
          <TabsList variant="line" className="mb-6">
            {TABS.map((t) => (
              <TabsTrigger key={t.id} value={t.id} className="flex-none px-3">
                {t.label}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="releases" className="space-y-8">
            <PerPlatform app={app} render={(p) => <ReleasesPanel slug={app.slug} platform={p} />} />
          </TabsContent>
          <TabsContent value="distribution" className="space-y-8">
            <PerPlatform
              app={app}
              render={(p) => <DistributionPanel slug={app.slug} platform={p} />}
            />
          </TabsContent>
          <TabsContent value="configuration" className="space-y-8">
            <PerPlatform app={app} render={(p) => <ConfigurationPanel app={app} platform={p} />} />
          </TabsContent>
        </Tabs>
      )}
    </PageLayout>
  );
}

/** One section per platform; the heading is dropped when there is only one. */
function PerPlatform({
  app,
  render,
}: {
  app: AppDetail;
  render: (p: PlatformDetail) => React.ReactNode;
}) {
  if (app.platforms.length === 1) return <>{render(app.platforms[0])}</>;
  return (
    <>
      {app.platforms.map((p) => (
        <Section
          key={p.platform}
          title={platformLabel(p.platform)}
          meta={<span className="font-mono">{p.packageName}</span>}
        >
          <div className="space-y-6">{render(p)}</div>
        </Section>
      ))}
    </>
  );
}
