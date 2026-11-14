/**
 * A child app can live in a subdirectory of a bigger repo (a monorepo, e.g. a server repo
 * with its mobile app in `mobile/`). Sync must read the baseline snapshot by its full tree:
 * `git ls-tree` run from a subdirectory otherwise lists only that subdirectory, so the
 * baseline reads as empty and every kit file the child edited would turn into a conflict.
 */
import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { after, before, test } from "node:test";
import { runSync } from "../lib/engine.ts";
import { exec } from "../lib/git.ts";

const ID = ["-c", "user.name=test", "-c", "user.email=test@localhost", "-c", "commit.gpgsign=false"];
const git = (dir: string, ...args: string[]) => exec("git", ["-C", dir, ...ID, ...args]);

async function put(dir: string, files: Record<string, string>): Promise<void> {
  for (const [path, content] of Object.entries(files)) {
    await mkdir(dirname(join(dir, path)), { recursive: true });
    await writeFile(join(dir, path), content);
  }
}

let root: string;
let boilerplate: string;
let monorepo: string;
let app: string;

before(async () => {
  root = await mkdtemp(join(tmpdir(), "groot-subdir-test-"));
  boilerplate = join(root, "boilerplate");
  monorepo = join(root, "server");
  app = join(monorepo, "mobile");

  // The boilerplate: two kit files and an app file.
  await mkdir(boilerplate);
  await git(boilerplate, "init", "-q", "-b", "main");
  await put(boilerplate, {
    "android/kit/Theme.kt": "line 1\nline 2\nline 3\nline 4\nline 5\n",
    "android/kit/Tokens.kt": "tokens v1\n",
    "android/app/Main.kt": "boilerplate app\n",
  });
  await git(boilerplate, "add", "-A");
  await git(boilerplate, "commit", "-q", "-m", "init");
  const { stdout } = await git(boilerplate, "rev-parse", "HEAD");

  // A server repo with the app in mobile/.
  await mkdir(monorepo);
  await git(monorepo, "init", "-q", "-b", "main");
  await put(monorepo, {
    "go.mod": "module server\n",
    "mobile/android/kit/Theme.kt": "line 1\nline 2\nline 3\nline 4\nline 5\n",
    "mobile/android/kit/Tokens.kt": "tokens v1\n",
    "mobile/android/app/Main.kt": "my app\n",
    "mobile/.groot/boilerplate-sync.json": JSON.stringify({
      // A unique name, so acquire never reuses a real ~/Code checkout.
      boilerplate: { name: `groot-subdir-test-${process.pid}`, repo: `file://${boilerplate}` },
      last_sync: { commit: stdout.trim(), date: "2026-01-01T00:00:00Z" },
      additional_exclusions: [],
    }),
  });
  await git(monorepo, "add", "-A");
  await git(monorepo, "commit", "-q", "-m", "server with mobile app");
});

after(async () => {
  await rm(root, { recursive: true, force: true });
});

test("first sync from a subdirectory applies upstream changes and records the baseline", async () => {
  await put(boilerplate, { "android/kit/Tokens.kt": "tokens v2\n" });
  await git(boilerplate, "commit", "-q", "-am", "tokens v2");

  const result = await runSync(app, { mode: "apply", skipConflicts: false, force: false });
  assert.deepEqual(result.autoApply.map((f) => f.file), ["android/kit/Tokens.kt"]);
  assert.equal(await readFile(join(app, "android/kit/Tokens.kt"), "utf8"), "tokens v2\n");
  // App code and files outside the app are untouched.
  assert.equal(await readFile(join(app, "android/app/Main.kt"), "utf8"), "my app\n");
  assert.equal(await readFile(join(monorepo, "go.mod"), "utf8"), "module server\n");
  await git(monorepo, "add", "-A");
  await git(monorepo, "commit", "-q", "-m", "sync");
});

test("later syncs merge against the baseline: a local kit edit and an upstream edit both survive", async () => {
  await put(app, { "android/kit/Theme.kt": "line 1 (local)\nline 2\nline 3\nline 4\nline 5\n" });
  await git(monorepo, "commit", "-q", "-am", "local kit fix");
  await put(boilerplate, { "android/kit/Theme.kt": "line 1\nline 2\nline 3\nline 4\nline 5 (upstream)\n" });
  await git(boilerplate, "commit", "-q", "-am", "upstream theme change");

  const result = await runSync(app, { mode: "apply", skipConflicts: false, force: false });
  assert.deepEqual(result.conflicts, []);
  assert.deepEqual(result.autoMerged.map((f) => f.file), ["android/kit/Theme.kt"]);
  assert.equal(
    await readFile(join(app, "android/kit/Theme.kt"), "utf8"),
    "line 1 (local)\nline 2\nline 3\nline 4\nline 5 (upstream)\n",
  );
  await git(monorepo, "add", "-A");
  await git(monorepo, "commit", "-q", "-m", "sync 2");

  const again = await runSync(app, { mode: "check", skipConflicts: false, force: false });
  assert.equal(again.autoApply.length + again.autoMerged.length + again.conflicts.length, 0);
});
