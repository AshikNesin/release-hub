import { useEffect, useState } from "react";
import { Monitor, Moon, Sun, type LucideIcon } from "lucide-react";

const THEME_STORAGE_KEY = "groot.theme";

export type Theme = "light" | "dark" | "system";
export type ResolvedTheme = Exclude<Theme, "system">;

/** The theme choices every picker offers (account menu, ⌘K, settings). */
export const THEME_OPTIONS: ReadonlyArray<{ value: Theme; label: string; icon: LucideIcon }> = [
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
  { value: "system", label: "System", icon: Monitor },
];

let defaultTheme: Theme = "dark";
let systemThemeQuery: MediaQueryList | undefined;
let systemThemeListener: (() => void) | undefined;
const listeners = new Set<() => void>();

function canUseDom(): boolean {
  return typeof window !== "undefined" && typeof document !== "undefined";
}

function readStoredTheme(): Theme | undefined {
  if (!canUseDom()) return undefined;

  try {
    const storedTheme = window.localStorage.getItem(THEME_STORAGE_KEY);
    return storedTheme && isTheme(storedTheme) ? storedTheme : undefined;
  } catch {
    return undefined;
  }
}

function writeStoredTheme(theme: Theme): void {
  if (!canUseDom()) return;

  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // Theme selection still applies when storage is unavailable.
  }
}

function notifyListeners(): void {
  for (const listener of listeners) listener();
}

function stopListeningToSystemTheme(): void {
  if (systemThemeQuery && systemThemeListener) {
    systemThemeQuery.removeEventListener("change", systemThemeListener);
  }
  systemThemeQuery = undefined;
  systemThemeListener = undefined;
}

function syncSystemThemeListener(theme: Theme): void {
  if (!canUseDom()) return;

  if (theme !== "system" || typeof window.matchMedia !== "function") {
    stopListeningToSystemTheme();
    return;
  }

  if (systemThemeQuery) return;

  systemThemeQuery = window.matchMedia("(prefers-color-scheme: dark)");
  systemThemeListener = () => {
    applyThemeClass("system");
    notifyListeners();
  };
  systemThemeQuery.addEventListener("change", systemThemeListener);
}

function applyThemeClass(theme: Theme): void {
  if (!canUseDom()) return;
  document.documentElement.classList.toggle("dark", resolveTheme(theme) === "dark");
}

function applyTheme(theme: Theme): void {
  applyThemeClass(theme);
  syncSystemThemeListener(theme);
}

export function isTheme(value: string): value is Theme {
  return value === "light" || value === "dark" || value === "system";
}

export function setDefaultTheme(theme: Theme): void {
  defaultTheme = theme;
}

export function getTheme(): Theme {
  return readStoredTheme() ?? defaultTheme;
}

export function resolveTheme(theme: Theme): ResolvedTheme {
  if (theme !== "system") return theme;
  return canUseDom() &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

/**
 * Apply the persisted theme before the first React render.
 *
 * Apps may call `setDefaultTheme()` before this to override the shell's dark
 * default while retaining each user's stored preference.
 */
export function initTheme(): Theme {
  const theme = getTheme();
  applyTheme(theme);
  return theme;
}

export function setTheme(theme: Theme): void {
  writeStoredTheme(theme);
  applyTheme(theme);
  notifyListeners();
}

export function useTheme(): {
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: Theme) => void;
} {
  const [theme, setCurrentTheme] = useState(getTheme);
  const [resolvedTheme, setResolvedTheme] = useState(() => resolveTheme(getTheme()));

  useEffect(() => {
    initTheme();

    const syncTheme = () => {
      const currentTheme = getTheme();
      setCurrentTheme(currentTheme);
      setResolvedTheme(resolveTheme(currentTheme));
    };

    syncTheme();
    listeners.add(syncTheme);
    return () => {
      listeners.delete(syncTheme);
    };
  }, []);

  return { theme, resolvedTheme, setTheme };
}
