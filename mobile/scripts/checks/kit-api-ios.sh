#!/usr/bin/env bash
# check:kit-api for iOS (RFC 0001 §5.2), boilerplate repo only. macOS + Xcode only.
# Compares GrootKit's public API with the latest v* tag that has the package, using
# `swift package diagnose-api-breaking-changes`. A break needs a `major` changeset with a migration note.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

if [[ -f .groot/boilerplate-sync.json ]] && grep -q '"commit"' .groot/boilerplate-sync.json \
  && [[ "$(git remote get-url origin 2>/dev/null)" != *groot-mobile-app* ]]; then
  echo "⏭  check:kit-api (iOS): runs in the boilerplate only; children use check:kit-edits"
  exit 0
fi

BASELINE=""
for tag in $(git tag --list 'v*' --sort=-v:refname); do
  if git cat-file -e "$tag:ios/GrootKit/Package.swift" 2>/dev/null; then BASELINE="$tag"; break; fi
done
if [[ -z "$BASELINE" ]]; then
  echo "⏭  check:kit-api (iOS): no v* tag contains ios/GrootKit yet; nothing to compare against"
  exit 0
fi

if ls .changeset/*.md 2>/dev/null | xargs grep -lqs -E '^"groot-mobile": major'; then
  echo "A major changeset is pending; API breaks are allowed in this release."
  exit 0
fi

SDK="$(xcrun --sdk iphonesimulator --show-sdk-path)"
echo "Comparing GrootKit's public API with $BASELINE"
swift package --package-path ios/GrootKit \
  --sdk "$SDK" --triple arm64-apple-ios17.0-simulator \
  diagnose-api-breaking-changes "$BASELINE"
