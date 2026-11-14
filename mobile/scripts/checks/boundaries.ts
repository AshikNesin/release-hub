/**
 * check:boundaries (RFC 0001 §5.2 rule 1, AGENTS.md):
 *  - The kit never imports app code, and holds no app-layer concerns (ViewModels,
 *    navigation, networking, databases).
 *  - App versions live in version.properties / Version.xcconfig, never in build files.
 */
import { exists, lines, read, report, walk, type Violation } from "./lib.ts";

const violations: Violation[] = [];

// --- Android kit imports -----------------------------------------------------
const appNamespace = exists("android/app/build.gradle.kts")
  ? (read("android/app/build.gradle.kts").match(/namespace\s*=\s*"([^"]+)"/)?.[1] ?? null)
  : null;

const KIT_BANNED: Array<[RegExp, string]> = [
  [/^import androidx\.lifecycle\.ViewModel\b/, "ViewModels belong in app code"],
  [/^import androidx\.navigation\./, "navigation belongs in app code"],
  [/^import (java\.net|okhttp3|retrofit2|io\.ktor)\./, "networking belongs in app code"],
  [/^import (androidx\.room|android\.database\.sqlite|app\.cash\.sqldelight)\./, "databases belong in app code"],
];

for (const file of walk("android/kit/src/main", (n) => n.endsWith(".kt"))) {
  for (const [no, text] of lines(file)) {
    const line = text.trim();
    if (!line.startsWith("import ")) continue;
    if (appNamespace && line.startsWith(`import ${appNamespace}.`)) {
      violations.push({ file, line: no, message: `kit imports app code (${appNamespace})` });
    }
    for (const [pattern, why] of KIT_BANNED) {
      if (pattern.test(line)) violations.push({ file, line: no, message: `banned in kit: ${why}` });
    }
  }
}

// --- iOS kit imports -----------------------------------------------------------
for (const file of walk("ios/GrootKit/Sources", (n) => n.endsWith(".swift"))) {
  for (const [no, text] of lines(file)) {
    if (/^\s*(@testable\s+)?import\s+(GrootMobile|App)\b/.test(text)) {
      violations.push({ file, line: no, message: "kit imports app code" });
    }
  }
}

// --- Versions only in version files ----------------------------------------------
for (const file of walk("android", (n) => n === "build.gradle.kts")) {
  for (const [no, text] of lines(file)) {
    if (/^\s*versionCode\s*=\s*\d+/.test(text) || /^\s*versionName\s*=\s*"/.test(text)) {
      violations.push({ file, line: no, message: "version literal in a build file; use android/version.properties" });
    }
  }
}

report("check:boundaries", violations, "kit imports and version locations are clean");
