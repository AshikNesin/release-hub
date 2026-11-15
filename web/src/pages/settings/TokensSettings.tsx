import { useState } from "react";
import { KeyRound, Loader2, Plus, Trash2 } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@groot/ui/alert";
import { Button } from "@groot/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@groot/ui/card";
import { EmptyState } from "@groot/ui/empty-state";
import { Input } from "@groot/ui/input";
import { SkeletonList } from "@groot/ui/loading-skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@groot/ui/table";
import { monoClass, Text } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { CopyField } from "@/components/copy";
import { releaseHub, keys, useTokens } from "@/lib/releaseHub";
import { formatDateTime, fromNow } from "@/lib/format";
import type { ApiToken } from "@/lib/types";

export function TokensSettings() {
  const { data: tokens, isLoading } = useTokens();
  const [name, setName] = useState("");
  const [fresh, setFresh] = useState<{ name: string; token: string } | null>(null);

  const create = useAction((n: string) => releaseHub.createToken(n), {
    success: (t) => ({
      title: "Token created",
      description: `Copy “${t.name}” now; it is shown only once.`,
    }),
    errorTitle: "Could not create the token",
    fallback: "Unable to create the token",
    invalidate: [keys.tokens],
    onSuccess: (t) => {
      setFresh(t);
      setName("");
    },
  });
  const revoke = useAction((t: ApiToken) => releaseHub.deleteToken(t.id), {
    confirm: (t) => ({
      title: `Revoke “${t.name}”?`,
      description:
        "Anything still using this token (CI, deploy scripts) gets 401 from the next request.",
      confirmLabel: "Revoke token",
      destructive: true,
    }),
    success: (_, t) => `Revoked “${t.name}”`,
    errorTitle: "Could not revoke the token",
    fallback: "Unable to revoke the token",
    invalidate: [keys.tokens],
  });

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Create a token</CardTitle>
          <CardDescription>
            Bearer tokens for CI and deploy scripts:{" "}
            <code className={monoClass}>Authorization: Bearer rh_…</code>. Stored hashed; a token
            also unlocks signing keys, so keep them scoped to trusted builders.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            className="flex flex-wrap gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (name.trim()) void create.run(name.trim());
            }}
          >
            <Input
              aria-label="Token name"
              placeholder="e.g. github-actions"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="max-w-xs"
            />
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              {create.isPending ? <Loader2 className="animate-spin" /> : <Plus />}
              Create token
            </Button>
          </form>
          {fresh && (
            <Alert>
              <KeyRound />
              <AlertTitle>New token “{fresh.name}”; copy it now</AlertTitle>
              <AlertDescription className="w-full space-y-2">
                <p>It won't be shown again. Store it as a CI secret, e.g. RELEASE_HUB_TOKEN.</p>
                <CopyField value={fresh.token} what="Token" className="w-full" />
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      <Card className="overflow-hidden p-0">
        {isLoading ? (
          <SkeletonList rows={3} />
        ) : !tokens?.length ? (
          <EmptyState
            icon={KeyRound}
            title="No tokens yet"
            description="Create one for CI or a deploy script."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-5">Name</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Last used</TableHead>
                <TableHead className="w-0 pr-5">
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((t) => (
                <TableRow key={t.id}>
                  <TableCell className="pl-5 font-medium">{t.name}</TableCell>
                  <TableCell className="text-muted-foreground" title={formatDateTime(t.createdAt)}>
                    {fromNow(t.createdAt)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {t.lastUsedAt ? (
                      <span title={formatDateTime(t.lastUsedAt)}>{fromNow(t.lastUsedAt)}</span>
                    ) : (
                      <Text variant="meta">Never</Text>
                    )}
                  </TableCell>
                  <TableCell className="pr-5">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`Revoke ${t.name}`}
                      title="Revoke"
                      disabled={revoke.isPending}
                      onClick={() => void revoke.run(t)}
                      className="text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </>
  );
}
