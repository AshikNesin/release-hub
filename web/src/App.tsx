import { lazy, Suspense, useEffect, useMemo, type ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { Box, LayoutGrid, PackageOpen, Settings as SettingsIcon } from "lucide-react";
import { GrootUIProvider } from "@groot/ui/provider";
import { Layout } from "@groot/shell/components/Layout";
import { ProtectedRoute } from "@groot/shell/components/ProtectedRoute";
import type { CommandGroupEntry } from "@groot/shell/components/CommandPalette";
import {
  type NavDestination,
  type ShellConfig,
  ShellProvider,
} from "@groot/shell/lib/shell-config";
import { selectAuthStatus, useAuthStore } from "@groot/shell/store/auth";
import { Login } from "@/pages/auth/Login";
import { Setup } from "@/pages/auth/Setup";
import { Apps } from "@/pages/apps/Apps";
import { LoadingSpinner } from "@groot/ui/loading-spinner";

// Route-level code splitting: the list page ships in the main chunk.
const AppDetailPage = lazy(() =>
  import("@/pages/apps/AppDetail").then((m) => ({ default: m.AppDetailPage })),
);
const ReleaseHubSettingsPage = lazy(() =>
  import("@/pages/settings/ReleaseHubSettings").then((m) => ({ default: m.ReleaseHubSettingsPage })),
);

const pageFallback = (
  <div className="flex min-h-[50dvh] items-center justify-center">
    <LoadingSpinner size="md" />
  </div>
);
import { useApps } from "@/lib/releaseHub";

/** Every page with a place in the navigation: a route, a sidebar row or account-menu row, and a ⌘K entry. */
const PAGES: Array<NavDestination & { element: ReactNode; path?: string }> = [
  { label: "Apps", icon: LayoutGrid, to: "/apps", primary: true, element: <Apps /> },
  {
    label: "Settings",
    icon: SettingsIcon,
    to: "/settings",
    path: "/settings/*",
    fullPage: true,
    shortcut: "mod+,",
    element: <ReleaseHubSettingsPage />,
  },
];

const SHELL_CONFIG: ShellConfig = {
  brand: { label: "release-hub", icon: PackageOpen, to: "/apps" },
  nav: PAGES,
};

/** The shell layout plus a ⌘K group that jumps straight to any app. */
function AppLayout() {
  const navigate = useNavigate();
  const signedIn = useAuthStore((s) => selectAuthStatus(s).status === "signedIn");
  const { data: apps } = useApps();
  const groups = useMemo<CommandGroupEntry[]>(() => {
    if (!signedIn || !apps?.length) return [];
    return [
      {
        heading: "Apps",
        items: apps.map((app) => ({
          id: `app:${app.slug}`,
          label: app.slug,
          icon: Box,
          description: app.platforms.map((p) => p.packageName).join(" · ") || "no platform yet",
          keywords: app.platforms.map((p) => p.platform),
          action: `Open ${app.slug}`,
          onSelect: () => navigate(`/apps/${app.slug}`),
        })),
      },
    ];
  }, [apps, navigate, signedIn]);
  return <Layout commandGroups={groups} />;
}

export default function App() {
  const checkAuth = useAuthStore((state) => state.checkAuth);
  useEffect(() => {
    void checkAuth();
  }, [checkAuth]);

  return (
    <BrowserRouter>
      <GrootUIProvider>
        <ShellProvider config={SHELL_CONFIG}>
          <Suspense fallback={pageFallback}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route path="/setup" element={<Setup />} />
              <Route
                path="/"
                element={
                  <ProtectedRoute>
                    <AppLayout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<Navigate to="/apps" replace />} />
                {PAGES.map((page) => (
                  <Route key={page.to} path={page.path ?? page.to} element={page.element} />
                ))}
                <Route path="apps/:slug" element={<AppDetailPage />} />
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        </ShellProvider>
      </GrootUIProvider>
    </BrowserRouter>
  );
}
