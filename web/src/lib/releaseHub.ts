import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@groot/shell/lib/api";
import type {
  ApiToken,
  AppDetail,
  AppSummary,
  GithubConnectResult,
  GithubRepository,
  GithubSettings,
  ReleaseHubSettings,
  OidcGrant,
  OidcGrantAll,
  Platform,
  PlayAccount,
  PlayTrack,
  Preflight,
  SettingsPatch,
  VaultInfo,
  VaultMoveResult,
} from "./types";

/** Query keys, one place: actions invalidate these. */
export const keys = {
  apps: ["apps"] as const,
  app: (slug: string) => ["apps", slug] as const,
  settings: ["settings"] as const,
  tokens: ["tokens"] as const,
  playAccounts: ["play-accounts"] as const,
  oidcGrants: (slug: string) => ["apps", slug, "oidc-grants"] as const,
  allOidcGrants: ["oidc-grants"] as const,
  vault: ["vault"] as const,
  github: ["github"] as const,
  githubRepositories: ["github", "repositories"] as const,
  tracks: (slug: string, platform: Platform) => ["apps", slug, platform, "tracks"] as const,
};

const plat = (slug: string, platform: Platform) => `/apps/${encodeURIComponent(slug)}/${platform}`;

/** Every call the UI makes, typed. The shell's apiClient unwraps the envelope. */
export const releaseHub = {
  listApps: () => apiClient.get<AppSummary[]>("/apps"),
  createApp: (slug: string) => apiClient.post<{ slug: string }>("/apps", { slug }),
  getApp: (slug: string) => apiClient.get<AppDetail>(`/apps/${encodeURIComponent(slug)}`),
  deleteApp: (slug: string) => apiClient.delete<{ deleted: string }>(`/apps/${encodeURIComponent(slug)}`),
  addPlatform: (slug: string, v: { platform: Platform; packageName: string }) =>
    apiClient.post<{ signingKey?: string; signingSha256?: string }>(
      `/apps/${encodeURIComponent(slug)}/platforms`,
      v,
    ),
  deleteRelease: (slug: string, platform: Platform, versionCode: number) =>
    apiClient.delete(`${plat(slug, platform)}/releases/${versionCode}`),
  setRetention: (slug: string, platform: Platform, keep: number | null) =>
    apiClient.put<{ override: number | null; effective: number; label: string }>(
      `${plat(slug, platform)}/retention`,
      { keep },
    ),
  /** Enable with a shared account id or a new service-account JSON file, or disable. */
  setPlay: (
    slug: string,
    platform: Platform,
    v: { enable: false } | { account: number } | { file: File },
  ) => {
    const form = new FormData();
    if ("enable" in v) form.set("enable", "false");
    else if ("account" in v) form.set("account", String(v.account));
    else form.set("file", v.file);
    return apiClient.postForm<{ playEnabled: boolean; serviceAccount?: string }>(
      `${plat(slug, platform)}/play`,
      form,
    );
  },
  preflight: (slug: string, platform: Platform) =>
    apiClient.get<Preflight>(`${plat(slug, platform)}/play/preflight`),
  tracks: (slug: string, platform: Platform) =>
    apiClient.get<PlayTrack[]>(`${plat(slug, platform)}/tracks`),
  inviteTesters: (slug: string, platform: Platform) =>
    apiClient.post<{ track: string; groups: string[] }>(`${plat(slug, platform)}/testers`, {
      channel: "alpha",
    }),

  getSettings: () => apiClient.get<ReleaseHubSettings>("/settings"),
  patchSettings: (patch: SettingsPatch) => apiClient.patch<ReleaseHubSettings>("/settings", patch),
  listTokens: () => apiClient.get<ApiToken[]>("/tokens"),
  createToken: (name: string) =>
    apiClient.post<{ name: string; token: string }>("/tokens", { name }),
  deleteToken: (id: number) => apiClient.delete(`/tokens/${id}`),
  listOidcGrants: (slug: string) =>
    apiClient.get<OidcGrant[]>(`/apps/${encodeURIComponent(slug)}/oidc/grants`),
  listAllOidcGrants: () => apiClient.get<OidcGrantAll[]>("/oidc/grants"),
  createOidcGrant: (
    slug: string,
    v: { issuer: string; subject: string; name?: string },
  ) =>
    apiClient.post<OidcGrant[]>(
      `/apps/${encodeURIComponent(slug)}/oidc/grants`,
      v,
    ),
  /** variables: also remove the RELEASE_HUB_* repo variables Connect set (repository grants). */
  deleteOidcGrant: (id: number, variables = false) =>
    apiClient.delete<{ ok: true; variablesRemoved?: boolean; variablesError?: string }>(
      `/oidc/grants/${id}${variables ? "?variables=1" : ""}`,
    ),
  getGithub: () => apiClient.get<GithubSettings>("/github"),
  githubRepositories: () => apiClient.get<GithubRepository[]>("/github/repositories"),
  githubManifest: (organization: string) =>
    apiClient.post<{ url: string; manifest: object }>(
      "/github/manifest",
      organization ? { organization } : {},
    ),
  githubInstall: () => apiClient.post<{ url: string }>("/github/install", {}),
  githubDisconnect: () => apiClient.post<{ disconnected: true }>("/github/disconnect", {}),
  githubConnect: (slug: string, repository: string, overwrite = false) =>
    apiClient.post<GithubConnectResult>(`/apps/${encodeURIComponent(slug)}/github/connect`, {
      repository,
      overwrite,
    }),
  listPlayAccounts: () => apiClient.get<PlayAccount[]>("/play-accounts"),
  addPlayAccount: (file: File) => {
    const form = new FormData();
    form.set("file", file);
    return apiClient.postForm<{ id: number; serviceAccount: string }>("/play-accounts", form);
  },
  deletePlayAccount: (id: number) => apiClient.delete(`/play-accounts/${id}`),
  getVault: (refresh = false) => apiClient.get<VaultInfo>(refresh ? "/vault?refresh=1" : "/vault"),
  migrateToVault: () => apiClient.post<VaultMoveResult>("/vault/migrate", {}),
};

export const useApps = () => useQuery({ queryKey: keys.apps, queryFn: releaseHub.listApps });
export const useApp = (slug: string) =>
  useQuery({ queryKey: keys.app(slug), queryFn: () => releaseHub.getApp(slug) });
export const useSettings = () => useQuery({ queryKey: keys.settings, queryFn: releaseHub.getSettings });
export const useTokens = () => useQuery({ queryKey: keys.tokens, queryFn: releaseHub.listTokens });
export const useOidcGrants = (slug: string) =>
  useQuery({ queryKey: keys.oidcGrants(slug), queryFn: () => releaseHub.listOidcGrants(slug) });
export const useAllOidcGrants = () =>
  useQuery({ queryKey: keys.allOidcGrants, queryFn: releaseHub.listAllOidcGrants });
export const usePlayAccounts = () =>
  useQuery({ queryKey: keys.playAccounts, queryFn: releaseHub.listPlayAccounts });
export const useGithub = () => useQuery({ queryKey: keys.github, queryFn: releaseHub.getGithub });
export const useGithubRepositories = (enabled = true) =>
  useQuery({
    queryKey: keys.githubRepositories,
    queryFn: releaseHub.githubRepositories,
    enabled,
    retry: false,
  });
