import { Link } from "react-router-dom";
import { ExternalLink, Loader2, Send, Users } from "lucide-react";
import { Badge } from "@groot/ui/badge";
import { Button } from "@groot/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@groot/ui/card";
import { metaLabelClass, Text } from "@groot/ui/typography";
import { useAction } from "@groot/shell/lib/feedback";
import { CHANNELS, ChannelBadge } from "@/components/badges";
import { CopyField } from "@/components/copy";
import { hub, keys } from "@/lib/hub";
import type { PlatformDetail } from "@/lib/types";

export function DistributionPanel({ slug, platform }: { slug: string; platform: PlatformDetail }) {
  const invite = useAction(() => hub.inviteTesters(slug, platform.platform), {
    confirm: {
      title: "Invite testers to alpha?",
      description:
        "The alpha (closed testing) track's tester list is replaced with the groups saved in Settings. Testers added directly in Play Console are removed.",
      confirmLabel: "Replace and invite",
    },
    success: (res) => ({
      title: "Testers invited",
      description: `${res.groups.length} group(s) on the ${res.track} track.`,
    }),
    errorTitle: "Inviting testers failed",
    fallback: "Unable to update the alpha track",
    invalidate: [keys.app(slug)],
  });

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Share links</CardTitle>
          <CardDescription>
            Stable URLs that always resolve to the newest release on each channel — safe to put in a
            README or send to testers.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {platform.shares.map((s) => (
            <div key={s.label} className="grid gap-2 sm:grid-cols-[8rem_1fr] sm:items-center">
              <div className="flex flex-col gap-0.5">
                <ChannelBadge channel={s.label} className="w-fit" />
                <Text variant="meta" className="hidden sm:block">
                  {CHANNELS[s.label]?.where.replace(" — direct APK + update manifest", "")}
                </Text>
              </div>
              <CopyField value={s.url} what={`${s.label} link`} />
            </div>
          ))}
          {platform.platform === "android" && !platform.play.enabled && (
            <Text variant="meta" className="block pt-1">
              Play testing and store links appear once Google Play publishing is enabled on the{" "}
              <Link
                to={`/apps/${slug}?tab=configuration`}
                className="underline underline-offset-2 hover:text-foreground"
              >
                Configuration
              </Link>{" "}
              tab.
            </Text>
          )}
        </CardContent>
      </Card>

      {platform.play.enabled && (
        <Card>
          <CardHeader>
            <CardTitle>Testers</CardTitle>
            <CardDescription>
              Google Groups go to the alpha (closed testing) track through the API. Individual
              emails can't be pushed by Google's API — paste them into Play Console's tester lists.
            </CardDescription>
            <CardAction>
              <Button
                variant="outline"
                size="sm"
                onClick={() => void invite.run()}
                disabled={invite.isPending || platform.betaTesters.length === 0}
              >
                {invite.isPending ? <Loader2 className="animate-spin" /> : <Send />}
                Invite testers to alpha
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent className="space-y-5">
            <div>
              <p className={metaLabelClass}>Google Groups</p>
              {platform.betaTesters.length ? (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {platform.betaTesters.map((g) => (
                    <Badge key={g} variant="secondary" className="font-mono">
                      <Users />
                      {g}
                    </Badge>
                  ))}
                </div>
              ) : (
                <Text variant="meta" className="mt-1 block">
                  None yet —{" "}
                  <Link
                    to="/settings/testers"
                    className="underline underline-offset-2 hover:text-foreground"
                  >
                    add Google Group addresses in Settings
                  </Link>
                  .
                </Text>
              )}
            </div>
            <div>
              <p className={metaLabelClass}>Individual emails</p>
              {platform.testerEmails.length ? (
                <CopyField
                  value={platform.testerEmails.join(", ")}
                  what="Tester emails"
                  className="mt-2"
                />
              ) : (
                <Text variant="meta" className="mt-1 block">
                  None saved. Keep them in{" "}
                  <Link
                    to="/settings/testers"
                    className="underline underline-offset-2 hover:text-foreground"
                  >
                    Settings → Testers
                  </Link>{" "}
                  as the copy-paste source for Play Console.
                </Text>
              )}
            </div>
            <Button variant="link" size="sm" asChild className="h-auto px-0">
              <a
                href="https://play.google.com/console/u/0/developers"
                target="_blank"
                rel="noreferrer"
              >
                Open Play Console
                <ExternalLink />
              </a>
            </Button>
          </CardContent>
        </Card>
      )}
    </>
  );
}
