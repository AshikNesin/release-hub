/**
 * check:kit-edits (RFC 0001 §5.3): warns about every KIT file a child changed since the
 * last sync. groot's engine would silently auto-merge a non-overlapping edit, so this
 * makes drift from the kit visible before it becomes a merge. Warns; never fails.
 * Kit fixes go upstream with `pnpm groot:upstream`.
 */
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { categorizeFile, FORCE_MERGE_FILES } from "../../.groot/lib/patterns.ts";
import { additionalExclusions, exists, git, ROOT } from "./lib.ts";

const BASELINE = "refs/groot/baseline";

let tree: string;
try {
  // --full-tree: the app may sit in a subdirectory of its repo (monorepo); baseline paths start at the app root.
  tree = git("ls-tree", "-r", "--full-tree", BASELINE);
} catch {
  console.log("✓ check:kit-edits: no sync baseline yet (the boilerplate itself, or a child before its first sync); skipping");
  process.exit(0);
}

const exclusions = additionalExclusions();
const gitBlobSha = (buf: Buffer) =>
  createHash("sha1").update(`blob ${buf.length}\0`).update(buf).digest("hex");

const edited: string[] = [];
for (const line of tree.split("\n").filter(Boolean)) {
  const [meta, path] = line.split("\t");
  const sha = meta.split(" ")[2];
  if (FORCE_MERGE_FILES.includes(path)) continue; // SHARED: children edit these by design.
  if (categorizeFile(path, exclusions).action !== "sync") continue;
  if (!exists(path)) continue; // Local deletions are reported by sync itself.
  if (gitBlobSha(readFileSync(join(ROOT, path))) !== sha) edited.push(path);
}

if (edited.length === 0) {
  console.log("✓ check:kit-edits: no kit files edited since the last sync");
} else {
  console.warn(`⚠ check:kit-edits: ${edited.length} kit file(s) differ from the last sync. Kit files are read-only in children;`);
  console.warn("  move the change into app code, or send it upstream with `pnpm groot:upstream`.");
  for (const path of edited) console.warn(`  ${path}`);
}
