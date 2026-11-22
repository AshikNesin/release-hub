import { Link } from "react-router-dom";
import { ShieldCheck, Trash2, TriangleAlert } from "lucide-react";
import { Badge } from "@groot/ui/badge";
import { Button } from "@groot/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@groot/ui/card";
import { EmptyState } from "@groot/ui/empty-state";
import { SkeletonList } from "@groot/ui/loading-skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@groot/ui/table";
import { Text } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { releaseHub, keys, useAllOidcGrants } from "@/lib/releaseHub";
import { fromNow } from "@/lib/format";
import type { OidcGrantAll } from "@/lib/types";

/**
 * Every keyless (OIDC) grant across apps, in one place. A grant whose app no
 * longer exists is stale — it can neither publish anything nor be reached
 * from the app's page, so it can be removed here.
 */
export function KeylessGrantsSettings() {
  const { data: grants, isLoading } = useAllOidcGrants();
  const stale = grants?.filter((g) => g.app === null) ?? [];

  const revoke = useAction((g: OidcGrantAll) => releaseHub.deleteOidcGrant(g.id, g.repositoryId !== null), {
    confirm: (g) => ({
      title: `Remove the grant for ${g.name || subjectRepo(g.subject)}?`,
      description: g.app
        ? `That repository loses keyless access to ${g.app} immediately; its minted tokens stop working.`
        : "This grant is stale (its app was deleted). Its minted tokens already stopped working.",
      confirmLabel: "Remove grant",
      destructive: true,
    }),
    success: (_, g) => `Removed the grant for ${g.name || subjectRepo(g.subject)}`,
    errorTitle: "Could not remove the grant",
    fallback: "Unable to remove the grant",
    invalidate: [keys.allOidcGrants, keys.apps],
  });

  return (
    <Card className="overflow-hidden p-0">
      {isLoading ? (
        <SkeletonList rows={2} />
      ) : !grants?.length ? (
        <EmptyState
          icon={ShieldCheck}
          title="No keyless grants"
          description="Grants are created per app: App → Configuration → Keyless publishing."
        />
      ) : (
        <>
          <CardHeader className="border-b">
            <CardTitle>
              {stale.length > 0 ? (
                <span className="flex items-center gap-2">
                  <TriangleAlert className="size-4 text-warning" />
                  {stale.length} stale grant{stale.length === 1 ? "" : "s"}
                </span>
              ) : (
                <span className="flex items-center gap-2">
                  <ShieldCheck className="size-4 text-success" />
                  All grants healthy
                </span>
              )}
            </CardTitle>
            <CardDescription>
              Repositories that publish keyless. Removing a grant kills its minted tokens
              immediately.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-5">Repository</TableHead>
                  <TableHead>App</TableHead>
                  <TableHead className="hidden sm:table-cell">Last used</TableHead>
                  <TableHead className="w-0 pr-5">
                    <span className="sr-only">Actions</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {grants.map((g) => (
                  <TableRow key={g.id}>
                    <TableCell className="pl-5 font-medium">{g.name || subjectRepo(g.subject)}</TableCell>
                    <TableCell className="whitespace-normal">
                      {g.app ? (
                        <Badge variant="secondary" asChild>
                          <Link to={`/apps/${g.app}?tab=configuration`}>{g.app}</Link>
                        </Badge>
                      ) : (
                        <Badge variant="destructive">stale — app deleted</Badge>
                      )}
                    </TableCell>
                    <TableCell className="hidden text-muted-foreground sm:table-cell">
                      {g.lastUsedAt ? (
                        <span title={g.lastUsedAt}>{fromNow(g.lastUsedAt)}</span>
                      ) : (
                        <Text variant="meta">Never</Text>
                      )}
                    </TableCell>
                    <TableCell className="pr-5">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`Remove the grant for ${g.name || subjectRepo(g.subject)}`}
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
          </CardContent>
        </>
      )}
    </Card>
  );
}

/** "repo:owner@ID/name@ID:…" → "owner/name" for display (IDs stripped). */
function subjectRepo(subject: string): string {
  const m = /^repo:([^:]+)/.exec(subject);
  return m ? m[1].replace(/@\d+/g, "") : subject;
}
