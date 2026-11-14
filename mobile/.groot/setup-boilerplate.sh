#!/usr/bin/env bash
# pnpm groot:setup — first run in a new child app (RFC 0001 §7):
#   1. Ask for the app name, Android applicationId and iOS bundle id.
#   2. Write them where they live: android/app/build.gradle.kts + strings.xml,
#      ios/App/Config/Base.xcconfig + ios/App/Sources/AppConfig.swift. Kit packages and Kotlin namespaces are never renamed.
#   3. Reset version.properties / Version.xcconfig and install dependencies.
set -euo pipefail

# The app root is this script's parent, not the git top level: the app may live in a monorepo subdirectory.
cd "$(dirname "$0")/.."

ask() { local prompt="$1" default="$2" reply; read -r -p "$prompt [$default]: " reply; echo "${reply:-$default}"; }
sed_inplace() { if [[ "$OSTYPE" == darwin* ]]; then sed -i '' "$@"; else sed -i "$@"; fi; }

APP_NAME="${GROOT_APP_NAME:-$(ask "App name" "Groot Mobile")}"
APPLICATION_ID="${GROOT_APPLICATION_ID:-$(ask "Android applicationId" "io.nesin.groot")}"
BUNDLE_ID="${GROOT_BUNDLE_ID:-$(ask "iOS bundle id" "$APPLICATION_ID")}"
FEEDBACK_EMAIL="${GROOT_FEEDBACK_EMAIL:-$(ask "Feedback email (Settings › About)" "feedback@example.com")}"
MAKER_NAME="${GROOT_MAKER_NAME:-$(ask "Built by (Settings footer)" "Nesin Technologies LLP")}"
MAKER_URL="${GROOT_MAKER_URL:-$(ask "Maker website" "https://nesin.io")}"

if [[ -f android/app/build.gradle.kts ]]; then
  sed_inplace -E "s|applicationId = \"[^\"]+\"|applicationId = \"$APPLICATION_ID\"|" android/app/build.gradle.kts
  sed_inplace -E "s|<string name=\"app_name\">[^<]*</string>|<string name=\"app_name\">$APP_NAME</string>|" android/app/src/main/res/values/strings.xml
  sed_inplace -E "s|<string name=\"home_title\">[^<]*</string>|<string name=\"home_title\">$APP_NAME</string>|" android/app/src/main/res/values/strings.xml
  CONFIG=android/app/src/main/res/values/config.xml
  sed_inplace -E "s|(name=\"config_feedback_email\"[^>]*>)[^<]*|\1$FEEDBACK_EMAIL|" "$CONFIG"
  sed_inplace -E "s|(name=\"config_maker_name\"[^>]*>)[^<]*|\1$MAKER_NAME|" "$CONFIG"
  sed_inplace -E "s|(name=\"config_maker_url\"[^>]*>)[^<]*|\1$MAKER_URL|" "$CONFIG"
  printf '# APP-owned. Bumped by the release script; versionCode only goes up.\nVERSION_NAME=0.1.0\nVERSION_CODE=1\n' > android/version.properties
  echo "✓ Android: applicationId=$APPLICATION_ID, name=\"$APP_NAME\""
fi

if [[ -f ios/App/Config/Base.xcconfig ]]; then
  sed_inplace -E "s|^PRODUCT_BUNDLE_IDENTIFIER = .*|PRODUCT_BUNDLE_IDENTIFIER = $BUNDLE_ID|" ios/App/Config/Base.xcconfig
  sed_inplace -E "s|^INFOPLIST_KEY_CFBundleDisplayName = .*|INFOPLIST_KEY_CFBundleDisplayName = $APP_NAME|" ios/App/Config/Base.xcconfig
  APP_CONFIG=ios/App/Sources/AppConfig.swift
  sed_inplace -E "s|(static let feedbackEmail = )\"[^\"]*\"|\1\"$FEEDBACK_EMAIL\"|" "$APP_CONFIG"
  sed_inplace -E "s|(static let makerName = )\"[^\"]*\"|\1\"$MAKER_NAME\"|" "$APP_CONFIG"
  sed_inplace -E "s|(static let makerURL = )\"[^\"]*\"|\1\"$MAKER_URL\"|" "$APP_CONFIG"
  printf 'MARKETING_VERSION = 0.1.0\nCURRENT_PROJECT_VERSION = 1\n' > ios/App/Config/Version.xcconfig
  echo "✓ iOS: bundle id=$BUNDLE_ID"
fi

pnpm install
echo
echo "Next: open android/ in Android Studio (or ios/ in Xcode) and run \`pnpm check\`."
