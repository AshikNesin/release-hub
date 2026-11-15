import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@groot/shell/lib/api";
import type {
  ApiToken,
  AppDetail,
  AppSummary,
  HubSettings,
  Platform,
  PlayAccount,
  PlayTrack,
  Preflight,
  SettingsPatch,
} from "./types";

/** Query keys, one place: actions invalidate these. */
export const keys = {
  apps: ["apps"] as const,
  app: (slug: string) => ["apps", slug] as const,
  settings: ["settings"] as const,
  tokens: ["tokens"] as const,
  playAccounts: ["play-accounts"] as const,
  tracks: (slug: string, platform: Platform) => ["apps", slug, platform, "tracks"] as const,
};

const plat = (slug: string, platform: Platform) => `/apps/${encodeURIComponent(slug)}/${platform}`;

/** Every call the UI makes, typed. The shell's apiClient unwraps the envelope. */
export const hub = {
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

  getSettings: () => apiClient.get<HubSettings>("/settings"),
  patchSettings: (patch: SettingsPatch) => apiClient.patch<HubSettings>("/settings", patch),
  listTokens: () => apiClient.get<ApiToken[]>("/tokens"),
  createToken: (name: string) =>
    apiClient.post<{ name: string; token: string }>("/tokens", { name }),
  deleteToken: (id: number) => apiClient.delete(`/tokens/${id}`),
  listPlayAccounts: () => apiClient.get<PlayAccount[]>("/play-accounts"),
  addPlayAccount: (file: File) => {
    const form = new FormData();
    form.set("file", file);
    return apiClient.postForm<{ id: number; serviceAccount: string }>("/play-accounts", form);
  },
  deletePlayAccount: (id: number) => apiClient.delete(`/play-accounts/${id}`),
};

export const useApps = () => useQuery({ queryKey: keys.apps, queryFn: hub.listApps });
export const useApp = (slug: string) =>
  useQuery({ queryKey: keys.app(slug), queryFn: () => hub.getApp(slug) });
export const useSettings = () => useQuery({ queryKey: keys.settings, queryFn: hub.getSettings });
export const useTokens = () => useQuery({ queryKey: keys.tokens, queryFn: hub.listTokens });
export const usePlayAccounts = () =>
  useQuery({ queryKey: keys.playAccounts, queryFn: hub.listPlayAccounts });
