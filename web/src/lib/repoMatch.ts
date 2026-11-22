/**
 * Guessing which GitHub repository publishes an app, and fuzzy search over
 * repository names, for the Keyless card's repo picker.
 *
 * Relevance is a blend of three signals between the app (slug, package ids)
 * and each repo name:
 *   - TF-IDF cosine over word tokens, with IDF over the listed repos, so a
 *     shared rare word ("goldview") counts and a common one ("android",
 *     "app") barely does;
 *   - character-trigram (Dice) similarity of the squashed names, which
 *     catches "gold-view" vs "goldview" and small spelling differences;
 *   - bonuses for an exact squashed match and for one name containing the
 *     other.
 */

/** Words that say nothing about which repo it is (package-id noise, platform words). */
const STOPWORDS = new Set([
  "com", "org", "io", "net", "dev", "co", "in", "me", "app", "apps", "android", "ios",
  "mobile", "example", "www", "the", "and", "client",
]);

/** Splits "myApp-Android_v2.client" → ["my", "app", "android", "v2", "client"]. */
export function words(s: string): string[] {
  return s
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter(Boolean);
}

/** Lowercase letters and digits only: "Gold-View" → "goldview". */
export function squash(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]/g, "");
}

function trigrams(s: string): Map<string, number> {
  const out = new Map<string, number>();
  const padded = `  ${s} `;
  for (let i = 0; i + 3 <= padded.length; i++) {
    const g = padded.slice(i, i + 3);
    out.set(g, (out.get(g) ?? 0) + 1);
  }
  return out;
}

/** Sørensen–Dice coefficient over character trigrams, 0..1. */
export function dice(a: string, b: string): number {
  if (!a || !b) return 0;
  const ta = trigrams(a);
  const tb = trigrams(b);
  let shared = 0;
  let total = 0;
  for (const [g, n] of ta) {
    shared += Math.min(n, tb.get(g) ?? 0);
    total += n;
  }
  for (const n of tb.values()) total += n;
  return total ? (2 * shared) / total : 0;
}

/**
 * The names an app goes by: its slug and the meaningful tail of each
 * package id (com.example.gold_view → "gold_view"; reverse-DNS prefixes
 * and platform words dropped).
 */
export function appNames(slug: string, packageNames: string[]): string[] {
  const names = new Set<string>([slug]);
  for (const pkg of packageNames) {
    const parts = pkg.split(".").filter(Boolean);
    const meaningful = parts.filter((p, i) => !(i < 2 && parts.length > 2) && !STOPWORDS.has(p.toLowerCase()));
    if (meaningful.length) names.add(meaningful[meaningful.length - 1]);
    if (meaningful.length > 1) names.add(meaningful.join("-"));
  }
  return [...names].filter((n) => squash(n).length > 0);
}

export interface Scored<T> {
  item: T;
  score: number;
}

/**
 * Ranks repos by how likely they publish the app, best first. `name` gives
 * a repo's "owner/name". Scores are roughly 0..3; see SUGGEST_THRESHOLD.
 */
export function rankByRelevance<T>(repos: T[], name: (r: T) => string, names: string[]): Scored<T>[] {
  // Each name's words, plus the whole name squashed, so "tiny-firewall" and
  // "tinyfirewall" share a token.
  const bag = (s: string) => [...words(s).filter((w) => !STOPWORDS.has(w)), squash(s)];
  const repoWords = repos.map((r) => bag(repoName(name(r))));
  // IDF over the repo list: a word in every repo carries no signal.
  const df = new Map<string, number>();
  for (const ws of repoWords) for (const w of new Set(ws)) df.set(w, (df.get(w) ?? 0) + 1);
  const idf = (w: string) => Math.log((repos.length + 1) / ((df.get(w) ?? 0) + 1)) + 1;

  const queryWords = names.flatMap(bag);
  const qVec = tfidf(queryWords, idf);
  const qSquashed = [...new Set(names.map(squash))].filter((s) => s.length > 0);

  return repos
    .map((item, i) => {
      const rName = squash(repoName(name(item)));
      const cosine = cosineSim(qVec, tfidf(repoWords[i], idf));
      let best = 0;
      for (const q of qSquashed) {
        let s = 0.6 * dice(q, rName);
        if (q === rName) s += 1.5;
        else if (Math.min(q.length, rName.length) >= 4 && (rName.includes(q) || q.includes(rName))) s += 0.5;
        best = Math.max(best, s);
      }
      return { item, score: cosine + best };
    })
    .sort((a, b) => b.score - a.score);
}

/** Below this, a repo is not worth suggesting. */
export const SUGGEST_THRESHOLD = 0.6;

/**
 * The repo to pre-select: the best match, when it is a confident one and
 * clearly ahead of the runner-up.
 */
export function confidentPick<T>(ranked: Scored<T>[]): T | null {
  const [first, second] = ranked;
  if (!first || first.score < 1) return null;
  if (second && first.score - second.score < 0.25) return null;
  return first.item;
}

/**
 * Fuzzy match of a typed query against "owner/name": 0 when it doesn't
 * match; higher for an exact name, a prefix, a substring, then a
 * subsequence (letters in order, like "gv" → "goldview").
 */
export function queryScore(query: string, fullName: string): number {
  const q = query.trim().toLowerCase();
  if (!q) return 1;
  const full = fullName.toLowerCase();
  const name = repoName(full);
  if (name === q || full === q) return 100;
  if (name.startsWith(q)) return 80;
  if (full.startsWith(q)) return 70;
  if (name.includes(q)) return 60;
  if (full.includes(q)) return 50;
  const qs = squash(q);
  const ns = squash(full);
  if (ns.includes(qs)) return 45;
  // Subsequence: every query character in order, fewer gaps score higher.
  let j = 0;
  let gaps = 0;
  for (let i = 0; i < ns.length && j < qs.length; i++) {
    if (ns[i] === qs[j]) j++;
    else if (j > 0) gaps++;
  }
  if (j < qs.length) return 0;
  return Math.max(1, 40 - gaps);
}

function repoName(fullName: string): string {
  const i = fullName.indexOf("/");
  return i === -1 ? fullName : fullName.slice(i + 1);
}

function tfidf(ws: string[], idf: (w: string) => number): Map<string, number> {
  const tf = new Map<string, number>();
  for (const w of ws) tf.set(w, (tf.get(w) ?? 0) + 1);
  for (const [w, n] of tf) tf.set(w, n * idf(w));
  return tf;
}

function cosineSim(a: Map<string, number>, b: Map<string, number>): number {
  let dot = 0;
  let na = 0;
  let nb = 0;
  for (const [w, x] of a) {
    na += x * x;
    const y = b.get(w);
    if (y) dot += x * y;
  }
  for (const y of b.values()) nb += y * y;
  return na && nb ? dot / Math.sqrt(na * nb) : 0;
}
