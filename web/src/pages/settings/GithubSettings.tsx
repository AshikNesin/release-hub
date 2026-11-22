import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { ExternalLink, GitBranch, Loader2, RefreshCw, TriangleAlert, Unplug } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@groot/ui/alert";
import { Button } from "@groot/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@groot/ui/card";
import { Input } from "@groot/ui/input";
import { Label } from "@groot/ui/label";
import { SkeletonList } from "@groot/ui/loading-skeleton";
import { StatusBadge } from "@groot/ui/status-badge";
import { monoClass, Text } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { keys, releaseHub, useGithub, useGithubRepositories } from "@/lib/releaseHub";
import type { GithubSettings as GithubSettingsInfo } from "@/lib/types";

/**
 * GitHub's manifest flow takes a form POST from the browser: build the form
 * and submit it, so GitHub shows its "Create GitHub App" screen with the
 * manifest filled in. Nothing is pasted.
 */
function submitManifest(url: string, manifest: object) {
  const form = document.createElement("form");
  form.method = "post";
  form.action = url;
  const input = document.createElement("input");
  input.type = "hidden";
  input.name = "manifest";
  input.value = JSON.stringify(manifest);
  form.append(input);
  document.body.append(form);
  form.submit();
}

const CALLBACK_MESSAGES: Record<string, { tone: "success" | "error"; text: string }> = {
  connected: {
    tone: "success",
    text: "GitHub App created. Now install it on the repositories that publish your apps.",
  },
  installed: {
    tone: "success",
    text: "Installation updated. Connect a repository from any app's Keyless publishing card.",
  },
  error: { tone: "error", text: "GitHub could not finish the setup. Start again." },
};

/**
 * Settings → GitHub: register this release hub's own GitHub App with one
 * confirm on GitHub (manifest flow), install it on repositories, and see
 * which repositories it reaches. Ported from FeedbackBee's GitHub settings.
 */
export function GithubSettings() {
  const [params, setParams] = useSearchParams();
  const queryClient = useQueryClient();
  const { data, isLoading } = useGithub();
  const callback = CALLBACK_MESSAGES[params.get("github") ?? ""];

  return (
    <div className="space-y-4">
      {callback ? (
        <Alert variant={callback.tone === "error" ? "destructive" : "default"}>
          {callback.tone === "error" ? <TriangleAlert /> : <GitBranch className="text-success" />}
          <AlertDescription className="flex w-full items-center justify-between gap-2">
            <span>{callback.text}</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                params.delete("github");
                setParams(params, { replace: true });
              }}
            >
              Dismiss
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {isLoading || !data ? (
        <Card className="p-0">
          <SkeletonList rows={2} />
        </Card>
      ) : data.app ? (
        <Connected info={data} onRefresh={() => queryClient.invalidateQueries({ queryKey: keys.github })} />
      ) : (
        <Register info={data} />
      )}
    </div>
  );
}

function Register({ info }: { info: GithubSettingsInfo }) {
  const [organization, setOrganization] = useState("");
  const create = useAction((org: string) => releaseHub.githubManifest(org), {
    errorTitle: "Could not start the GitHub setup",
    fallback: "Unable to start the GitHub setup",
    onSuccess: (r) => submitManifest(r.url, r.manifest),
  });
  const blocked = !info.baseUrlOk || !info.canStoreKey;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranch className="size-4" /> Connect GitHub
        </CardTitle>
        <CardDescription>
          Creates a private GitHub App owned by you (or your organization) for this release hub only.
          Then any app's Keyless publishing card can link a repository in one click: it creates the
          grant and sets the repo's <code className={monoClass}>{info.variables.join(", ")}</code>{" "}
          Actions variables.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!info.baseUrlOk ? (
          <Alert variant="destructive">
            <TriangleAlert />
            <AlertTitle>Set the public URL first</AlertTitle>
            <AlertDescription>{info.baseUrlProblem}</AlertDescription>
          </Alert>
        ) : null}
        {!info.canStoreKey ? (
          <Alert variant="destructive">
            <TriangleAlert />
            <AlertTitle>Nowhere to keep the app's private key</AlertTitle>
            <AlertDescription>
              Set <code className={monoClass}>RELEASE_HUB_SECRET_KEY</code> or set up 1Password: the
              GitHub App's key is stored like your signing keys.
            </AlertDescription>
          </Alert>
        ) : null}
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void create.run(organization.trim());
          }}
        >
          <div className="min-w-56 flex-1 space-y-2">
            <Label htmlFor="gh-org">Organization (optional)</Label>
            <Input
              id="gh-org"
              placeholder="Leave blank for your personal account"
              value={organization}
              onChange={(e) => setOrganization(e.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <Button type="submit" disabled={blocked || create.isPending}>
            {create.isPending ? <Loader2 className="animate-spin" /> : <ExternalLink />}
            Create GitHub App
          </Button>
        </form>
        <Text variant="meta" className="block">
          GitHub asks you to confirm once, then you pick the repositories it may see. The app gets
          only Metadata (read) and Variables (read and write); webhooks stay off, so GitHub never
          needs to reach this server.
        </Text>
      </CardContent>
    </Card>
  );
}

function Connected({ info, onRefresh }: { info: GithubSettingsInfo; onRefresh: () => void }) {
  const app = info.app!;
  const repos = useGithubRepositories();
  const install = useAction(() => releaseHub.githubInstall(), {
    errorTitle: "Could not open GitHub",
    fallback: "Unable to start the installation",
    onSuccess: (r) => window.location.assign(r.url),
  });
  const disconnect = useAction(() => releaseHub.githubDisconnect(), {
    confirm: {
      title: "Disconnect the GitHub App?",
      description:
        "Release-hub forgets the app's key. Existing grants keep publishing; connecting new repositories needs the app again. The app stays registered and installed on GitHub, so delete it there too if you no longer need it.",
      confirmLabel: "Disconnect",
      destructive: true,
    },
    success: () => "GitHub App disconnected",
    errorTitle: "Could not disconnect",
    fallback: "Unable to disconnect",
    invalidate: [keys.github, keys.githubRepositories],
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranch className="size-4" /> {app.name}
        </CardTitle>
        <CardDescription>
          GitHub App <code className={monoClass}>{app.slug}</code>
          {app.owner ? <> owned by {app.owner}</> : null}. Connect repositories from each app's
          Keyless publishing card.
        </CardDescription>
        <CardAction>
          <StatusBadge status="success" label="Connected" />
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-4">
        {!info.baseUrlOk ? (
          <Alert variant="destructive">
            <TriangleAlert />
            <AlertDescription>{info.baseUrlProblem}</AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => void install.run(undefined)} disabled={install.isPending}>
            {install.isPending ? <Loader2 className="animate-spin" /> : <ExternalLink />}
            Install or choose repositories
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              onRefresh();
              void repos.refetch();
            }}
            disabled={repos.isFetching}
          >
            <RefreshCw className={repos.isFetching ? "animate-spin" : undefined} />
            Refresh
          </Button>
          {app.htmlUrl ? (
            <Button variant="ghost" asChild>
              <a href={app.htmlUrl} target="_blank" rel="noreferrer">
                App on GitHub
              </a>
            </Button>
          ) : null}
          <Button
            variant="ghost"
            onClick={() => void disconnect.run(undefined)}
            disabled={disconnect.isPending}
            className="text-muted-foreground hover:text-destructive"
          >
            <Unplug /> Disconnect
          </Button>
        </div>

        <div className="space-y-2">
          <Text variant="title">Repositories the app can reach</Text>
          {repos.isLoading ? (
            <SkeletonList rows={2} />
          ) : repos.isError ? (
            <Text tone="destructive" className="block">
              {repos.error instanceof Error ? repos.error.message : "Could not reach GitHub"}
            </Text>
          ) : !repos.data?.length ? (
            <Text tone="muted" className="block">
              None yet. Install the app on at least one repository.
            </Text>
          ) : (
            <ul className="divide-y rounded-xl border">
              {repos.data.map((r) => (
                <li key={r.id} className="flex items-center justify-between gap-2 px-4 py-2">
                  <span className={monoClass}>{r.fullName}</span>
                  {r.app ? (
                    <Text variant="meta">publishes {r.app}</Text>
                  ) : (
                    <Text variant="meta">not connected</Text>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
