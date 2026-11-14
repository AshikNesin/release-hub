/**
 * `pnpm check`: runs what ci.yml runs, so a green local run predicts CI (RFC 0001 §8).
 * Platforms that are missing (single-platform child) or can't build on this machine are
 * skipped with a message, never silently.
 */
import { spawnSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { join } from "node:path";
import micromatch from "micromatch";
import { additionalExclusions, exists, ROOT } from "./lib.ts";

type Step = { name: string; cmd: string; args: string[]; cwd?: string };

const exclusions = additionalExclusions();
const hasPlatform = (dir: string) => exists(dir) && !micromatch.isMatch(`${dir}/x`, exclusions, { dot: true });

const steps: Step[] = [
  { name: "check:boundaries", cmd: "tsx", args: ["scripts/checks/boundaries.ts"] },
  { name: "check:tokens", cmd: "tsx", args: ["scripts/checks/tokens.ts"] },
  { name: "check:doc-refs", cmd: "tsx", args: ["scripts/checks/doc-refs.ts"] },
  { name: "check:kit-edits", cmd: "tsx", args: ["scripts/checks/kit-edits.ts"] },
  {
    name: "test:groot",
    cmd: "tsx",
    args: ["--test", ...readdirSync(join(ROOT, ".groot/tests")).filter((f) => f.endsWith(".test.ts")).map((f) => `.groot/tests/${f}`)],
  },
];
const skipped: string[] = [];

if (!hasPlatform("android")) {
  skipped.push("Android: no android/ folder in this app");
} else if (!process.env.ANDROID_HOME && !process.env.ANDROID_SDK_ROOT && !exists("android/local.properties")) {
  skipped.push("Android: no Android SDK (set ANDROID_HOME); see docs/quick-start.md");
} else {
  steps.push({
    name: "android: lint + unit tests",
    cmd: "./gradlew",
    args: ["lintDebug", "testDebugUnitTest", "--console=plain", "-q"],
    cwd: join(ROOT, "android"),
  });
}

if (!hasPlatform("ios")) {
  skipped.push("iOS: no ios/ folder in this app");
} else if (process.platform !== "darwin") {
  skipped.push("iOS: needs macOS with Xcode; CI runs it on a macOS runner");
} else {
  if (spawnSync("which", ["swiftlint"]).status === 0) {
    steps.push({ name: "ios: SwiftLint", cmd: "swiftlint", args: ["lint", "--strict", "--quiet"] });
  } else {
    skipped.push("iOS SwiftLint: not installed (brew install swiftlint)");
  }
  steps.push({ name: "ios: kit API check", cmd: "bash", args: ["scripts/checks/kit-api-ios.sh"] });
  steps.push({
    name: "ios: kit tests (GrootKit)",
    cmd: "xcodebuild",
    args: ["test", "-quiet", "-scheme", "GrootKit", "-destination", "platform=iOS Simulator,name=iPhone 17"],
    cwd: join(ROOT, "ios/GrootKit"),
  });
  steps.push({
    name: "ios: app tests (Unit test plan)",
    cmd: "xcodebuild",
    args: [
      "test", "-quiet", "-project", "ios/GrootMobile.xcodeproj", "-scheme", "GrootMobile", "-testPlan", "Unit",
      "-destination", "platform=iOS Simulator,name=iPhone 17", "CODE_SIGNING_ALLOWED=NO",
    ],
  });
}

let failed = 0;
for (const step of steps) {
  console.log(`\n▶ ${step.name}`);
  const result = spawnSync(step.cmd, step.args, { cwd: step.cwd ?? ROOT, stdio: "inherit", shell: false });
  if (result.status !== 0) {
    failed++;
    console.error(`✗ ${step.name} failed`);
  }
}

for (const s of skipped) console.log(`\n⏭  skipped — ${s}`);
console.log(failed === 0 ? "\n✓ pnpm check passed" : `\n✗ pnpm check: ${failed} step(s) failed`);
process.exit(failed === 0 ? 0 : 1);
