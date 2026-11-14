#!/usr/bin/env tsx
/**
 * Groot Resolve - AI-assisted conflict resolution for groot sync.
 *
 * After `pnpm groot:sync` records conflicts in
 * `.groot/needs-review/manifest.json`, this tool resolves each conflicting
 * file using the pi coding agent CLI (https://pi.dev). The model reads the
 * conflict markers, merges both sides intelligently while preserving local
 * customizations, and returns the fully resolved file; this tool validates the
 * output (no markers, non-empty) before writing anything, so a malformed model
 * response is never written to disk. The repo's AGENTS.md conventions are fed
 * to the model as part of its system prompt.
 *
 * The source of truth is the working tree plus local git state: files that
 * already contain markers are resolved directly with no boilerplate checkout.
 * A checkout is only acquired when markers must be regenerated (sync ran with
 * `--skip-conflicts`, e.g. CI); the merge base then comes from the same lookup
 * sync uses (`lib/sync-state.ts`).
 *
 * Prerequisite: the pi CLI must be installed and authenticated
 * (`npm install -g --ignore-scripts @earendil-works/pi-coding-agent`, then an
 * API key env var such as `ZAI_API_KEY`, or `pi` + `/login`). Resolution runs
 * on whatever provider/model pi is configured with, unless overridden via
 * `--model` / `GROOT_RESOLVE_MODEL` / `GROOT_RESOLVE_PROVIDER`.
 *
 * Usage:
 *   pnpm groot:resolve                  - Resolve every conflict in the manifest
 *   pnpm groot:resolve --dry-run        - List pending conflicts, run nothing
 *   pnpm groot:resolve --file <path>    - Resolve a single file (repeatable)
 *   pnpm groot:resolve --no-verify      - Skip the post-resolution `pnpm check`
 *   pnpm groot:resolve --model <m>      - Model pattern or `provider/id`
 *                                         (default: pi's configured model)
 */
import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { spawn } from "node:child_process";
import { loadConfig } from "./lib/config";
import {
  exec,
  gitMergeFile,
  twoWayConflictMarkers,
  resolveDefaultBranch,
  showBlob,
  CONFLICT_MARKER_RE,
} from "./lib/git";
import { acquireBoilerplate, releaseBoilerplate, type AcquiredRepo } from "./lib/acquire";
import {
  openMergeBase,
  readManifest,
  writeManifest,
  MANIFEST_RELATIVE_PATH,
  type ConflictManifest,
  type MergeBase,
} from "./lib/sync-state";

// ============================================================
// PI CLI - prerequisite check and model selection
// ============================================================

const PI_INSTALL_CMD = "npm install -g --ignore-scripts @earendil-works/pi-coding-agent";

async function checkPiCli(): Promise<void> {
  try {
    await exec("pi", ["--version"]);
  } catch {
    throw new Error(
      [
        "`pi` CLI is not installed.",
        "",
        "groot:resolve uses the pi coding agent (https://pi.dev) for AI conflict",
        "resolution. Install it with:",
        `  ${PI_INSTALL_CMD}`,
        "",
        "Then authenticate with your provider — either an API key env var (e.g.",
        "`export ZAI_API_KEY=...` for the ZAI Coding Plan) or run `pi` and use /login.",
      ].join("\n"),
    );
  }
}

/**
 * Provider/model CLI args for pi. When neither `--model` nor the env vars are
 * set, pi uses whatever model the developer has configured/last used, which
 * keeps resolution provider-agnostic (Anthropic, ZAI/GLM, Copilot, ...).
 */
function piModelArgs(opts: { model?: string }): string[] {
  const args: string[] = [];
  const provider = process.env.GROOT_RESOLVE_PROVIDER;
  const model = opts.model ?? process.env.GROOT_RESOLVE_MODEL;
  if (provider) args.push("--provider", provider);
  if (model) args.push("--model", model);
  return args;
}

// ============================================================
// SIDE SOURCING - the boilerplate checkout and merge base are only
// acquired when a side is actually needed.
// ============================================================

/** The boilerplate side of a conflict, acquired on first use. */
export interface BoilerplateSides {
  /** Boilerplate checkout directory. */
  dir(): Promise<string>;
  /** Merge base at the manifest's `fromCommit`, or null when it cannot be found. */
  base(): Promise<MergeBase | null>;
}

async function readBase(sides: BoilerplateSides, file: string): Promise<string | null> {
  const base = await sides.base();
  const blob = base === null ? null : await base.read(file);
  return blob === null ? null : blob.toString("utf-8");
}

/** Read the upstream side at the manifest's target commit, or null if deleted. */
async function readTheirs(
  manifest: ConflictManifest,
  file: string,
  sides: BoilerplateSides,
): Promise<string | null> {
  const blob = await showBlob(await sides.dir(), manifest.toCommit, file);
  return blob === null ? null : blob.toString("utf-8");
}

/**
 * Ensure the local file contains conflict markers for the agent to resolve. If
 * the file already has markers (sync wrote them), use as-is. Otherwise rebuild
 * them from the base/target commits recorded in the manifest.
 */
export async function ensureMarkers(
  file: string,
  projectRoot: string,
  manifest: ConflictManifest,
  sides: BoilerplateSides,
): Promise<"ready" | "missing-local" | "clean"> {
  const localPath = join(projectRoot, file);
  let ours: string;
  try {
    ours = await readFile(localPath, "utf-8");
  } catch {
    return "missing-local";
  }

  if (CONFLICT_MARKER_RE.test(ours)) return "ready";

  const theirs = await readTheirs(manifest, file, sides);
  if (theirs === null) return "clean"; // deleted upstream — nothing to merge
  if (ours === theirs) return "clean";

  const base = await readBase(sides, file);
  const markers =
    base === null
      ? twoWayConflictMarkers(file, ours, theirs)
      : (await gitMergeFile(file, ours, base, theirs)).content;

  await writeFile(localPath, markers);
  return "ready";
}

// ============================================================
// PI INVOCATION - locked-down single-shot `pi -p` run
// ============================================================

const RESOLVE_SYSTEM_PROMPT = [
  "You are a merge-conflict resolver. You receive a single file containing",
  "diff3-style git conflict markers and must produce the fully resolved file.",
  "",
  "Marker layout:",
  "  <<<<<<< <file> (current_repo)      — this repo's current version (PRESERVE its customizations)",
  "  ||||||| <file> (last_sync)         — groot at the commit last synced from (common ancestor)",
  "  =======",
  "  >>>>>>> <file> (groot_boilerplate)  — latest upstream groot boilerplate (ADOPT its improvements)",
  "",
  "Merge both sides intelligently: keep the local project's intent and customizations",
  "while incorporating the upstream improvements, and follow the conventions in AGENTS.md",
  "(included below when present).",
  "",
  "Respond with the COMPLETE resolved file content and NOTHING else:",
  "no commentary, no explanations, no markdown code fences. Your entire response",
  "is written to disk verbatim as the file. It MUST NOT contain any conflict",
  "markers (<<<<<<<, |||||||, =======, >>>>>>>).",
].join("\n");

function buildUserPrompt(file: string, commits: string[]): string {
  const commitList =
    commits.length > 0
      ? commits.map((c) => `  - ${c}`).join("\n")
      : "  (no commit details recorded)";
  return [
    `Resolve the git merge conflict in \`${file}\`.`,
    "",
    "Upstream commits that changed this file:",
    commitList,
    "",
    "The file content with conflict markers is provided as piped input below.",
    "Respond with only the fully resolved file content (markers removed).",
  ].join("\n");
}

/** Build the system prompt, appending AGENTS.md when present. Context-file
 * discovery is disabled on the pi run, so this append is the only (and
 * deterministic) way conventions reach the model. */
async function buildSystemPrompt(projectRoot: string): Promise<string> {
  let agentsMd = "";
  try {
    agentsMd = await readFile(join(projectRoot, "AGENTS.md"), "utf-8");
  } catch {
    agentsMd = "";
  }
  return agentsMd
    ? `${RESOLVE_SYSTEM_PROMPT}\n\n--- AGENTS.md ---\n${agentsMd}`
    : RESOLVE_SYSTEM_PROMPT;
}

/** Spawn `pi -p` with the conflicted content on stdin and capture stdout. */
function spawnPi(
  args: string[],
  stdinContent: string,
  cwd: string,
): Promise<{ code: number; stdout: string }> {
  return new Promise((resolve, reject) => {
    const child = spawn("pi", args, {
      cwd,
      env: { ...process.env, PI_SKIP_VERSION_CHECK: "1" },
      stdio: ["pipe", "pipe", "inherit"],
    });
    let stdout = "";
    child.stdout.on("data", (d) => (stdout += d));
    child.on("error", reject);
    child.on("close", (code) => resolve({ code: code ?? 1, stdout }));
    child.stdin.on("error", () => {});
    child.stdin.write(stdinContent);
    child.stdin.end();
  });
}

/**
 * Normalize a pi response into file content: reject empty output, unwrap a
 * single wrapping markdown code fence if the model added one despite
 * instructions, and ensure a trailing newline.
 */
export function extractFileContent(raw: string): string | null {
  let text = raw.replace(/^\s*\n/, "").replace(/\s+$/, "");
  if (!text) return null;
  const fenced = text.match(/^```[\w.+-]*\n([\s\S]*?)\n```$/);
  if (fenced) text = fenced[1];
  if (!text.trim()) return null;
  return text + "\n";
}

/**
 * Resolve one conflicted file via a locked-down, single-shot `pi -p` run: no
 * session, no tools, no extensions/skills/context files — just the merge
 * prompt in, the resolved file out. The output is validated (non-empty, no
 * conflict markers) before the caller writes it; one retry with feedback is
 * attempted when validation fails. Returns the resolved content, or null.
 */
async function runPi(
  file: string,
  projectRoot: string,
  conflictedContent: string,
  commits: string[],
  opts: { model?: string },
): Promise<string | null> {
  const baseArgs = [
    "-p",
    "--no-session",
    "--no-tools",
    "--no-extensions",
    "--no-skills",
    "--no-prompt-templates",
    "--no-context-files",
    ...piModelArgs(opts),
    "--system-prompt",
    await buildSystemPrompt(projectRoot),
  ];

  let feedback = "";
  for (let attempt = 1; attempt <= 2; attempt++) {
    const prompt = buildUserPrompt(file, commits) + feedback;
    let result: { code: number; stdout: string };
    try {
      result = await spawnPi([...baseArgs, prompt], conflictedContent, projectRoot);
    } catch (err) {
      console.error(`  ✗ pi run error: ${(err as Error).message}`);
      return null;
    }
    if (result.code !== 0) {
      console.error(`  ✗ pi exited with code ${result.code}`);
      return null;
    }

    const content = extractFileContent(result.stdout);
    if (content !== null && !CONFLICT_MARKER_RE.test(content)) {
      return content;
    }
    console.log(`  → attempt ${attempt} produced invalid output, retrying...`);
    feedback =
      "\n\nYour previous attempt was empty or still contained conflict markers. " +
      "Respond with ONLY the complete resolved file content — no fences, no commentary, " +
      "and absolutely no conflict markers.";
  }
  return null;
}

function runCheck(projectRoot: string): Promise<number> {
  return new Promise((resolve) => {
    const child = spawn("pnpm", ["check"], { cwd: projectRoot, stdio: "inherit" });
    child.on("close", (code) => resolve(code ?? 1));
    child.on("error", () => resolve(1));
  });
}

// ============================================================
// MAIN
// ============================================================

function parseArgValues(args: string[], flag: string): string[] {
  const values: string[] = [];
  for (let i = 0; i < args.length; i++) {
    if (args[i] === flag && i + 1 < args.length) values.push(args[i + 1]);
  }
  return values;
}

function parseArgValue(args: string[], flag: string): string | undefined {
  const idx = args.indexOf(flag);
  return idx >= 0 && idx + 1 < args.length ? args[idx + 1] : undefined;
}

async function main() {
  const args = process.argv.slice(2);
  const dryRun = args.includes("--dry-run");
  const noVerify = args.includes("--no-verify");
  const fileFilters = parseArgValues(args, "--file");
  const model = parseArgValue(args, "--model");

  const projectRoot = process.cwd();

  console.log("\n## Groot Resolve\n");

  // 1. Load config (validates this is a groot-synced project) + manifest
  const config = await loadConfig(projectRoot);

  const manifest = await readManifest(projectRoot);
  if (manifest === null) {
    console.log(`No conflicts to resolve — \`${MANIFEST_RELATIVE_PATH}\` not found.\n`);
    console.log("Run `pnpm groot:sync` first if you expect pending conflicts.\n");
    return;
  }

  let conflicts = manifest.conflicts;
  if (fileFilters.length > 0) {
    conflicts = conflicts.filter((c) => fileFilters.includes(c.file));
  }

  if (conflicts.length === 0) {
    console.log("No matching conflicts in the manifest.\n");
    return;
  }

  const from = manifest.fromVersion ? `v${manifest.fromVersion}` : manifest.fromCommit.slice(0, 7);
  const to = manifest.toVersion ? `v${manifest.toVersion}` : manifest.toCommit.slice(0, 7);
  console.log(`Resolving conflicts from ${from} → ${to}\n`);

  conflicts.forEach((c) => {
    const hunks = `${c.conflicts} conflict${c.conflicts === 1 ? "" : "s"}`;
    console.log(`  ${c.file} (${hunks})`);
  });
  console.log();

  if (dryRun) {
    console.log(
      `${conflicts.length} file(s) pending. Run without --dry-run to resolve with the pi coding agent.\n`,
    );
    return;
  }

  // 2. The pi CLI is required up front; the boilerplate checkout is only
  //    acquired when markers must be regenerated.
  await checkPiCli();

  let repo: AcquiredRepo | undefined;
  let base: Promise<MergeBase | null> | undefined;
  const sides: BoilerplateSides = {
    dir: async () => {
      repo ??= await acquireBoilerplate({
        repoUrl: config.boilerplate.repo,
        name: config.boilerplate.name,
        defaultBranch: await resolveDefaultBranch(config.boilerplate.repo),
        purpose: "resolve",
        ensureCommitReachable: manifest.toCommit,
      });
      return repo.dir;
    },
    // No base anywhere degrades to two-way markers rather than failing.
    base: () =>
      (base ??= sides
        .dir()
        .then((dir) => openMergeBase(projectRoot, dir, manifest.fromCommit))
        .catch(() => null)),
  };

  const resolved: string[] = [];
  const failed: string[] = [];
  const skipped: string[] = [];

  try {
    for (const c of conflicts) {
      console.log(`\n── Resolving ${c.file} ──\n`);

      const state = await ensureMarkers(c.file, projectRoot, manifest, sides);

      if (state === "missing-local") {
        console.log(`  ⚠ ${c.file} does not exist locally — skipping.`);
        skipped.push(c.file);
        continue;
      }
      if (state === "clean") {
        console.log(`  ✓ ${c.file} has no conflict markers — already resolved.`);
        resolved.push(c.file);
        continue;
      }

      const conflictedContent = await readFile(join(projectRoot, c.file), "utf-8");
      const resolvedContent = await runPi(c.file, projectRoot, conflictedContent, c.commits, {
        model,
      });
      if (resolvedContent !== null) {
        await writeFile(join(projectRoot, c.file), resolvedContent);
      }

      const stillConflicted = await fileHasMarkers(projectRoot, c.file);
      if (resolvedContent !== null && !stillConflicted) {
        console.log(`\n  ✓ Resolved ${c.file}`);
        resolved.push(c.file);
      } else {
        console.log(`\n  ✗ ${c.file} still has conflict markers — needs manual attention.`);
        failed.push(c.file);
      }
    }
  } finally {
    if (repo) await releaseBoilerplate(repo);
  }

  // 3. Update the manifest: drop resolved entries, keep failures.
  await writeManifest(projectRoot, {
    ...manifest,
    conflicts: manifest.conflicts.filter((c) => !resolved.includes(c.file)),
  });

  // 4. Summary + verification
  console.log(`\n## Summary\n`);
  console.log(`  - Resolved: ${resolved.length}`);
  console.log(`  - Failed:   ${failed.length}`);
  console.log(`  - Skipped:  ${skipped.length}`);

  if (failed.length > 0) {
    console.log(`\n⚠ Resolve the remaining files manually, then re-run 'pnpm groot:resolve'.`);
  }

  if (!noVerify && resolved.length > 0) {
    console.log(`\n→ Running 'pnpm check' to verify...\n`);
    const code = await runCheck(projectRoot);
    if (code === 0) {
      console.log(`\n✓ Checks passed.`);
    } else {
      console.log(`\n⚠ 'pnpm check' reported issues — review the resolved files.`);
    }
  }
}

async function fileHasMarkers(projectRoot: string, file: string): Promise<boolean> {
  try {
    return CONFLICT_MARKER_RE.test(await readFile(join(projectRoot, file), "utf-8"));
  } catch {
    return false;
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((err) => {
    console.error("\nError:", err.message);
    process.exit(1);
  });
}
