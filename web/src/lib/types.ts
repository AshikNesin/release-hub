// Wire types of the Go UI API (/api/v1, srv/api_v1_handlers.go). Change both
// sides together. Dates are RFC 3339 strings.

export type Platform = "android" | "ios";

/** Hub channels; legacy spellings (open, closed:<name>) can still appear on old rows. */
export type Channel = "direct" | "internal" | "alpha" | "beta" | "public" | (string & {});

/** Play push outcome per release: direct/.apk rows stay `pending`. */
export type PlayStatus = "pending" | "ok" | "error";

export interface Release {
  versionCode: number;
  versionName: string;
  channel: Channel;
  notes: string;
  url: string;
  sizeBytes: number;
  sha256: string;
  createdAt: string;
  playStatus: PlayStatus;
  playError?: string;
  playRelease?: string;
}

export interface PlatformSummary {
  platform: Platform;
  packageName: string;
  playEnabled: boolean;
  hasSigningKey: boolean;
  releaseCount: number;
  latest: Release | null;
}

export interface AppSummary {
  slug: string;
  createdAt: string;
  platforms: PlatformSummary[];
}

export interface ShareLink {
  label: Channel;
  url: string;
}

export interface PlatformDetail {
  platform: Platform;
  packageName: string;
  manifestUrl: string;
  uploadUrl: string;
  shares: ShareLink[];
  betaTesters: string[];
  testerEmails: string[];
  retention: { override: number | null; effective: number; label: string };
  signing: { hasKey: boolean; sha256: string; alias: string };
  play: { enabled: boolean; accountId: number; email: string };
  releases: Release[];
}

export interface AppDetail {
  slug: string;
  createdAt: string;
  suggestedPackage: string;
  platforms: PlatformDetail[];
  playAccounts: { id: number; email: string }[];
}

export interface CertField {
  field: string;
  label: string;
  hint: string;
  placeholder: string;
  value: string;
}

export interface HubSettings {
  bundlePrefix: string;
  playTesters: string;
  playTesterEmails: string;
  pruneKeep: string;
  certFields: CertField[];
}

/** PATCH body: only the keys present are saved. */
export type SettingsPatch = Partial<
  Record<
    | "bundle_prefix"
    | "play_testers"
    | "play_tester_emails"
    | "prune_keep"
    | "org"
    | "ou"
    | "locality"
    | "state"
    | "country",
    string
  >
>;

export interface ApiToken {
  id: number;
  name: string;
  createdAt: string;
  lastUsedAt: string | null;
}

export interface PlayAccount {
  id: number;
  email: string;
  createdAt: string;
  /** "slug/platform" entries publishing through this account. */
  usedBy: string[];
}

export interface Preflight {
  ok: boolean;
  detail: string;
}

export interface PlayTrack {
  track: string;
  isClosed: boolean;
  channel: string;
  releases: string[] | null;
}
