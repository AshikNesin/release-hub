import { Navigate, useLocation, useParams } from "react-router-dom";
import { SettingsLayout } from "./SettingsLayout";
import { DEFAULT_SETTINGS_SECTIONS, type SettingsSection } from "./sections";

export type { SettingsItem, SettingsSection } from "./sections";
export { DEFAULT_SETTINGS_SECTIONS } from "./sections";

interface SettingsProps {
  /** The sections, in sidebar order. Defaults to Account, Appearance, Security and Configuration. */
  sections?: SettingsSection[];
}

/**
 * The full-page settings screen. Mount it on a splat route
 * (`<Route path="/settings/*">`) so each section gets its own URL
 * (`/settings/appearance`); the bare path and unknown sections redirect to
 * the first section. Pair it with `fullPage: true` on its `NavDestination`
 * so `Layout` hides the app sidebar here.
 */
export function Settings({ sections = DEFAULT_SETTINGS_SECTIONS }: SettingsProps) {
  const { pathname } = useLocation();
  const splat = useParams()["*"] ?? "";

  // Strip the splat by segment count, not length: `pathname` is URL-encoded
  // and the param is decoded, so their lengths can differ.
  const splatSegments = splat.split("/").filter(Boolean);
  const pathSegments = pathname.split("/").filter(Boolean);
  const baseSegments = pathSegments.slice(0, pathSegments.length - splatSegments.length);
  const basePath = baseSegments.length > 0 ? `/${baseSegments.join("/")}` : "";

  const first = sections[0];
  if (!first) return null;

  const active = sections.find((section) => section.id === splatSegments[0]);
  if (!active || splatSegments.length > 1) {
    return <Navigate to={`${basePath}/${(active ?? first).id}`} replace />;
  }

  return <SettingsLayout sections={sections} active={active} basePath={basePath} />;
}
