import { useQuery } from "@tanstack/react-query";
import { ArrowRightLeft, Loader2, RefreshCw } from "lucide-react";
import { Alert, AlertDescription } from "@groot/ui/alert";
import { Button } from "@groot/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@groot/ui/card";
import { SkeletonList } from "@groot/ui/loading-skeleton";
import { StatusBadge } from "@groot/ui/status-badge";
import { monoClass, Text } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { keys, releaseHub } from "@/lib/releaseHub";
import type { VaultCounts, VaultInfo } from "@/lib/types";

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

function describe(c: VaultCounts): string {
  const parts = [];
  if (c.signingKeys) parts.push(plural(c.signingKeys, "signing key"));
  if (c.playAccounts) parts.push(plural(c.playAccounts, "Play service account"));
  return parts.join(" and ") || "nothing";
}

function StateBadge({ info }: { info: VaultInfo }) {
  switch (info.status.state) {
    case "ready":
      return <StatusBadge status="success" label={`Ready · ${info.status.vault ?? "vault"}`} />;
    case "starting":
      return <StatusBadge status="pending" label="Starting" />;
    case "error":
      return <StatusBadge status="error" label="Not usable" />;
    default:
      return <StatusBadge status="cancelled" label="Not set up" />;
  }
}

function Setup() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Set up 1Password</CardTitle>
        <CardDescription>
          The release-hub image runs 1Password Connect itself, so no second service is needed.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <ol className="list-decimal space-y-1 pl-5">
          <li>
            <Text>In 1Password, create a vault for release-hub (for example "release-hub").</Text>
          </li>
          <li>
            <Text>
              On 1Password.com → Developer → Connect servers, create a server with access to only
              that vault and download <code className={monoClass}>1password-credentials.json</code>.
            </Text>
          </li>
          <li>
            <Text>Issue an access token for it with read and write access to that vault only.</Text>
          </li>
          <li>
            <Text>
              Set <code className={monoClass}>OP_CONNECT_CREDENTIALS</code> (the file's contents,
              JSON or base64) and <code className={monoClass}>OP_CONNECT_TOKEN</code>, then
              redeploy.
            </Text>
          </li>
        </ol>
        <Text variant="meta">
          Running Connect elsewhere? Set <code className={monoClass}>OP_CONNECT_HOST</code> instead
          of the credentials. Details in the README, "1Password vault".
        </Text>
      </CardContent>
    </Card>
  );
}

export function VaultSettings() {
  const { data: info, isLoading } = useQuery({
    queryKey: keys.vault,
    queryFn: () => releaseHub.getVault(),
    refetchInterval: (q) => (q.state.data?.status.state === "starting" ? 3000 : false),
  });
  const refresh = useAction(() => releaseHub.getVault(true), {
    errorTitle: "Could not check 1Password",
    fallback: "Unable to check 1Password",
    invalidate: [keys.vault],
  });
  const local = info ? info.local.signingKeys + info.local.playAccounts : 0;
  const move = useAction(() => releaseHub.migrateToVault(), {
    confirm: () => ({
      title: `Move ${plural(local, "key")} to 1Password?`,
      description:
        "Each key is copied into an item in the vault and read back, then release-hub uses the copy. The sealed copy in the database is replaced. CI keeps fetching the same keys.",
      confirmLabel: "Move keys",
    }),
    success: (res) =>
      res.failed.length
        ? {
            title: `Moved ${plural(res.moved.length, "key")}; ${res.failed.length} stayed in the database`,
            description: res.failed.map((f) => `${f.name}: ${f.reason}`).join(" · "),
          }
        : { title: `Moved ${plural(res.moved.length, "key")} to 1Password` },
    errorTitle: "Could not move the keys",
    fallback: "Unable to move the keys",
    invalidate: [keys.vault, keys.apps, keys.playAccounts],
  });

  if (isLoading || !info) {
    return (
      <Card>
        <SkeletonList rows={2} />
      </Card>
    );
  }
  const ready = info.status.state === "ready";
  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <CardTitle>1Password</CardTitle>
            <div className="flex items-center gap-2">
              <StateBadge info={info} />
              {info.configured && (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Check again"
                  title="Check again"
                  disabled={refresh.isPending}
                  onClick={() => void refresh.run()}
                >
                  {refresh.isPending ? <Loader2 className="animate-spin" /> : <RefreshCw />}
                </Button>
              )}
            </div>
          </div>
          <CardDescription>
            {info.mode === "bundled"
              ? "Connect runs inside this container."
              : info.mode === "external"
                ? "Connect runs on OP_CONNECT_HOST."
                : "Signing keys and Play service accounts are sealed in the database."}
            {info.configured && " New keys are stored in the vault."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {info.configured && info.status.detail && (
            <Alert variant={info.status.state === "error" ? "destructive" : "default"}>
              <AlertDescription>{info.status.detail}</AlertDescription>
            </Alert>
          )}
          <div className="grid gap-1">
            <Text>In the database: {describe(info.local)}</Text>
            {info.configured && <Text>In 1Password: {describe(info.inVault)}</Text>}
          </div>
          {info.configured && local > 0 && (
            <div>
              <Button disabled={!ready || move.isPending} onClick={() => void move.run()}>
                {move.isPending ? <Loader2 className="animate-spin" /> : <ArrowRightLeft />}
                Move {plural(local, "key")} to 1Password
              </Button>
            </div>
          )}
          {info.configured && info.pending.some((p) => p.reason) && (
            <Alert variant="destructive">
              <AlertDescription>
                <ul className="space-y-1">
                  {info.pending
                    .filter((p) => p.reason)
                    .map((p) => (
                      <li key={`${p.kind}:${p.name}`}>
                        <Text>
                          <code className={monoClass}>{p.name}</code> stayed in the database:{" "}
                          {p.reason}
                        </Text>
                      </li>
                    ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}
          {info.orphanedKeys > 0 && (
            <p>
              <Text variant="meta">
                Not counted: {plural(info.orphanedKeys, "signing key")} of apps deleted before
                November 15. No app uses them, so they are not moved.
              </Text>
            </p>
          )}
          {info.inVault.signingKeys + info.inVault.playAccounts > 0 && (
            <p>
              <Text variant="meta">
                Release-hub never deletes 1Password items. A replaced or removed key stays in the
                vault, tagged <code className={monoClass}>release-hub/retired</code>.
              </Text>
            </p>
          )}
        </CardContent>
      </Card>
      {!info.configured && <Setup />}
    </>
  );
}
