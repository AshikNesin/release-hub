/**
 * check:doc-refs: every repo path a doc mentions exists, so agents can trust the docs.
 * Checks markdown links and backticked paths under known roots. Paths matched by a
 * child's additional_exclusions count as present (RFC 0001 §7). RFCs describe future
 * state, so docs/rfcs/** is not checked; children don't have it (new-app.sh drops it), so
 * links into it count as present. Root dot-folders other than .github/ and .groot/ hold
 * tool config and vendored agent skills (.agents/, .claude/, …), not this repo's docs.
 * Synced docs (docs/ outside docs/app/) point at the reference app's code; a child's app code
 * is its own, so those links are checked only in the boilerplate (the repo that has docs/rfcs/).
 */
import micromatch from "micromatch";
import { additionalExclusions, exists, lines, report, walk, type Violation } from "./lib.ts";

const isBoilerplate = exists("docs/rfcs");
const APP_CODE = ["android/app/", "ios/App/", "ios/AppTests/", "ios/AppUITests/"];
const isSyncedDoc = (f: string) => f.startsWith("docs/") && !f.startsWith("docs/app/");

const ROOTS = ["android/", "ios/", "docs/", "scripts/", ".groot/", ".github/", ".changeset/"];
const ROOT_FILES = new Set(["AGENTS.md", "README.md", "package.json", "CHANGELOG.md", ".gitignore", ".editorconfig"]);
const exclusions = additionalExclusions();

const OWN_DOT_DIRS = [".github/", ".groot/"];
const isVendored = (f: string) => f.startsWith(".") && f.includes("/") && !OWN_DOT_DIRS.some((d) => f.startsWith(d));

const docs = walk(".", (n) => n.endsWith(".md")).filter(
  (f) => !f.startsWith("docs/rfcs/") && !f.startsWith(".changeset/") && f !== "CHANGELOG.md" && !isVendored(f),
);

const looksLikePath = (p: string) =>
  (ROOTS.some((r) => p.startsWith(r)) || ROOT_FILES.has(p)) && !/[*<>…{}\s]/.test(p) && !p.includes("://");

const violations: Violation[] = [];
for (const file of docs) {
  for (const [no, text] of lines(file)) {
    const candidates = [
      ...[...text.matchAll(/\]\(([^)#\s]+)/g)].map((m) => m[1]),
      ...[...text.matchAll(/`([^`\s]+)`/g)].map((m) => m[1]),
    ];
    for (const raw of candidates) {
      const path = raw.replace(/^\.\//, "").replace(/[:,.;]+$/, "").replace(/\/$/, "");
      if (!looksLikePath(path)) continue;
      if (micromatch.isMatch(path, exclusions, { dot: true })) continue;
      if (path.startsWith("docs/rfcs/")) continue;
      if (!isBoilerplate && isSyncedDoc(file) && APP_CODE.some((r) => path.startsWith(r))) continue;
      if (!exists(path)) violations.push({ file, line: no, message: `refers to missing path: ${path}` });
    }
  }
}

report("check:doc-refs", violations, `${docs.length} docs, every referenced path exists`);
