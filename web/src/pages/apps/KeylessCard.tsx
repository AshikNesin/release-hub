import { useState } from "react";
import { Link } from "react-router-dom";
import { Cable, GitBranch, Loader2, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@groot/ui/alert";
import { Button } from "@groot/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@groot/ui/card";
import { EmptyState } from "@groot/ui/empty-state";
import { Input } from "@groot/ui/input";
import { Label } from "@groot/ui/label";
import { useConfirm } from "@groot/ui/primitives";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@groot/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@groot/ui/table";
import { Text, monoClass } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import {
  releaseHub,
  keys,
  useGithub,
  useGithubRepositories,
  useOidcGrants,
} from "@/lib/releaseHub";
import { formatDateTime, fromNow } from "@/lib/format";
import type { AppDetail, GithubConnectResult, OidcGrant } from "@/lib/types";

const GITHUB_ISSUER = "https://token.actions.githubusercontent.com";

/** Default-branch ref suffix appended to a subject pasted without one. */
const REF_SUFFIX = ":ref:refs/heads/main";

/**
 * GitHub's OIDC settings page shows just the repo part (`repo:owner@ID/name@ID`,
 * or `repo:owner/name` on older repos), but the token's subject also carries the
 * ref. Complete a bare prefix to the default branch; subjects that already name
 * a ref, environment or pull request are kept verbatim.
 */
function completeSubject(subject: string): string {
  const s = subject.trim();
  if (!s.startsWith("repo:") || [":ref:", ":environment:", ":pull_request"].some((p) => s.includes(p))) {
    return s;
  }
  return s + REF_SUFFIX;
}

/**
 * The workflow snippet CI copies. Audience is fixed; the grant decides which
 * repo may publish. With `vars` (a repo connected through the GitHub App),
 * the URL and slug come from its RELEASE_HUB_URL / RELEASE_HUB_APP variables.
 */
function workflowSnippet(slug: string, releaseHubUrl: string, vars: boolean): string {
  const url = vars ? "$RELEASE_HUB_URL" : releaseHubUrl;
  const app = vars ? "$RELEASE_HUB_APP" : slug;
  const env = vars
    ? `
    env:
      RELEASE_HUB_URL: \${{ vars.RELEASE_HUB_URL }}
      RELEASE_HUB_APP: \${{ vars.RELEASE_HUB_APP }}`
    : "";
  return `# .github/workflows/release.yml: keyless publish to ${slug}
permissions:
  id-token: write   # request the repo's OIDC token
  contents: read

jobs:
  release:
    runs-on: ubuntu-latest${env}
    steps:
      - uses: actions/checkout@v4
      - name: Get a release-hub token (no stored secret)
        run: |
          oidc=$(curl -fsS \\
            -H "Authorization: Bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" \\
            -H "Accept: application/json; api-version=2.0" \\
            "$ACTIONS_ID_TOKEN_REQUEST_URL&audience=release-hub" | jq -r .value)
          RELEASE_HUB_TOKEN=$(curl -fsS -X POST "${url}/api/oidc/exchange" \\
            --data-urlencode "token=$oidc" | jq -r .token)
          echo "::add-mask::$RELEASE_HUB_TOKEN"
          echo "RELEASE_HUB_TOKEN=$RELEASE_HUB_TOKEN" >> "$GITHUB_ENV"
      # …then upload as usual:
      # curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" -F file=@app-release.apk \\
      #   -F channel=direct "${url}/api/apps/${app}/releases"`;
}

export function KeylessCard({ app }: { app: AppDetail }) {
  const { data: grants, isLoading } = useOidcGrants(app.slug);
  const { data: github } = useGithub();
  const [subject, setSubject] = useState("");
  const [name, setName] = useState("");
  // The release hub's own base URL, as the upload URLs already carry it.
  const releaseHubUrl = app.platforms[0]?.uploadUrl?.replace(/\/api\/apps.*$/, "") ?? "";

  const create = useAction(
    (v: { subject: string; name: string }) =>
      releaseHub.createOidcGrant(app.slug, {
        issuer: GITHUB_ISSUER,
        subject: v.subject,
        name: v.name,
      }),
    {
      success: () => ({ title: "Grant created", description: "That repo can now publish this app." }),
      errorTitle: "Could not create the grant",
      fallback: "Unable to create the grant",
      invalidate: [keys.oidcGrants(app.slug), keys.allOidcGrants],
      onSuccess: () => {
        setSubject("");
        setName("");
      },
    },
  );
  const revoke = useAction((g: OidcGrant) => releaseHub.deleteOidcGrant(g.id, g.repositoryId !== null), {
    confirm: (g) => ({
      title: "Remove this grant?",
      description: `The repo ${g.name || g.subject} loses keyless access immediately; its minted tokens stop working.${
        g.repositoryId !== null
          ? " Its RELEASE_HUB_URL and RELEASE_HUB_APP variables are removed too, where they still hold this release hub's values."
          : ""
      }`,
      confirmLabel: "Remove grant",
      destructive: true,
    }),
    success: (r) =>
      r.variablesError
        ? { title: "Grant removed", description: `The repo variables were kept: ${r.variablesError}` }
        : "Grant removed",
    errorTitle: "Could not remove the grant",
    fallback: "Unable to remove the grant",
    invalidate: [keys.oidcGrants(app.slug), keys.allOidcGrants],
  });

  // The manual path: paste a repo's OIDC subject (any CI, or no GitHub App).
  const manualForm = (
    <>
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (subject.trim()) void create.run({ subject: completeSubject(subject), name: name.trim() });
          }}
        >
          <div className="min-w-64 flex-1 space-y-2">
            <Label htmlFor="oidc-subject">Repository OIDC subject</Label>
            <Input
              id="oidc-subject"
              placeholder="repo:owner/name or repo:owner@ID/name@ID"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              onBlur={() => setSubject((s) => completeSubject(s))}
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <div className="w-44 space-y-2">
            <Label htmlFor="oidc-name">Label</Label>
            <Input
              id="oidc-name"
              placeholder="owner/name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoComplete="off"
            />
          </div>
          <Button type="submit" disabled={!subject.trim() || create.isPending}>
            {create.isPending ? <Loader2 className="animate-spin" /> : <Plus />}
            Add grant
          </Button>
        </form>
        <Text variant="meta" className="block">
          Paste the subject from the repo's Settings → Actions → OIDC page —{" "}
          <code className={monoClass}>repo:owner@ID/name@ID</code> is completed to the default-branch
          ref (<code className={monoClass}>{REF_SUFFIX}</code>) automatically. The issuer is fixed
          to <code className={monoClass}>{GITHUB_ISSUER}</code>.
        </Text>

    </>
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="size-4" /> Keyless publishing
        </CardTitle>
        <CardDescription>
          Let a GitHub repo publish this app with no stored secret: its Actions
          identity is exchanged for a 1-hour release-hub token scoped to{" "}
          <code className={monoClass}>{app.slug}</code> alone. One repo → one app.
        </CardDescription>
        <CardAction>
          {grants?.length ? (
            <span className="text-muted-foreground text-xs">
              {grants.length} grant{grants.length === 1 ? "" : "s"}
            </span>
          ) : undefined}
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-4">
        {isLoading ? null : !grants?.length ? (
          <EmptyState
            icon={GitBranch}
            title="No keyless grants yet"
            description="Add the repository that publishes this app and drop the RELEASE_HUB_TOKEN secret from CI."
          />
        ) : (
          <div className="overflow-hidden rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-4">Repository</TableHead>
                  <TableHead>Identity</TableHead>
                  <TableHead>Last used</TableHead>
                  <TableHead className="w-0 pr-4">
                    <span className="sr-only">Actions</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {grants.map((g) => (
                  <TableRow key={g.id}>
                    <TableCell className="pl-4 font-medium">{g.name || subjectRepo(g.subject)}</TableCell>
                    <TableCell className="max-w-[22rem]">
                      {g.repositoryId !== null ? (
                        <span className="text-xs text-muted-foreground" title={`GitHub repository id ${g.repositoryId}`}>
                          Pushes to <code className={monoClass}>{g.ref?.replace(/^refs\/heads\//, "")}</code>{" "}
                          · via GitHub App
                        </span>
                      ) : (
                        <span className="font-mono text-xs text-muted-foreground" title={g.subject}>
                          {g.subject}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {g.lastUsedAt ? (
                        <span title={formatDateTime(g.lastUsedAt)}>{fromNow(g.lastUsedAt)}</span>
                      ) : (
                        <Text variant="meta">Never</Text>
                      )}
                    </TableCell>
                    <TableCell className="pr-4">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="Remove grant"
                        title="Remove"
                        disabled={revoke.isPending}
                        onClick={() => void revoke.run(g)}
                        className="text-muted-foreground hover:text-destructive"
                      >
                        <Trash2 />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        {github?.app ? (
          <>
            <ConnectRepository slug={app.slug} />
            <details className="group rounded-xl border px-4 py-3">
              <summary className="cursor-pointer text-sm text-muted-foreground select-none">
                Other CI, or set up by hand: paste an OIDC subject
              </summary>
              <div className="mt-3 space-y-2">
        {manualForm}
              </div>
            </details>
          </>
        ) : (
          <>
        {manualForm}
            {github && !github.app ? (
              <Text variant="meta" className="block">
                Tired of pasting? <Link to="/settings/github" className="underline">Connect GitHub</Link>{" "}
                once, then pick the repository here: release-hub creates the grant and sets its{" "}
                <code className={monoClass}>RELEASE_HUB_URL</code> /{" "}
                <code className={monoClass}>RELEASE_HUB_APP</code> variables for you.
              </Text>
            ) : null}
          </>
        )}

        {grants?.length ? (
          <Alert>
            <GitBranch />
            <AlertTitle>Workflow snippet</AlertTitle>
            <AlertDescription className="w-full space-y-2">
              <p>No RELEASE_HUB_TOKEN secret needed; just <code className={monoClass}>id-token: write</code>.</p>
              <pre className="overflow-x-auto rounded-lg bg-muted p-3 text-xs">
                <code>
                  {workflowSnippet(
                    app.slug,
                    releaseHubUrl || "https://release-hub.example.com",
                    grants.some((g) => g.repositoryId !== null),
                  )}
                </code>
              </pre>
            </AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  );
}

/**
 * Connect a repository the GitHub App can reach: one click creates a
 * repository grant (by repo id, default branch) and sets the repo's
 * RELEASE_HUB_URL / RELEASE_HUB_APP Actions variables. A variable that
 * already holds something else is only replaced after a confirmation.
 */
function ConnectRepository({ slug }: { slug: string }) {
  const repos = useGithubRepositories();
  const confirm = useConfirm();
  const [repository, setRepository] = useState("");
  const free = repos.data?.filter((r) => !r.app) ?? [];

  const connect = useAction(
    async (v: { repository: string }): Promise<GithubConnectResult> => {
      const first = await releaseHub.githubConnect(slug, v.repository);
      if (first.connected) return first;
      const ok = await confirm({
        title: "Replace existing repo variables?",
        description: `${first.repository} already has ${first.conflicts
          .map((c) => `${c.name} = ${c.current}`)
          .join(", ")}. Connecting sets ${first.conflicts.map((c) => `${c.name} = ${c.value}`).join(", ")}.`,
        confirmLabel: "Replace and connect",
      });
      return ok ? releaseHub.githubConnect(slug, v.repository, true) : first;
    },
    {
      success: (r) =>
        r.connected
          ? {
              title: `Connected ${r.repository}`,
              description: `Pushes to ${r.ref.replace(/^refs\/heads\//, "")} can publish ${slug}; RELEASE_HUB_URL and RELEASE_HUB_APP are set.`,
            }
          : { title: "Nothing changed", description: `${r.repository} was not connected.` },
      errorTitle: "Could not connect the repository",
      fallback: "Unable to connect the repository",
      invalidate: [keys.oidcGrants(slug), keys.allOidcGrants, keys.githubRepositories],
      onSuccess: (r) => {
        if (r.connected) setRepository("");
      },
    },
  );

  return (
    <div className="space-y-2">
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (repository) void connect.run({ repository });
        }}
      >
        <div className="min-w-64 flex-1 space-y-2">
          <Label htmlFor="gh-repo">GitHub repository</Label>
          <Select value={repository} onValueChange={setRepository} disabled={repos.isLoading || !free.length}>
            <SelectTrigger id="gh-repo" className="w-full">
              <SelectValue
                placeholder={
                  repos.isLoading
                    ? "Loading repositories…"
                    : free.length
                      ? "Pick the repository that publishes this app"
                      : "No unconnected repositories"
                }
              />
            </SelectTrigger>
            <SelectContent>
              {free.map((r) => (
                <SelectItem key={r.id} value={r.fullName}>
                  {r.fullName}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button type="submit" disabled={!repository || connect.isPending}>
          {connect.isPending ? <Loader2 className="animate-spin" /> : <Cable />}
          Connect
        </Button>
      </form>
      <Text variant="meta" className="block">
        {repos.isError ? (
          <span className="text-destructive">
            {repos.error instanceof Error ? repos.error.message : "Could not reach GitHub"}.{" "}
          </span>
        ) : null}
        Missing a repository?{" "}
        <Link to="/settings/github" className="underline">
          Install the GitHub App on it
        </Link>
        . Repos that already publish another app aren't listed (one repo → one app).
      </Text>
    </div>
  );
}

/** "repo:owner@ID/name@ID:…" → "owner/name" for display (IDs stripped). */
function subjectRepo(subject: string): string {
  const m = /^repo:([^:]+)/.exec(subject);
  return m ? m[1].replace(/@\d+/g, "") : subject;
}
