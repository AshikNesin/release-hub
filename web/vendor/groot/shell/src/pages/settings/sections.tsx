// RELEASE-HUB OVERRIDE of @groot/shell/pages/settings/sections (skipped by
// scripts/sync-groot.sh). Groot's default sections (account, passkeys, JSON
// app settings) need groot's backend; release-hub passes its own sections to
// <Settings sections={…}> (web/src/pages/settings), so the default is empty.
import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

/** One searchable setting inside a section (it links to its section). */
export interface SettingsItem {
  label: string;
  description?: string;
  keywords?: string[];
}

/** One sidebar entry and sub-route of the settings page (`/settings/:id`). */
export interface SettingsSection {
  /** URL segment, e.g. `appearance` for `/settings/appearance`. */
  id: string;
  label: string;
  /** Sidebar group heading; sections with the same group are listed together. */
  group: string;
  icon?: LucideIcon;
  /** Shown under the section title and in search results. */
  description?: string;
  /** Extra search terms, e.g. `dark` for Appearance. */
  keywords?: string[];
  items?: SettingsItem[];
  element: ReactNode;
  /** Use the wider content column (master/detail editors). */
  wide?: boolean;
}

export const DEFAULT_SETTINGS_SECTIONS: SettingsSection[] = [];
