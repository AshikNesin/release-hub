import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Loader2, Store, Trash2, Upload } from "lucide-react";
import { Badge } from "@groot/ui/badge";
import { Button } from "@groot/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@groot/ui/card";
import { EmptyState } from "@groot/ui/empty-state";
import { Input } from "@groot/ui/input";
import { SkeletonList } from "@groot/ui/loading-skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@groot/ui/table";
import { monoClass, Text } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { releaseHub, keys, usePlayAccounts } from "@/lib/releaseHub";
import { fromNow } from "@/lib/format";
import type { PlayAccount } from "@/lib/types";

export function PlayAccountsSettings() {
  const { data: accounts, isLoading } = usePlayAccounts();
  const [file, setFile] = useState<File | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const add = useAction((f: File) => releaseHub.addPlayAccount(f), {
    success: (res) => ({
      title: "Service account saved",
      description: `${res.serviceAccount}: enable it per app on the app's Configuration tab.`,
    }),
    errorTitle: "Could not save the service account",
    fallback: "Unable to save the service account",
    invalidate: [keys.playAccounts, keys.apps],
    onSuccess: () => {
      setFile(null);
      if (fileInput.current) fileInput.current.value = "";
    },
  });
  const remove = useAction((a: PlayAccount) => releaseHub.deletePlayAccount(a.id), {
    confirm: (a) => ({
      title: `Remove ${a.email}?`,
      description: a.usedBy.length
        ? `Play publishing is turned off for ${a.usedBy.join(", ")}.`
        : "No app publishes through it.",
      confirmLabel: "Remove",
      destructive: true,
    }),
    success: "Service account removed",
    errorTitle: "Could not remove the service account",
    fallback: "Unable to remove the service account",
    invalidate: [keys.playAccounts, keys.apps],
  });

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Add a service account</CardTitle>
          <CardDescription>
            One shared service account with release access covers every app. Re-uploading a key for
            the same email rotates it. Setup guide:{" "}
            <code className={monoClass}>docs/play-internal-testing.md</code>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-wrap items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (file) void add.run(file);
            }}
          >
            <Input
              ref={fileInput}
              type="file"
              accept=".json,application/json"
              aria-label="Service-account JSON key"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              className="max-w-sm"
            />
            <Button type="submit" disabled={!file || add.isPending}>
              {add.isPending ? <Loader2 className="animate-spin" /> : <Upload />}
              Save key
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="overflow-hidden p-0">
        {isLoading ? (
          <SkeletonList rows={2} />
        ) : !accounts?.length ? (
          <EmptyState
            icon={Store}
            title="No service accounts"
            description="Upload a Play Console service-account JSON key to publish .aab uploads to Play."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-5">Service account</TableHead>
                <TableHead>Used by</TableHead>
                <TableHead className="hidden sm:table-cell">Added</TableHead>
                <TableHead className="w-0 pr-5">
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {accounts.map((a) => (
                <TableRow key={a.id}>
                  <TableCell className={`pl-5 ${monoClass}`}>{a.email || `#${a.id}`}</TableCell>
                  <TableCell className="whitespace-normal">
                    {a.usedBy.length ? (
                      <div className="flex flex-wrap gap-1">
                        {a.usedBy.map((u) => (
                          <Badge key={u} variant="secondary" asChild>
                            <Link to={`/apps/${u.split("/")[0]}?tab=configuration`}>{u}</Link>
                          </Badge>
                        ))}
                      </div>
                    ) : (
                      <Text variant="meta">No apps yet</Text>
                    )}
                  </TableCell>
                  <TableCell className="hidden text-muted-foreground sm:table-cell">
                    {fromNow(a.createdAt)}
                  </TableCell>
                  <TableCell className="pr-5">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`Remove ${a.email}`}
                      title="Remove"
                      disabled={remove.isPending}
                      onClick={() => void remove.run(a)}
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
