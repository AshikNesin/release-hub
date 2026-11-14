/**
 * Single source of truth for what syncs between groot-mobile-app and child apps
 * (RFC 0001 §1 and §7). The engine classifies with it in both directions
 * (`groot:sync` and `groot:upstream`), so the two can never disagree.
 *
 * This is the only file that differs from groot's engine: the mobile port changes
 * patterns, not engine code. Order matters in `categorizeFile`: IMMUTABLE, then APP
 * (incl. a child's `additional_exclusions`), then SHARED, then KIT, else drift.
 */
import { normalize } from "node:path";
import micromatch from "micromatch";

// ============================================================
// IMMUTABLE — never synced or upstreamed; cannot be overridden by config
// ============================================================

export const IMMUTABLE_EXCLUSIONS: readonly string[] = [
  // Secrets and signing material.
  ".env",
  ".env.*",
  "**/*.keystore",
  "**/*.jks",
  "**/*.p12",
  "**/*.mobileprovision",
  "**/*.pem",
  "**/*.key",
  "secrets/**",
  "android/local.properties",

  // Build output and dependencies.
  "node_modules/**",
  "**/build/**",
  "**/.gradle/**",
  ".git/**",

  // Sync tooling state — must never be overwritten with the boilerplate's own values.
  ".groot/boilerplate-sync.json",
  ".groot/sync-report.json",
  ".groot/needs-review/**",
] as const;

// ============================================================
// KIT — owned upstream, read-only in children (3-way reconciled)
// ============================================================

export const SYNC_PATTERNS: readonly string[] = [
  // Android kit, build logic, benchmark harness, Gradle wrapper.
  "android/kit/**",
  "android/build-logic/**",
  "android/benchmark/**",
  "android/build.gradle.kts",
  "android/gradle/wrapper/**",
  "android/gradlew",
  "android/gradlew.bat",

  // iOS kit (local Swift package).
  "ios/GrootKit/**",

  // Tooling, CI, scripts, docs, agent rules.
  ".groot/**",
  ".github/workflows/**",
  ".github/actions/**",
  ".github/pull_request_template.md",
  "scripts/**",
  "docs/**",
  ".changeset/config.json",
  ".changeset/README.md",
  ".editorconfig",
  ".swiftlint.yml",
] as const;

// ============================================================
// APP — child-owned, never synced or upstreamed
// ============================================================

export const SKIP_PATTERNS: readonly string[] = [
  // Your code: business logic, custom UI, tests.
  "android/app/**",
  "android/benchmark/**/Journeys.kt",
  "android/version.properties",
  "ios/App/**",
  "ios/AppTests/**",
  "ios/AppUITests/**",
  "ios/*.xcodeproj/**",
  "ios/*.xctestplan",
  "ios/ExportOptions.plist",

  // Per-repo docs and release state.
  "docs/app/**",
  "docs/rfcs/**",
  ".changeset/*.md",
  "CHANGELOG.md",
  "README.md",
  ".github/CODEOWNERS",
  "pnpm-lock.yaml",
] as const;

// ============================================================
// SHARED — both sides edit by design (3-way merged)
// ============================================================

export const FORCE_MERGE_FILES: readonly string[] = [
  "AGENTS.md",
  "package.json",
  ".gitignore",
  "android/settings.gradle.kts",
  "android/gradle/libs.versions.toml",
  "android/gradle.properties",
  ".github/dependabot.yml",
] as const;

// ============================================================
// PATH VALIDATION - Security against traversal attacks
// ============================================================

const FORBIDDEN_PATTERNS: readonly RegExp[] = [
  /\.\./, // Path traversal
  /~/, // Home directory
  /\\0/, // Null byte (escaped for lint)
  /^\//, // Absolute path
  /\$\(/, // Command substitution
  /`/, // Backtick
] as const;

export function validateFilePath(filePath: string): { valid: boolean; reason?: string } {
  for (const pattern of FORBIDDEN_PATTERNS) {
    if (pattern.test(filePath)) {
      return { valid: false, reason: "Forbidden pattern detected" };
    }
  }

  const normalized = normalize(filePath);
  if (normalized.startsWith("..") || normalized.startsWith("/")) {
    return { valid: false, reason: "Path traversal attempt" };
  }

  return { valid: true };
}

// ============================================================
// CATEGORIZATION - Where does a changed file belong?
// ============================================================

export type Action = "sync" | "skip-immutable" | "skip-app" | "drift";

const MATCH_OPTIONS = { dot: true } as const;

export function categorizeFile(filePath: string, additionalExclusions: string[]): { action: Action } {
  // 1. Immutable exclusions first (security).
  if (micromatch.isMatch(filePath, IMMUTABLE_EXCLUSIONS as string[], MATCH_OPTIONS)) {
    return { action: "skip-immutable" };
  }

  // 2. App-owned paths, incl. a child's additional_exclusions (e.g. "ios/**" for an
  //    Android-only app, RFC 0001 §7).
  const allSkipPatterns = [...SKIP_PATTERNS, ...additionalExclusions];
  if (micromatch.isMatch(filePath, allSkipPatterns, MATCH_OPTIONS)) {
    return { action: "skip-app" };
  }

  // 3. Shared files that always merge.
  if (FORCE_MERGE_FILES.includes(filePath)) {
    return { action: "sync" };
  }

  // 4. Kit paths.
  if (micromatch.isMatch(filePath, SYNC_PATTERNS as string[], MATCH_OPTIONS)) {
    return { action: "sync" };
  }

  // 5. Changed upstream, matches no pattern → drift (reported, not synced).
  return { action: "drift" };
}
