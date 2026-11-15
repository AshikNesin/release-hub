import { useState } from "react";
import { GitBranch, Loader2, Plus, ShieldCheck, Trash2 } from "lucide-react";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@groot/ui/table";
import { Text, monoClass } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { releaseHub, keys, useOidcGrants } from "@/lib/releaseHub";
import { formatDateTime, fromNow } from "@/lib/format";
import type { AppDetail } from "@/lib/types";

const GITHUB_ISSUER = "https://token.actions.githubusercontent.com";

/** The workflow snippet CI copies. Audience is fixed; subject comes from the grant. */
function workflowSnippet(slug: string, releaseHubUrl: string): string {
  return `# .github/workflows/release.yml: keyless publish to ${slug}
permissions:
  id-token: write   # request the repo's OIDC token
  contents: read

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Get a release-hub token (no stored secret)
        run: |
          oidc=$(curl -fsS \\
            -H "Authorization: Bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" \\
            -H "Accept: application/json; api-version=2.0" \\
            "$ACTIONS_ID_TOKEN_REQUEST_URL&audience=release-hub" | jq -r .value)
          RELEASE_HUB_TOKEN=$(curl -fsS -X POST "${releaseHubUrl}/api/oidc/exchange" \\
            --data-urlencode "token=$oidc" | jq -r .token)
          echo "::add-mask::$RELEASE_HUB_TOKEN"
          echo "RELEASE_HUB_TOKEN=$RELEASE_HUB_TOKEN" >> "$GITHUB_ENV"
      # …then upload as usual:
      # curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" -F file=@app-release.apk \\
      #   -F channel=direct "${releaseHubUrl}/api/apps/${slug}/releases"`;
}

export function KeylessCard({ app }: { app: AppDetail }) {
  const { data: grants, isLoading } = useOidcGrants(app.slug);
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
      invalidate: [keys.oidcGrants(app.slug)],
      onSuccess: () => {
        setSubject("");
        setName("");
      },
    },
  );
  const revoke = useAction((id: number) => releaseHub.deleteOidcGrant(id), {
    confirm: (id) => {
      const g = grants?.find((x) => x.id === id);
      return {
        title: "Remove this grant?",
        description: `The repo ${
          g?.name || g?.subject || ""
        } loses keyless access immediately; its minted tokens stop working.`,
        confirmLabel: "Remove grant",
        destructive: true,
      };
    },
    success: () => "Grant removed",
    errorTitle: "Could not remove the grant",
    fallback: "Unable to remove the grant",
    invalidate: [keys.oidcGrants(app.slug)],
  });

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
                  <TableHead>Subject</TableHead>
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
                      <span className="font-mono text-xs text-muted-foreground" title={g.subject}>
                        {g.subject}
                      </span>
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
                        onClick={() => void revoke.run(g.id)}
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

        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (subject.trim()) void create.run({ subject: subject.trim(), name: name.trim() });
          }}
        >
          <div className="min-w-64 flex-1 space-y-2">
            <Label htmlFor="oidc-subject">Repository OIDC subject</Label>
            <Input
              id="oidc-subject"
              placeholder="repo:owner/name:ref:refs/heads/main"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
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
          Find the exact subject under the workflow's{" "}
          <code className={monoClass}>Actions OIDC issuer</code>; it differs per ref and event. The
          issuer is fixed to <code className={monoClass}>{GITHUB_ISSUER}</code>.
        </Text>

        {grants?.length ? (
          <Alert>
            <GitBranch />
            <AlertTitle>Workflow snippet</AlertTitle>
            <AlertDescription className="w-full space-y-2">
              <p>No RELEASE_HUB_TOKEN secret needed; just <code className={monoClass}>id-token: write</code>.</p>
              <pre className="overflow-x-auto rounded-lg bg-muted p-3 text-xs">
                <code>{workflowSnippet(app.slug, releaseHubUrl || "https://release-hub.example.com")}</code>
              </pre>
            </AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  );
}

/** "repo:owner/name:…" → "owner/name" for display. */
function subjectRepo(subject: string): string {
  const m = /^repo:([^:]+):/.exec(subject);
  return m ? m[1] : subject;
}
