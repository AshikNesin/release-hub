import { Apple, Smartphone } from "lucide-react";
import { Badge } from "@groot/ui/badge";
import { StatusBadge } from "@groot/ui/status-badge";
import { cn } from "@groot/ui/lib/utils";
import { platformLabel } from "@/lib/format";
import type { Channel, Platform, Release } from "@/lib/types";

/** Where each release hub channel lands, for tooltips and the dot colour. */
export const CHANNELS: Record<string, { label: string; dot: string; where: string }> = {
  direct: {
    label: "direct",
    dot: "bg-muted-foreground",
    where: "Release hub only, direct APK + update manifest",
  },
  internal: { label: "internal", dot: "bg-info", where: "Play internal testing" },
  alpha: { label: "alpha", dot: "bg-warning", where: "Play closed testing (alpha track)" },
  beta: { label: "beta", dot: "bg-primary", where: "Play open testing" },
  public: { label: "public", dot: "bg-success", where: "Play production" },
};

export function ChannelBadge({ channel, className }: { channel: Channel; className?: string }) {
  const known = CHANNELS[channel];
  return (
    <Badge variant="outline" className={cn("gap-1.5 font-mono", className)} title={known?.where}>
      <span className={cn("size-1.5 rounded-full", known?.dot ?? "bg-muted-foreground")} />
      {channel}
    </Badge>
  );
}

export const isPlayChannel = (channel: Channel) => channel !== "direct" && !!channel;

/** Play outcome of one release; nothing for release-hub-only (direct) releases. */
export function PlayStatus({ release }: { release: Release }) {
  if (!isPlayChannel(release.channel)) return null;
  switch (release.playStatus) {
    case "ok":
      return <StatusBadge status="success" label="On Play" />;
    case "error":
      return <StatusBadge status="error" label="Action needed" />;
    default:
      return <StatusBadge status="pending" label="Unverified" />;
  }
}

export function PlatformIcon({ platform, className }: { platform: Platform; className?: string }) {
  const Icon = platform === "ios" ? Apple : Smartphone;
  return <Icon className={cn("size-4", className)} aria-hidden />;
}

export function PlatformBadge({ platform }: { platform: Platform }) {
  return (
    <Badge variant="secondary" className="gap-1">
      <PlatformIcon platform={platform} className="size-3" />
      {platformLabel(platform)}
    </Badge>
  );
}
