#!/usr/bin/env bash
# Create a new child app from this boilerplate (RFC 0001 §7):
#   scripts/new-app.sh <dir> [--platforms android,ios]
# Copies the tree at the latest v* tag (or HEAD if there is none) with fresh history,
# seeds .groot/boilerplate-sync.json so the first sync has a baseline, and drops the
# boilerplate's own release state. Then run `pnpm groot:setup` inside <dir>.
#
# When <dir> is inside an existing git repo (a monorepo, e.g. ../my-server/mobile), the app
# is created as a subdirectory of that repo instead: no `git init`, nothing committed, and
# the GitHub files that only work at a repo root (workflows, dependabot, PR template) are
# left out and excluded from sync. Wire CI at the repo root (docs/guides/sync.md).
set -euo pipefail

usage() { echo "usage: scripts/new-app.sh <dir> [--platforms android,ios]" >&2; exit 2; }
[[ $# -ge 1 ]] || usage
DEST="$1"; shift
PLATFORMS="android,ios"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --platforms) PLATFORMS="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[[ -e "$DEST" ]] && { echo "$DEST already exists" >&2; exit 1; }

SRC="$(cd "$(dirname "$0")/.." && pwd)"
REF="$(git -C "$SRC" describe --tags --abbrev=0 --match 'v*' 2>/dev/null || echo HEAD)"
COMMIT="$(git -C "$SRC" rev-parse --short "$REF")"
VERSION=""
[[ "$REF" == v* ]] && VERSION="${REF#v}"

# A destination inside an existing work tree becomes a subdirectory app of that repo.
PARENT="$(dirname "$DEST")"
IN_REPO=false
if [[ -d "$PARENT" ]] && git -C "$PARENT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  IN_REPO=true
fi

mkdir -p "$DEST"
git -C "$SRC" archive "$REF" | tar -x -C "$DEST"
cd "$DEST"

# The boilerplate's own release state and design docs never reach a child.
find .changeset -name '*.md' ! -name README.md -delete
rm -rf docs/rfcs CHANGELOG.md
printf '# Changelog\n' > CHANGELOG.md

EXCLUDED=()
if [[ "$PLATFORMS" == "android" ]]; then
  rm -rf ios .swiftlint.yml .github/actions/setup-ios scripts/size/ios-size.sh scripts/release/release-internal-ios.sh
  EXCLUDED+=("ios/**" ".github/actions/setup-ios/**" ".swiftlint.yml" "scripts/size/ios-size.sh" "scripts/release/release-internal-ios.sh")
elif [[ "$PLATFORMS" == "ios" ]]; then
  rm -rf android .github/actions/setup-android scripts/size/android-size.sh scripts/size/device-spec-android.json scripts/release/release-internal-android.sh
  EXCLUDED+=("android/**" ".github/actions/setup-android/**" "scripts/size/android-size.sh" "scripts/size/device-spec-android.json" "scripts/release/release-internal-android.sh")
fi
if [[ "$IN_REPO" == true ]]; then
  # GitHub reads these only at the repo root; a nested copy would be dead code.
  # .github/actions/ stays: root workflows can call it as ./<dir>/.github/actions/...
  rm -rf .github/workflows .github/dependabot.yml .github/pull_request_template.md
  EXCLUDED+=(".github/workflows/**" ".github/dependabot.yml" ".github/pull_request_template.md")
fi
EXCLUSIONS="[]"
if [[ ${#EXCLUDED[@]} -gt 0 ]]; then
  EXCLUSIONS="[$(printf '"%s", ' "${EXCLUDED[@]}" | sed 's/, $//')]"
fi

VERSION_JSON=""
[[ -n "$VERSION" ]] && VERSION_JSON="\"version\": \"$VERSION\", "
cat > .groot/boilerplate-sync.json <<JSON
{
  "boilerplate": {
    "name": "groot-mobile-app",
    "repo": "$(git -C "$SRC" remote get-url origin 2>/dev/null || echo "file://$SRC")"
  },
  "last_sync": { ${VERSION_JSON}"commit": "$COMMIT", "date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)" },
  "additional_exclusions": $EXCLUSIONS
}
JSON

if [[ "$IN_REPO" == true ]]; then
  echo "✓ Created $DEST from groot-mobile-app $REF ($COMMIT) inside $(git rev-parse --show-toplevel), platforms: $PLATFORMS"
  echo "Next: cd $DEST && pnpm groot:setup, commit it, and add root CI (docs/guides/sync.md, \"Apps inside a monorepo\")."
else
  git init -q
  git add -A
  git commit -q -m "Create app from groot-mobile-app ${REF} (${COMMIT})"
  echo "✓ Created $DEST from groot-mobile-app $REF ($COMMIT), platforms: $PLATFORMS"
  echo "Next: cd $DEST && pnpm groot:setup"
fi
