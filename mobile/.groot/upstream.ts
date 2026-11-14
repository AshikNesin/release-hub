#!/usr/bin/env tsx
/**
 * Groot Upstream - Push child-repo fixes back to boilerplate
 *
 * Upstream is sync with the sides swapped (see `planUpstream` in
 * `lib/engine.ts`): every synced file the child changed since its last sync is
 * three-way merged onto groot HEAD, with the last sync as the base. Groot's
 * own changes since then are kept; files where both sides changed the same
 * lines are reported as conflicts and left out of the PR.
 *
 * Usage:
 *   pnpm groot:upstream           - Interactive mode: select files to upstream
 *   pnpm groot:upstream --dry-run - Show what would be upstreamed (no changes)
 *   pnpm groot:upstream --all     - Upstream every file that merges cleanly
 */
import { readFile, writeFile, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";
import { loadConfig, type SyncConfig } from "./lib/config";
import { exec, resolveDefaultBranch } from "./lib/git";
import { acquireBoilerplate, releaseBoilerplate } from "./lib/acquire";
import { planUpstream, applyUpstream, type UpstreamItem, type UpstreamPlan } from "./lib/engine";

// ============================================================
// DIFF STATS
// ============================================================

interface DiffStats {
  added: number;
  removed: number;
}

/** Line counts of the change from `before` to `after` (zero for binary files). */
async function diffStats(before: Buffer, after: Buffer | string): Promise<DiffStats> {
  const dir = await mkdtemp(join(tmpdir(), "groot-upstream-diff-"));
  try {
    await writeFile(join(dir, "before"), before);
    await writeFile(join(dir, "after"), after);
    const { stdout } = await exec("git", ["diff", "--no-index", "--numstat", "before", "after"], {
      cwd: dir,
    }).catch((err) => {
      // git diff --no-index exits 1 when the files differ — that's expected
      if (typeof err.stdout === "string") return { stdout: err.stdout as string };
      throw err;
    });
    const [added, removed] = stdout.split("\t");
    return { added: Number(added) || 0, removed: Number(removed) || 0 };
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

// ============================================================
// REMOTE HELPERS - Repo slug resolution
// ============================================================

/**
 * Extract an "OWNER/REPO" slug from any GitHub URL form for `gh pr create`.
 * Handles https://github.com/owner/repo(.git) and
 * ssh://git@github.com/owner/repo(.git) (both pass Zod's url() validation),
 * as well as the scheme-less SSH shorthand git@github.com:owner/repo(.git).
 */
function parseRepoSlug(repoUrl: string): string {
  try {
    const url = new URL(repoUrl);
    const parts = url.pathname
      .replace(/^\/+|\/+$/g, "")
      .replace(/\.git$/, "")
      .split("/");
    if (parts.length >= 2) return `${parts[0]}/${parts[1]}`;
  } catch {
    // SSH shorthand without a scheme: git@github.com:owner/repo(.git)
    const match = repoUrl.match(/[:/]([^/]+)\/([^/]+?)(?:\.git)?$/);
    if (match) return `${match[1]}/${match[2]}`;
  }
  throw new Error(`Could not parse GitHub owner/repo from URL: ${repoUrl}`);
}

// ============================================================
// PREREQUISITES CHECK
// ============================================================

async function checkGhCli(): Promise<void> {
  try {
    await exec("gh", ["--version"]);
  } catch {
    throw new Error(
      [
        "`gh` CLI is not installed.",
        "",
        "Install it with:",
        "  brew install gh",
        "",
        "Then authenticate:",
        "  gh auth login",
        "",
        "See: https://cli.github.com/",
      ].join("\n"),
    );
  }

  // Check authentication
  try {
    await exec("gh", ["auth", "status"]);
  } catch {
    throw new Error(
      ["`gh` CLI is not authenticated.", "", "Run:", "  gh auth login", "", "Then try again."].join(
        "\n",
      ),
    );
  }
}

async function getChildAppName(projectRoot: string): Promise<string> {
  const pkgPath = join(projectRoot, "package.json");
  try {
    const raw = JSON.parse(await readFile(pkgPath, "utf-8"));
    const name = raw.name;
    if (typeof name !== "string" || name.length === 0) {
      throw new Error("Missing `name` field in package.json");
    }
    return name;
  } catch (err) {
    if (err instanceof Error && err.message.includes("Missing")) throw err;
    throw new Error(`Could not read package.json at ${pkgPath}`);
  }
}

// ============================================================
// INTERACTIVE SELECTION
// ============================================================

interface UpstreamCandidate {
  item: UpstreamItem;
  stats: DiffStats;
}

async function promptSelection(candidates: UpstreamCandidate[]): Promise<UpstreamCandidate[]> {
  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  const ask = (question: string): Promise<string> =>
    new Promise((resolve) => rl.question(question, resolve));

  try {
    console.log("\nEnter file numbers (comma-separated), 'all', or 'q' to quit:");
    const answer = await ask("> ");

    const trimmed = answer.trim().toLowerCase();

    if (trimmed === "q" || trimmed === "") {
      console.log("\nAborted.");
      process.exit(0);
    }

    if (trimmed === "all") {
      return candidates;
    }

    const indices = trimmed
      .split(",")
      .map((s) => Number.parseInt(s.trim(), 10))
      .filter((n) => !Number.isNaN(n));

    const selected = indices
      .map((i) => candidates[i - 1])
      .filter((c): c is UpstreamCandidate => c !== undefined);

    if (selected.length === 0) {
      console.log("\nNo valid files selected. Aborted.");
      process.exit(0);
    }

    return selected;
  } finally {
    rl.close();
  }
}

// ============================================================
// CREATE UPSTREAM PR
// ============================================================

async function createUpstreamPR(
  config: SyncConfig,
  selected: UpstreamCandidate[],
  childAppName: string,
  defaultBranch: string,
): Promise<string> {
  const tempDir = await mkdtemp(join(tmpdir(), "groot-upstream-pr-"));

  try {
    // Clone boilerplate fresh to its default branch
    console.log(`\nCloning ${config.boilerplate.repo} for PR (branch: ${defaultBranch})...`);
    await exec("git", [
      "clone",
      "--depth",
      "1",
      "--branch",
      defaultBranch,
      config.boilerplate.repo,
      tempDir,
    ]);

    // Create branch
    const dateSuffix = new Date().toISOString().slice(0, 10);
    const branchName = `upstream/${childAppName}/${dateSuffix}`;

    await exec("git", ["-C", tempDir, "checkout", "-b", branchName]);

    // Merge the selected files onto this clone's copy of groot
    const { written, unmerged } = await applyUpstream(
      tempDir,
      selected.map((c) => c.item),
    );
    if (unmerged.length > 0) {
      throw new Error(
        [
          `groot ${defaultBranch} changed since the files were listed, and these no longer merge cleanly:`,
          ...unmerged.map((f) => `  ${f}`),
          "",
          "Run `pnpm groot:sync`, then `pnpm groot:upstream` again.",
        ].join("\n"),
      );
    }
    if (written.length === 0) {
      throw new Error(`groot ${defaultBranch} already has every selected change.`);
    }
    written.forEach((f) => console.log(`  + ${f}`));

    // Stage and commit
    await exec("git", ["-C", tempDir, "add", "."]);

    const commitMessage = `fix: upstream changes from ${childAppName}`;
    await exec("git", ["-C", tempDir, "commit", "-m", commitMessage]);

    // Push branch
    console.log(`\nPushing branch ${branchName}...`);
    await exec("git", ["-C", tempDir, "push", "origin", branchName]);

    // Build PR body
    const fileList = selected
      .filter((c) => written.includes(c.item.file))
      .map((c) => `- \`${c.item.file}\` (+${c.stats.added} -${c.stats.removed})`)
      .join("\n");

    const prBody = [
      `## Upstream from \`${childAppName}\``,
      "",
      "### Files changed",
      fileList,
      "",
      `_Created by \`pnpm groot:upstream\`_`,
    ].join("\n");

    // Create PR using gh CLI
    const { stdout: prUrl } = await exec(
      "gh",
      [
        "pr",
        "create",
        "--repo",
        parseRepoSlug(config.boilerplate.repo),
        "--title",
        `fix: upstream from ${childAppName}`,
        "--body",
        prBody,
        "--base",
        defaultBranch,
        "--head",
        branchName,
      ],
      { cwd: tempDir },
    );

    return prUrl.trim();
  } finally {
    await rm(tempDir, { recursive: true, force: true });
  }
}

// ============================================================
// MAIN
// ============================================================

type Mode = "dry-run" | "interactive" | "all";

function printSection(title: string, files: string[]): void {
  if (files.length === 0) return;
  console.log(`### ${title} (${files.length})`);
  files.forEach((f) => console.log(`  ${f}`));
  console.log();
}

async function main() {
  const args = process.argv.slice(2);
  const dryRun = args.includes("--dry-run");
  const all = args.includes("--all");

  const mode: Mode = dryRun ? "dry-run" : all ? "all" : "interactive";

  const projectRoot = process.cwd();

  console.log("\n## Groot Upstream\n");
  console.log(`Mode: ${mode}\n`);

  // 1. Load and validate config
  const config = await loadConfig(projectRoot);

  // 2. Check prerequisites (skip for dry-run)
  if (!dryRun) {
    await checkGhCli();
  }

  // 3. Get child app name + resolve the boilerplate's default branch in parallel
  //    (independent reads — don't assume "main" for the branch).
  const [childAppName, defaultBranch] = await Promise.all([
    getChildAppName(projectRoot),
    resolveDefaultBranch(config.boilerplate.repo),
  ]);

  // 4. Merge every locally-changed synced file onto groot HEAD. Prefers a clean
  //    local checkout at ~/Code/<name>; never deletes one it did not create.
  const repo = await acquireBoilerplate({
    repoUrl: config.boilerplate.repo,
    name: config.boilerplate.name,
    defaultBranch,
    purpose: "upstream-diff",
    ensureCommitReachable: config.last_sync.commit,
  });
  let plan: UpstreamPlan;
  try {
    plan = await planUpstream(projectRoot, repo.dir);
  } finally {
    await releaseBoilerplate(repo);
  }

  const byKind = (kind: UpstreamItem["outcome"]["kind"]): string[] =>
    plan.items.filter((i) => i.outcome.kind === kind).map((i) => i.file);
  const candidates: UpstreamCandidate[] = await Promise.all(
    plan.items.flatMap((item) =>
      item.outcome.kind === "ready"
        ? [
            diffStats(item.groot ?? Buffer.alloc(0), item.outcome.content).then((stats) => ({
              item,
              stats,
            })),
          ]
        : [],
    ),
  );

  // 5. Display candidates and the files that cannot be upstreamed as-is
  printSection(
    "Conflicts — groot changed the same lines since your last sync; run `pnpm groot:sync` first",
    byKind("conflict"),
  );
  printSection(
    "Binary Conflicts — changed on both sides, reconcile manually",
    byKind("binary-conflict"),
  );
  printSection(
    "Deleted in groot since your last sync — not upstreamed",
    byKind("deleted-in-groot"),
  );

  if (candidates.length === 0) {
    console.log("No local changes to synced files can be upstreamed.\n");
    return;
  }

  console.log(`### Modified Synced Files (candidates for upstream)`);
  candidates.forEach((c, i) => {
    console.log(`  ${i + 1}. ${c.item.file} (+${c.stats.added} -${c.stats.removed})`);
  });
  console.log();

  // 6. Dry run stops here
  if (dryRun) {
    console.log(`Found ${candidates.length} file(s) that merge cleanly onto groot.`);
    console.log("Run without --dry-run to upstream changes.\n");
    return;
  }

  // 7. Select files
  const selected = all ? candidates : await promptSelection(candidates);

  console.log(`\n### Selected for Upstream`);
  selected.forEach((c) => {
    console.log(`  ${c.item.file}`);
  });
  console.log();

  // 8. Create upstream PR
  const prUrl = await createUpstreamPR(config, selected, childAppName, defaultBranch);

  console.log(`✓ Created PR: ${prUrl}\n`);
}

main().catch((err) => {
  console.error("\nError:", err.message);
  process.exit(1);
});
