/**
 * Classification guard for the mobile sync patterns (RFC 0001 §1, §7). If a path moves
 * between KIT, SHARED, APP and IMMUTABLE, it should be a deliberate edit to this test.
 */
import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { categorizeFile } from "../lib/patterns.ts";

const cases: Record<string, string[]> = {
  sync: [
    "android/kit/src/main/kotlin/groot/kit/theme/GrootTheme.kt",
    "android/build-logic/convention/src/main/kotlin/GrootAndroid.kt",
    "android/benchmark/src/main/kotlin/harness/StartupBenchmark.kt",
    "android/build.gradle.kts",
    "android/gradle/wrapper/gradle-wrapper.properties",
    "ios/GrootKit/Sources/GrootKit/Theme/GrootTheme.swift",
    ".groot/lib/engine.ts",
    ".github/workflows/ci.yml",
    "scripts/checks/tokens.ts",
    "docs/guides/sync.md",
    ".changeset/config.json",
    // SHARED files also take the "sync" action (3-way merged).
    "AGENTS.md",
    "package.json",
    "android/settings.gradle.kts",
    "android/gradle/libs.versions.toml",
  ],
  "skip-app": [
    "android/app/src/main/kotlin/groot/mobile/MainActivity.kt",
    "android/app/build.gradle.kts",
    "android/benchmark/src/main/kotlin/Journeys.kt",
    "android/version.properties",
    "ios/App/Sources/GrootMobileApp.swift",
    "ios/AppTests/HomeTests.swift",
    "ios/GrootMobile.xcodeproj/project.pbxproj",
    "docs/app/product.md",
    "docs/rfcs/0001-groot-mobile-app.md",
    ".changeset/brave-kit-fix.md",
    "README.md",
    "CHANGELOG.md",
  ],
  "skip-immutable": [
    ".env",
    ".env.production",
    "android/local.properties",
    "android/app/upload.keystore",
    "android/app/build/outputs/bundle/release/app-release.aab",
    ".groot/boilerplate-sync.json",
    ".groot/needs-review/manifest.json",
  ],
  drift: ["some-new-root-file.txt", "fastlane/Fastfile"],
};

describe("categorizeFile", () => {
  for (const [action, paths] of Object.entries(cases)) {
    for (const path of paths) {
      test(`${path} → ${action}`, () => {
        assert.equal(categorizeFile(path, []).action, action);
      });
    }
  }

  test("additional_exclusions opt a child out of a platform (Android-only app)", () => {
    const androidOnly = ["ios/**", ".github/actions/setup-ios/**", ".swiftlint.yml"];
    assert.equal(categorizeFile("ios/GrootKit/Sources/GrootKit/Theme/GrootTheme.swift", androidOnly).action, "skip-app");
    assert.equal(categorizeFile(".swiftlint.yml", androidOnly).action, "skip-app");
    assert.equal(categorizeFile("android/kit/build.gradle.kts", androidOnly).action, "sync");
  });
});
