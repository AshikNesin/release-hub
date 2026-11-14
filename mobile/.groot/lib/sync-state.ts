/**
 * What the last sync left behind, and how every groot tool reads it.
 *
 * `groot:sync`, `groot:resolve` and `groot:upstream` all reason about the
 * same three versions of a file (base = groot at the last sync, the child's
 * working tree, groot HEAD). This module owns the two pieces of state they
 * share: where the base comes from, and the conflict manifest sync hands to
 * resolve.
 */
import { readFile, writeFile, mkdir, rm } from "node:fs/promises";
import { join, dirname } from "node:path";
import { z } from "zod";
import { hasCommit, listTree, resolveCommit, shaMatches, showBlob, type TreeEntry } from "./git";
import { readBaselineInfo, BASELINE_REF } from "./baseline";

// ============================================================
// MERGE BASE
// ============================================================

export interface MergeBase {
  /** The last-synced boilerplate commit (full SHA when the checkout has it). */
  commit: string;
  /** Whether `commit` is in the checkout's history (changelog, commit lists). */
  reachable: boolean;
  description: string;
  entries(): Promise<TreeEntry[]>;
  read(path: string): Promise<Buffer | null>;
}

/**
 * The base for three-way merges at `lastSyncCommit`. Prefers the local
 * baseline snapshot (no network, survives upstream history rewrites) and falls
 * back to the boilerplate checkout at that commit. Throws when neither has it.
 */
export async function openMergeBase(
  projectRoot: string,
  cloneDir: string,
  lastSyncCommit: string,
): Promise<MergeBase> {
  const reachable = await hasCommit(cloneDir, lastSyncCommit);
  const commit = reachable ? await resolveCommit(cloneDir, lastSyncCommit) : lastSyncCommit;

  const baseline = await readBaselineInfo(projectRoot);
  if (baseline !== null && shaMatches(baseline.boilerplateCommit, commit)) {
    return {
      commit,
      reachable,
      description: `local baseline snapshot (${BASELINE_REF})`,
      entries: () => listTree(projectRoot, BASELINE_REF),
      read: (path) => showBlob(projectRoot, BASELINE_REF, path),
    };
  }
  if (reachable) {
    return {
      commit,
      reachable,
      description: `boilerplate checkout at ${commit.slice(0, 7)}`,
      entries: () => listTree(cloneDir, commit),
      read: (path) => showBlob(cloneDir, commit, path),
    };
  }
  throw new Error(
    [
      `Last sync commit ${lastSyncCommit} is not reachable in the boilerplate`,
      `checkout at ${cloneDir}, and no matching local baseline snapshot (${BASELINE_REF})`,
      `exists. Run \`git fetch --unshallow\` (or a full fetch) in the boilerplate checkout,`,
      `or fix last_sync.commit in .groot/boilerplate-sync.json.`,
    ].join("\n"),
  );
}

// ============================================================
// CONFLICT MANIFEST
// ============================================================

export const MANIFEST_RELATIVE_PATH = ".groot/needs-review/manifest.json";

const ConflictManifestSchema = z.object({
  fromVersion: z.string().nullable(),
  toVersion: z.string().nullable(),
  fromCommit: z.string(),
  toCommit: z.string(),
  generatedAt: z.string(),
  conflicts: z.array(
    z.object({
      file: z.string(),
      conflicts: z.number(),
      commits: z.array(z.string()).default([]),
    }),
  ),
});

export type ConflictManifest = z.infer<typeof ConflictManifestSchema>;

/**
 * The pending conflict manifest, or null when there is none. A manifest that
 * exists but cannot be read or parsed is an error, not "nothing pending".
 */
export async function readManifest(projectRoot: string): Promise<ConflictManifest | null> {
  const path = join(projectRoot, MANIFEST_RELATIVE_PATH);
  let raw: string;
  try {
    raw = await readFile(path, "utf-8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return null;
    throw new Error(`Cannot read the conflict manifest at ${path}`, { cause: error });
  }
  try {
    return ConflictManifestSchema.parse(JSON.parse(raw));
  } catch (error) {
    throw new Error(`Invalid conflict manifest at ${path}: ${(error as Error).message}`, {
      cause: error,
    });
  }
}

/** Write the manifest; a manifest with no conflicts left is removed instead. */
export async function writeManifest(
  projectRoot: string,
  manifest: ConflictManifest,
): Promise<void> {
  const path = join(projectRoot, MANIFEST_RELATIVE_PATH);
  if (manifest.conflicts.length === 0) {
    await rm(path, { force: true });
    return;
  }
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, JSON.stringify(manifest, null, 2) + "\n");
}
