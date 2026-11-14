import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";

export const ROOT = join(import.meta.dirname, "..", "..");

export type Violation = { file: string; line?: number; message: string };

/** Repo-relative POSIX path. */
export const rel = (abs: string) => relative(ROOT, abs).split(sep).join("/");

const SKIP_DIRS = new Set(["node_modules", "build", ".gradle", ".git", ".idea", "DerivedData", ".build"]);

/** All files under `dir` (repo-relative) whose name passes `filter`. Missing dirs yield nothing. */
export function walk(dir: string, filter: (name: string) => boolean): string[] {
  const abs = join(ROOT, dir);
  if (!existsSync(abs)) return [];
  const out: string[] = [];
  const visit = (d: string) => {
    for (const name of readdirSync(d)) {
      const p = join(d, name);
      if (statSync(p).isDirectory()) {
        if (!SKIP_DIRS.has(name)) visit(p);
      } else if (filter(name)) {
        out.push(rel(p));
      }
    }
  };
  visit(abs);
  return out;
}

export const read = (file: string) => readFileSync(join(ROOT, file), "utf8");

export const exists = (file: string) => existsSync(join(ROOT, file));

/** A child's `additional_exclusions` (e.g. ["ios/**"] for an Android-only app). */
export function additionalExclusions(): string[] {
  const path = ".groot/boilerplate-sync.json";
  if (!exists(path)) return [];
  return (JSON.parse(read(path)).additional_exclusions as string[] | undefined) ?? [];
}

export function git(...args: string[]): string {
  return execFileSync("git", args, { cwd: ROOT, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
}

/** Print violations and exit non-zero if there are any. */
export function report(name: string, violations: Violation[], okMessage: string): void {
  if (violations.length === 0) {
    console.log(`✓ ${name}: ${okMessage}`);
    return;
  }
  console.error(`✗ ${name}: ${violations.length} problem(s)`);
  for (const v of violations) {
    console.error(`  ${v.file}${v.line ? `:${v.line}` : ""}  ${v.message}`);
  }
  process.exitCode = 1;
}

/** Each line of a file with its 1-based number, comments stripped of nothing (callers decide). */
export function lines(file: string): Array<[number, string]> {
  return read(file)
    .split("\n")
    .map((text, i) => [i + 1, text] as [number, string]);
}
