/**
 * Deterministic 3-way merge for `package.json`.
 *
 * `package.json` is the highest-risk file to hand to an LLM editor: it changes
 * in every boilerplate release (so it conflicts often) and a malformed result
 * — a trailing comma, a duplicate key, an unbalanced brace — breaks the whole
 * toolchain. This module merges it programmatically so the result is always
 * valid JSON, built from parsed objects.
 *
 * Scope: only `scripts`, `dependencies`, `devDependencies` are 3-way-merged;
 * every other top-level key (`name`, `version`, `private`, `engines`,
 * `packageManager`, pnpm overrides, …) is copied verbatim from the local file.
 *
 * Per-key semantics:
 *   - unchanged locally since base (ours[k] === base[k]) → adopt theirs[k];
 *     if theirs deleted k, accept the deletion.
 *   - changed locally (ours[k] !== base[k]) → keep ours[k] (local wins).
 *   - net-new in ours (not in base) → keep ours[k].
 *   - net-new in theirs (not in ours, not in base) → add theirs[k].
 *   - deleted locally (in base, not in ours) → stay deleted (local wins).
 *
 * This module is pure (no filesystem, no network, no side effects) so it can
 * be unit-tested in isolation. `reconcile.ts` calls it for the root
 * `package.json` during sync and upstream.
 */

export const PACKAGE_JSON_FILENAME = "package.json";

/** The only top-level keys that flow boilerplate → child on sync. */
export const PACKAGE_JSON_MERGE_KEYS = ["scripts", "dependencies", "devDependencies"] as const;

/** A plain, non-null, non-array object. */
export function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** Detect the indentation (spaces or a tab) of a JSON document. */
export function detectIndent(content: string): string {
  const m = content.match(/^([ \t]+)"/m);
  return m ? m[1] : "  ";
}

function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (typeof a !== typeof b) return false;
  if (isPlainObject(a) && isPlainObject(b)) {
    const ak = Object.keys(a);
    const bk = Object.keys(b);
    if (ak.length !== bk.length) return false;
    return ak.every((k) => k in b && deepEqual(a[k], (b as Record<string, unknown>)[k]));
  }
  return false;
}

/**
 * 3-way merge of a flat object (a `scripts` / `dependencies` section).
 *
 * Ordering: ours's keys first (in their original order), then keys introduced
 * only by theirs.
 */
export function mergeObject3Way(
  ours: Record<string, unknown>,
  base: Record<string, unknown>,
  theirs: Record<string, unknown>,
): Record<string, unknown> {
  const result: Record<string, unknown> = {};

  for (const k of Object.keys(ours)) {
    const inBase = k in base;
    const ourVal = ours[k];
    if (!inBase) {
      // Net-new in ours (or both sides added it) — keep ours.
      result[k] = ourVal;
    } else if (ourVal === base[k] || deepEqual(ourVal, base[k])) {
      // Unchanged locally — adopt theirs, including upstream deletion.
      if (k in theirs) result[k] = theirs[k];
    } else {
      // Changed locally — local wins.
      result[k] = ourVal;
    }
  }

  for (const k of Object.keys(theirs)) {
    if (!(k in result) && !(k in ours) && !(k in base)) {
      result[k] = theirs[k];
    }
  }

  return result;
}

export type PackageJsonMergeResult = { ok: true; text: string } | { ok: false; reason: string };

/**
 * Deterministically merge three `package.json` documents into one.
 *
 * Only `scripts` / `dependencies` / `devDependencies` are 3-way-merged; every
 * other top-level key is copied verbatim from `ours`. Formatting (indent +
 * trailing newline) is inferred from `ours`. Returns `{ ok: false }` on any
 * anomaly (unparseable JSON, non-object sides, non-object managed sections) so
 * the caller can fall back to a marker-based merge rather than writing a broken file.
 */
export function mergePackageJson(
  oursText: string,
  baseText: string,
  theirsText: string,
): PackageJsonMergeResult {
  let ours: unknown;
  let base: unknown;
  let theirs: unknown;
  try {
    ours = JSON.parse(oursText);
    base = JSON.parse(baseText);
    theirs = JSON.parse(theirsText);
  } catch (e) {
    return { ok: false, reason: `JSON parse failed: ${(e as Error).message}` };
  }

  if (!isPlainObject(ours) || !isPlainObject(base) || !isPlainObject(theirs)) {
    return { ok: false, reason: "package.json sides must be JSON objects" };
  }

  // Managed sections must be plain objects wherever they appear.
  const sides: Array<["ours" | "base" | "theirs", Record<string, unknown>]> = [
    ["ours", ours],
    ["base", base],
    ["theirs", theirs],
  ];
  for (const key of PACKAGE_JSON_MERGE_KEYS) {
    for (const [side, obj] of sides) {
      if (key in obj && !isPlainObject(obj[key])) {
        return { ok: false, reason: `package.json ${side}.${key} is not an object` };
      }
    }
  }

  // Start from a verbatim copy of ours — every project-owned key is untouched.
  const result: Record<string, unknown> = { ...ours };

  for (const key of PACKAGE_JSON_MERGE_KEYS) {
    const sectionInOurs = key in ours;
    const sectionInBase = key in base;

    // Local deleted the whole section after base had it — respect the deletion.
    if (!sectionInOurs && sectionInBase) {
      delete result[key];
      continue;
    }

    const oursSection = isPlainObject(ours[key]) ? ours[key] : {};
    const baseSection = isPlainObject(base[key]) ? base[key] : {};
    const theirsSection = isPlainObject(theirs[key]) ? theirs[key] : {};
    const merged = mergeObject3Way(oursSection, baseSection, theirsSection);

    if (Object.keys(merged).length === 0) {
      // Drop empty managed sections rather than leaving `{}`.
      delete result[key];
    } else {
      result[key] = merged;
    }
  }

  const indent = detectIndent(oursText);
  const trailingNewline = oursText.endsWith("\n");
  const text = JSON.stringify(result, null, indent) + (trailingNewline ? "\n" : "");
  return { ok: true, text };
}
