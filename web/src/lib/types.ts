// Wire types of the Go UI API (/api/v1, srv/api_v1_handlers.go). Change both
// sides together. Dates are RFC 3339 strings.

export type Platform = "android" | "ios";

/** Release hub channels; legacy spellings (open, closed:<name>) can still appear on old rows. */
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

export interface ReleaseHubSettings {
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

/** Keyless publishing: one OIDC identity (a GitHub repo) → one app. */
export interface OidcGrant {
  id: number;
  issuer: string;
  subject: string;
  name: string;
  createdAt: string;
  lastUsedAt: string | null;
  /** Set on a repository grant (made by Connect): bound by GitHub repo id, not subject. */
  repositoryId: number | null;
  /** The ref a repository grant may publish from, e.g. refs/heads/main. */
  ref?: string;
}

/** Every grant across apps; `app` is null when its app was deleted (stale). */
export interface OidcGrantAll extends OidcGrant {
  app: string | null;
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

/** 1Password vault (srv/vault_migrate.go): where stored keys live. */
export interface VaultCounts {
  signingKeys: number;
  playAccounts: number;
}

export interface VaultInfo {
  configured: boolean;
  /** bundled = Connect runs inside the release-hub container; external = OP_CONNECT_HOST. */
  mode: "off" | "bundled" | "external";
  status: {
    state: "off" | "starting" | "ready" | "error";
    code?: string;
    detail: string;
    vault?: string;
  };
  /** Keys still sealed in the database. */
  local: VaultCounts;
  inVault: VaultCounts;
  /** Keys still in the database, with why the last move left each (if it did). */
  pending: { kind: "signing" | "play"; name: string; reason?: string }[];
  /** Signing keys of apps deleted before 2026-11-15, which no app uses. */
  orphanedKeys: number;
}

export interface VaultMoveResult {
  moved: string[];
  failed: { kind: "signing" | "play"; name: string; reason: string }[];
}

/** This release hub's own GitHub App (Settings → GitHub). */
export interface GithubAppInfo {
  slug: string;
  name: string;
  owner: string;
  htmlUrl: string;
  installUrl: string;
  createdAt: string;
}

export interface GithubSettings {
  /** 1Password or RELEASE_HUB_SECRET_KEY is set, so the app's key can be stored. */
  canStoreKey: boolean;
  baseUrl: string;
  /** RELEASE_HUB_BASE_URL is a public https URL GitHub can redirect to. */
  baseUrlOk: boolean;
  baseUrlProblem?: string;
  app: GithubAppInfo | null;
  /** The repo Actions variables Connect sets. */
  variables: string[];
}

/** A repository the GitHub App's installations can reach. */
export interface GithubRepository {
  id: number;
  fullName: string;
  ownerId: number;
  defaultBranch: string;
  private: boolean;
  installationId: number;
  /** The app this repo already publishes (one repo → one app), or null. */
  app: string | null;
}

export interface GithubVariable {
  name: string;
  value: string;
  /** The repo's current, different value (only on a conflict). */
  current?: string;
}

export type GithubConnectResult =
  | { connected: true; repository: string; ref: string; variables: GithubVariable[] }
  | { connected: false; repository: string; conflicts: GithubVariable[] };
