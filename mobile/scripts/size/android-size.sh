#!/usr/bin/env bash
# Android download-size gate (RFC 0001 §6): release AAB -> bundletool build-apks -> get-size total
# for the reference device spec (arm64-v8a, xxhdpi, en). Fails above the budget.
# Writes android/build/size/android-size.json (uploaded by CI; main's copy is the diff baseline).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BUDGET_BYTES="${ANDROID_SIZE_BUDGET_BYTES:-3145728}" # 3 MB starting budget; re-baseline after M2.
BUNDLETOOL_VERSION="1.18.3"
BUNDLETOOL="${BUNDLETOOL_JAR:-$HOME/.cache/groot/bundletool-all-$BUNDLETOOL_VERSION.jar}"
OUT="$ROOT/android/build/size"

if [[ ! -f "$BUNDLETOOL" ]]; then
  mkdir -p "$(dirname "$BUNDLETOOL")"
  curl -sSL -o "$BUNDLETOOL" \
    "https://github.com/google/bundletool/releases/download/$BUNDLETOOL_VERSION/bundletool-all-$BUNDLETOOL_VERSION.jar"
fi

(cd "$ROOT/android" && ./gradlew :app:bundleRelease --console=plain -q)

mkdir -p "$OUT"
rm -f "$OUT/app.apks"
java -jar "$BUNDLETOOL" build-apks \
  --bundle="$ROOT/android/app/build/outputs/bundle/release/app-release.aab" \
  --output="$OUT/app.apks" \
  --device-spec="$ROOT/scripts/size/device-spec-android.json"

# Output is CSV: "MIN,MAX" header then the byte counts.
SIZE_CSV="$(java -jar "$BUNDLETOOL" get-size total --apks="$OUT/app.apks" \
  --device-spec="$ROOT/scripts/size/device-spec-android.json" | tail -1 | tr -d "\r")"
MAX_BYTES="${SIZE_CSV##*,}"

printf '{"platform":"android","downloadBytes":%s,"budgetBytes":%s}\n' "$MAX_BYTES" "$BUDGET_BYTES" > "$OUT/android-size.json"
awk -v b="$MAX_BYTES" -v l="$BUDGET_BYTES" 'BEGIN { printf "Android download size: %.2f MB (budget %.2f MB)\n", b/1048576, l/1048576 }'

if (( MAX_BYTES > BUDGET_BYTES )); then
  echo "Over the size budget. Shrink the change or add a size-budget note to the PR (RFC 0001 §6)." >&2
  exit 1
fi
