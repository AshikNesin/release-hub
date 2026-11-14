#!/usr/bin/env bash
# Refresh the vendored groot UI (web/vendor/groot) from a groot checkout, so
# release-hub keeps the same look and components as every other groot app.
#
#   web/scripts/sync-groot.sh [path/to/groot]     (default: ../groot next to this repo)
#
# What is synced:
#   - @groot/ui     : all of packages/ui/{src,vendor}
#   - @groot/shell  : the files listed in SHELL_FILES (layout, sidebar, ⌘K,
#                     settings chrome, theme, API client, feedback)
# Release-hub overrides (never overwritten; diff them by hand after a sync):
#   - shell/src/store/auth.ts              password-only single admin, no passkeys
#   - shell/src/pages/settings/sections.tsx  types only, no groot default sections
# The Go server speaks the shell's /api/v1 envelope contract (srv/api_v1.go),
# which is what lets the shell files stay verbatim.
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
groot="${1:-$here/../../groot}"
[ -d "$groot/packages/ui/src" ] || { echo "groot checkout not found at $groot" >&2; exit 1; }
dest="$here/vendor/groot"

SHELL_FILES=(
  index.css
  lib/api.ts lib/feedback.ts lib/query-client.tsx lib/shell-config.tsx lib/theme.ts
  lib/utils.ts lib/date.utils.ts
  components/Layout.tsx components/SidebarNav.tsx components/CommandPalette.tsx
  components/ProtectedRoute.tsx components/ThemeSettings.tsx
  components/layout/PageContainer.tsx components/layout/PageLayout.tsx components/layout/Section.tsx
  hooks/useSignOut.ts hooks/useToastMutation.ts
  store/command-palette.ts
  pages/settings/Settings.tsx pages/settings/SettingsLayout.tsx pages/settings/SettingsSearch.tsx
)

rm -rf "$dest/ui/src" "$dest/ui/vendor"
mkdir -p "$dest/ui"
cp -R "$groot/packages/ui/src" "$groot/packages/ui/vendor" "$dest/ui/"

for f in "${SHELL_FILES[@]}"; do
  mkdir -p "$dest/shell/src/$(dirname "$f")"
  cp "$groot/packages/shell/src/$f" "$dest/shell/src/$f"
done

rev="$(git -C "$groot" rev-parse --short HEAD 2>/dev/null || echo unknown)"
ver="$(node -p "require('$groot/package.json').version" 2>/dev/null || echo unknown)"
printf '%s\n' "groot $ver ($rev)" > "$dest/VERSION"
echo "Synced groot $ver ($rev) into web/vendor/groot."
echo "Now: check the overrides still match groot's surface, then pnpm --dir web typecheck && pnpm --dir web build"
