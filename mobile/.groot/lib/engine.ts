/**
 * Groot sync engine — full-tree snapshot reconciliation.
 *
 * Instead of diffing upstream commits (fragile: depends on JSON state and on
 * old commits staying reachable), every run reconciles the union of synced
 * files across three trees:
 *
 *   ours   = the child repo working tree
 *   base   = the baseline snapshot from the last sync (refs/groot/baseline,
 *            rebuilt from the boilerplate clone when missing or stale)
 *   theirs = the boilerplate default-branch HEAD
 *
 * This makes sync idempotent and self-healing: a crashed apply, a lost
 * manifest, or a stale report simply reconverges on the next run. Deletions
 * sync safely (only when the local file is unmodified since last sync), and
 * nothing is written until every decision has been computed. The baseline ref
 * and config are advanced last, so a mid-apply crash never loses state.
 *
 * Upstream (child → groot) is the same reconciliation with the sides swapped:
 * groot is the side being written, the child's copy is the incoming change.
 */
import { readFile, writeFile, mkdir, rm, chmod } from "node:fs/promises";
import { join, dirname } from "node:path";
import { loadConfig, saveConfig, type SyncConfig } from "./config";
import { categorizeFile, validateFilePath } from "./patterns";
import {
  git,
  blobSha,
  showBlob,
  listTree,
  resolveCommit,
  resolveDefaultBranch,
  resolveLatestVersion,
  fileCommits,
  gitMergeFile,
  dirtyPaths,
  shaMatches,
  wasEverTracked,
  type TreeEntry,
} from "./git";
import { acquireBoilerplate, releaseBoilerplate } from "./acquire";
import { readBaselineInfo, writeBaseline, BASELINE_REF } from "./baseline";
import { openMergeBase, writeManifest } from "./sync-state";
import { reconcileFile } from "./reconcile";
import { extractChangelog, findBreakingChanges } from "./changelog";

// ============================================================
// TYPES
// ============================================================

export interface SyncOptions {
  mode: "check" | "apply";
  /** Don't write conflict markers into the working tree (CI); manifest only. */
  skipConflicts: boolean;
  /** Overwrite files even when they have uncommitted git changes. */
  force: boolean;
}

interface WriteItem {
  file: string;
  content: Buffer | string;
  executable?: boolean;
}

export interface ConflictItem {
  file: string;
  content: string;
  conflicts: number;
  commits: string[];
}

export interface SyncResult {
  fromCommit: string;
  toCommit: string;
  fromVersion: string | null;
  toVersion: string | null;
  changelog: string[];
  breakingChanges: string[];
  autoApply: WriteItem[];
  autoMerged: WriteItem[];
  conflicts: ConflictItem[];
  binaryConflicts: string[];
  deleted: string[];
  reviewDeletions: string[];
  keptLocalDeletions: string[];
  drift: string[];
  skipped: { immutable: string[]; appSpecific: string[] };
}

const REPORT_RELATIVE_PATH = ".groot/sync-report.json";

/** Synced regular files of a tree, keyed by path. */
function syncedBlobs(entries: TreeEntry[], exclusions: string[]): Map<string, TreeEntry> {
  const synced = new Map<string, TreeEntry>();
  for (const entry of entries) {
    if (entry.type !== "blob" || entry.mode === "120000") continue;
    if (!validateFilePath(entry.path).valid) continue;
    if (categorizeFile(entry.path, exclusions).action === "sync") synced.set(entry.path, entry);
  }
  return synced;
}

// ============================================================
// SYNC
// ============================================================

export async function runSync(projectRoot: string, opts: SyncOptions): Promise<SyncResult> {
  const config = await loadConfig(projectRoot);
  const exclusions = config.additional_exclusions;

  const defaultBranch = await resolveDefaultBranch(config.boilerplate.repo);
  const repo = await acquireBoilerplate({
    repoUrl: config.boilerplate.repo,
    name: config.boilerplate.name,
    defaultBranch,
    purpose: "sync",
    ensureCommitReachable: config.last_sync.commit,
    fetchTags: true,
  });

  try {
    const cloneDir = repo.dir;
    // Independent reads on the clone — run concurrently.
    const [targetCommit, latestVersion, base] = await Promise.all([
      resolveCommit(cloneDir, "HEAD"),
      resolveLatestVersion(cloneDir),
      openMergeBase(projectRoot, cloneDir, config.last_sync.commit),
    ]);
    const lastSyncSha = base.commit;
    const lastSyncReachable = base.reachable;
    console.log(`Base: ${base.description}`);

    // --- Candidate set: union of synced files in boilerplate HEAD and in the
    // baseline (the latter catches upstream deletions).
    const theirsEntries = syncedBlobs(await listTree(cloneDir, targetCommit), exclusions);
    const baseEntries = syncedBlobs(await base.entries(), exclusions);

    const candidates = [...new Set([...theirsEntries.keys(), ...baseEntries.keys()])].sort();

    const result: SyncResult = {
      fromCommit: lastSyncSha,
      toCommit: targetCommit,
      fromVersion: config.last_sync.version ?? null,
      toVersion: latestVersion?.version ?? null,
      changelog: [],
      breakingChanges: [],
      autoApply: [],
      autoMerged: [],
      conflicts: [],
      binaryConflicts: [],
      deleted: [],
      reviewDeletions: [],
      keptLocalDeletions: [],
      drift: [],
      skipped: { immutable: [], appSpecific: [] },
    };

    // --- Report-only context: changelog, breaking changes, drift and skipped
    // files derived from the upstream commit range (best-effort).
    if (lastSyncReachable) {
      const fromRef = config.last_sync.version ? `v${config.last_sync.version}` : lastSyncSha;
      const toRef = latestVersion ? `v${latestVersion.version}` : "HEAD";
      result.changelog = await extractChangelog(cloneDir, fromRef, toRef);
      result.breakingChanges = findBreakingChanges(result.changelog);

      try {
        const changed = (
          await git(cloneDir, "diff", "--name-only", "--diff-filter=d", `${lastSyncSha}..HEAD`)
        )
          .split("\n")
          .filter(Boolean);
        for (const file of changed) {
          if (!validateFilePath(file).valid) continue;
          const { action } = categorizeFile(file, exclusions);
          if (action === "drift") result.drift.push(file);
          else if (action === "skip-immutable") result.skipped.immutable.push(file);
          else if (action === "skip-app") result.skipped.appSpecific.push(file);
        }
      } catch {
        /* report-only — never block the sync */
      }
    }

    // --- Reconcile every candidate file. Each file is independent (unique
    // path, stateless git merge via mkdtemp), so fan out concurrently.
    await Promise.all(
      candidates.map(async (path) => {
        const theirsEntry = theirsEntries.get(path) ?? null;
        const baseEntry = baseEntries.get(path) ?? null;
        const localPath = join(projectRoot, path);

        // Fast path: unchanged upstream since last sync (base blob == theirs
        // blob). When the file is present locally, every branch of the decision
        // table is a no-op, so skip the reads/merge entirely. When it's MISSING
        // locally we fall through to the reconcile path instead of classifying
        // here, so the deletion checks (kept-local-deletion and the
        // first-adoption guard) live in one place instead of being duplicated.
        if (theirsEntry && baseEntry && theirsEntry.sha === baseEntry.sha) {
          const exists = await readFile(localPath)
            .then(() => true)
            .catch(() => false);
          if (exists) return;
        }

        const ours = await readFile(localPath).catch(() => null);
        const baseContent = baseEntry ? await base.read(path) : null;
        const theirs = theirsEntry ? await showBlob(cloneDir, targetCommit, path) : null;

        const outcome = await reconcileFile(
          { path, ours, base: baseContent, theirs },
          gitMergeFile,
        );
        const executable = theirsEntry?.mode === "100755";

        switch (outcome.kind) {
          case "identical":
          case "ignore":
            break;
          case "auto-apply": {
            // First-adoption guard. Reconcile calls a file "new upstream" when
            // it has no base entry, but a missing base means "not synced at the
            // last sync", not "never existed here": the baseline is a snapshot
            // of groot's tree filtered by whichever classifier ran last, so a
            // path the kit only just grew to own has no base while the child
            // may have tracked it all along. When this repo's own history shows
            // the path was tracked and it is gone now, that is a deliberate
            // local deletion, not an adoption — honor it instead of
            // resurrecting a file the child chose to drop. Tracked-and-present
            // files never reach this branch (reconcile already returned
            // identical or merged above); this catches ours-missing only.
            const resurrectingDeletion =
              ours === null && baseEntry === null && (await wasEverTracked(projectRoot, path));
            if (resurrectingDeletion) {
              result.keptLocalDeletions.push(path);
            } else {
              result.autoApply.push({ file: path, content: outcome.content, executable });
            }
            break;
          }
          case "auto-merged":
            result.autoMerged.push({ file: path, content: outcome.content, executable });
            break;
          case "conflict":
            result.conflicts.push({
              file: path,
              content: outcome.content,
              conflicts: outcome.conflicts,
              commits: lastSyncReachable ? await fileCommits(cloneDir, lastSyncSha, path) : [],
            });
            break;
          case "binary-conflict":
            result.binaryConflicts.push(path);
            break;
          case "delete":
            result.deleted.push(path);
            break;
          case "review-delete":
            result.reviewDeletions.push(path);
            break;
          case "kept-local-deletion": {
            // The baseline (a snapshot of groot's tree) says this file was synced
            // but it's gone locally. That's usually an intentional delete, but
            // the baseline is built from groot's tree, not this repo's history —
            // a file a prior sync wrote into the working tree but never committed
            // (then swept, e.g. by `git clean -fd`) is indistinguishable from a
            // deliberate delete without consulting this repo's own history. Honor
            // the deletion only when the file was ever tracked here; otherwise
            // it's a phantom that never landed, so restore theirs.
            if (await wasEverTracked(projectRoot, path)) {
              result.keptLocalDeletions.push(path);
            } else {
              result.autoApply.push({ file: path, content: outcome.theirs, executable });
            }
            break;
          }
        }
      }),
    );

    // --- Apply phase. Everything is computed; now write, then advance state
    // last so a crash mid-apply never moves the baseline.
    if (opts.mode === "apply") {
      await applyChanges(projectRoot, result, opts);
      await advanceState(projectRoot, cloneDir, config, {
        targetCommit,
        version: latestVersion?.version ?? null,
        theirsEntries: [...theirsEntries.values()],
      });
    }

    await writeSyncReport(projectRoot, result);

    return result;
  } finally {
    await releaseBoilerplate(repo);
  }
}

// ============================================================
// APPLY
// ============================================================

async function applyChanges(
  projectRoot: string,
  result: SyncResult,
  opts: SyncOptions,
): Promise<void> {
  const writes: WriteItem[] = [...result.autoApply, ...result.autoMerged];
  const touched = [
    ...writes.map((w) => w.file),
    ...result.deleted,
    ...(opts.skipConflicts ? [] : result.conflicts.map((c) => c.file)),
  ];

  if (touched.length > 0 && !opts.force) {
    const dirty = await dirtyPaths(projectRoot, touched);
    if (dirty.length > 0) {
      throw new Error(
        [
          "Refusing to overwrite files that have uncommitted changes:",
          ...dirty.map((f) => `  ${f}`),
          "",
          "Commit or stash them first, or re-run with --force.",
        ].join("\n"),
      );
    }
  }

  // Each write/delete targets a unique path — fan out concurrently.
  await Promise.all(
    writes.map(async ({ file, content, executable }) => {
      const dest = join(projectRoot, file);
      await mkdir(dirname(dest), { recursive: true });
      await writeFile(dest, content);
      if (executable) await chmod(dest, 0o755);
      console.log(`  + ${file}`);
    }),
  );

  await Promise.all(
    result.deleted.map(async (file) => {
      await rm(join(projectRoot, file), { force: true });
      console.log(`  - ${file}`);
    }),
  );

  // Write conflict markers into the working tree (skipped in CI mode so the
  // automated PR stays compilable; the manifest still records them).
  if (!opts.skipConflicts) {
    await Promise.all(
      result.conflicts.map(async ({ file, content }) => {
        const dest = join(projectRoot, file);
        await mkdir(dirname(dest), { recursive: true });
        await writeFile(dest, content);
        console.log(`  ! ${file} (conflict markers)`);
      }),
    );
  }

  // Always record conflicts in the manifest so `pnpm groot:resolve` can act on
  // them regardless of whether markers were written to disk. When there are no
  // conflicts, this drops any stale manifest from a previous run.
  await writeManifest(projectRoot, {
    fromVersion: result.fromVersion,
    toVersion: result.toVersion,
    fromCommit: result.fromCommit,
    toCommit: result.toCommit,
    generatedAt: new Date().toISOString(),
    conflicts: result.conflicts.map((c) => ({
      file: c.file,
      conflicts: c.conflicts,
      commits: c.commits,
    })),
  });
}

async function advanceState(
  projectRoot: string,
  cloneDir: string,
  config: SyncConfig,
  target: {
    targetCommit: string;
    version: string | null;
    theirsEntries: TreeEntry[];
  },
): Promise<void> {
  const baseline = await readBaselineInfo(projectRoot);
  if (baseline === null || !shaMatches(baseline.boilerplateCommit, target.targetCommit)) {
    await writeBaseline(
      projectRoot,
      cloneDir,
      target.targetCommit,
      target.version,
      target.theirsEntries,
    );
    console.log(`\nUpdated ${BASELINE_REF} → ${target.targetCommit.slice(0, 7)}`);
  }

  const commitChanged = !shaMatches(config.last_sync.commit, target.targetCommit);
  const versionChanged =
    target.version !== null && target.version !== (config.last_sync.version ?? null);
  if (commitChanged || versionChanged) {
    await saveConfig(projectRoot, {
      ...config,
      last_sync: {
        version: target.version ?? config.last_sync.version,
        commit: target.targetCommit,
        date: new Date().toISOString(),
      },
    });
    console.log("Updated .groot/boilerplate-sync.json");
  }
}

// ============================================================
// REPORT
// ============================================================

export interface SyncReport {
  fromVersion: string | null;
  toVersion: string | null;
  fromCommit: string;
  toCommit: string;
  changelog: string[];
  breakingChanges: string[];
  autoApply: string[];
  autoMerged: string[];
  conflicts: { file: string; conflicts: number; commits: string[] }[];
  binaryConflicts: string[];
  deleted: string[];
  reviewDeletions: string[];
  keptLocalDeletions: string[];
  drift: string[];
  skipped: { immutable: string[]; appSpecific: string[] };
}

async function writeSyncReport(projectRoot: string, result: SyncResult): Promise<void> {
  const report: SyncReport = {
    fromVersion: result.fromVersion,
    toVersion: result.toVersion,
    fromCommit: result.fromCommit,
    toCommit: result.toCommit,
    changelog: result.changelog,
    breakingChanges: result.breakingChanges,
    autoApply: result.autoApply.map((f) => f.file),
    autoMerged: result.autoMerged.map((f) => f.file),
    conflicts: result.conflicts.map((c) => ({
      file: c.file,
      conflicts: c.conflicts,
      commits: c.commits,
    })),
    binaryConflicts: result.binaryConflicts,
    deleted: result.deleted,
    reviewDeletions: result.reviewDeletions,
    keptLocalDeletions: result.keptLocalDeletions,
    drift: result.drift,
    skipped: result.skipped,
  };
  await writeFile(join(projectRoot, REPORT_RELATIVE_PATH), JSON.stringify(report, null, 2) + "\n");
}

// ============================================================
// UPSTREAM - the same reconciliation with the sides swapped
// ============================================================

/**
 * What a child's change does to groot. Groot is the side being written
 * (ours = groot HEAD), the child's copy is the incoming change (theirs), and
 * the last sync is the base — so groot's changes since then are kept and
 * overlapping edits surface as conflicts instead of being reverted.
 */
export type UpstreamOutcome =
  | { kind: "ready"; content: Buffer | string }
  | { kind: "up-to-date" }
  | { kind: "conflict"; conflicts: number }
  | { kind: "binary-conflict" }
  | { kind: "deleted-in-groot" };

export interface UpstreamItem {
  file: string;
  /** Groot at the last sync. */
  base: Buffer;
  /** The child's working-tree copy. */
  local: Buffer;
  /** Groot HEAD the plan was computed against, or null when groot deleted the file. */
  groot: Buffer | null;
  outcome: UpstreamOutcome;
}

export interface UpstreamPlan {
  baseCommit: string;
  grootCommit: string;
  /** Synced files the child changed since the last sync that groot does not already have. */
  items: UpstreamItem[];
}

export async function mergeIntoGroot(
  file: string,
  base: Buffer,
  local: Buffer,
  groot: Buffer | null,
): Promise<UpstreamOutcome> {
  const outcome = await reconcileFile(
    { path: file, ours: groot, base, theirs: local },
    gitMergeFile,
  );
  switch (outcome.kind) {
    case "identical":
      return { kind: "up-to-date" };
    case "auto-apply":
    case "auto-merged":
      return { kind: "ready", content: outcome.content };
    case "conflict":
      return { kind: "conflict", conflicts: outcome.conflicts };
    case "binary-conflict":
      return { kind: "binary-conflict" };
    case "kept-local-deletion":
      return { kind: "deleted-in-groot" };
    default:
      // The remaining outcomes need a missing base or child copy, which
      // upstream never passes.
      throw new Error(`Unexpected ${outcome.kind} reconciling ${file} for upstream`);
  }
}

/** Every synced file the child changed since the last sync, merged onto `grootDir` HEAD. */
export async function planUpstream(projectRoot: string, grootDir: string): Promise<UpstreamPlan> {
  const config = await loadConfig(projectRoot);
  const [grootCommit, base] = await Promise.all([
    resolveCommit(grootDir, "HEAD"),
    openMergeBase(projectRoot, grootDir, config.last_sync.commit),
  ]);
  console.log(`Base: ${base.description}`);

  const baseEntries = syncedBlobs(await base.entries(), config.additional_exclusions);
  const items: UpstreamItem[] = [];
  await Promise.all(
    [...baseEntries.values()].map(async (entry) => {
      // Local deletions are not upstreamed.
      const local = await readFile(join(projectRoot, entry.path)).catch(() => null);
      // Hashing locally skips a git call per unmodified file; a mismatch only
      // means "compare the bytes".
      if (local === null || blobSha(local) === entry.sha) return;
      const baseContent = await base.read(entry.path);
      if (baseContent === null || local.equals(baseContent)) return;

      const groot = await showBlob(grootDir, grootCommit, entry.path);
      const outcome = await mergeIntoGroot(entry.path, baseContent, local, groot);
      if (outcome.kind === "up-to-date") return;
      items.push({ file: entry.path, base: baseContent, local, groot, outcome });
    }),
  );
  items.sort((a, b) => a.file.localeCompare(b.file));

  return { baseCommit: base.commit, grootCommit, items };
}

/**
 * Write `items` into the groot working tree at `grootDir`, re-merging each
 * against that tree's copy so the result is exact even when it is not the
 * commit the plan was computed against. Nothing is written when any item no
 * longer merges cleanly; those files are returned in `unmerged`.
 */
export async function applyUpstream(
  grootDir: string,
  items: UpstreamItem[],
): Promise<{ written: string[]; unmerged: string[] }> {
  const writes: { file: string; content: Buffer | string }[] = [];
  const unmerged: string[] = [];
  for (const item of items) {
    const groot = await readFile(join(grootDir, item.file)).catch(() => null);
    const outcome = await mergeIntoGroot(item.file, item.base, item.local, groot);
    if (outcome.kind === "ready") writes.push({ file: item.file, content: outcome.content });
    else if (outcome.kind !== "up-to-date") unmerged.push(item.file);
  }
  if (unmerged.length > 0) return { written: [], unmerged };

  for (const { file, content } of writes) {
    await writeFile(join(grootDir, file), content);
  }
  return { written: writes.map((w) => w.file), unmerged };
}
