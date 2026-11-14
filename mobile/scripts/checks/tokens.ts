/**
 * check:tokens (RFC 0001 §4): screens read colors and spacing from the theme.
 * Color literals and raw dp/point values are allowed only in the kit's token files
 * and the app's brand folder.
 */
import { lines, report, walk, type Violation } from "./lib.ts";

const ALLOWED = [/^android\/kit\/src\/main\/kotlin\/groot\/kit\/tokens\//, /\/ui\/brand\//, /^ios\/GrootKit\/Sources\/GrootKit\/Tokens\//, /\/UI\/Brand\//];
const allowed = (file: string) => ALLOWED.some((re) => re.test(file));

const KOTLIN_RULES: Array<[RegExp, string]> = [
  [/\bColor\(\s*0x[0-9A-Fa-f]+/, "hex color literal; use GrootTheme.colors / MaterialTheme.colorScheme"],
  [/\bColor\(\s*(red|green|blue)\s*=/, "color literal; use theme colors"],
  [/(?<![\w.])\d+(\.\d+)?\.dp\b/, "raw dp value; use S.* spacing tokens"],
];

const SWIFT_RULES: Array<[RegExp, string]> = [
  [/Color\(\s*red:/, "color literal; use GrootColors"],
  [/#colorLiteral/, "color literal; use GrootColors"],
  [/\.padding\(\s*\d+(\.\d+)?\s*\)/, "raw spacing value; use S tokens"],
];

const violations: Violation[] = [];

const scan = (files: string[], rules: Array<[RegExp, string]>) => {
  for (const file of files.filter((f) => !allowed(f))) {
    for (const [no, text] of lines(file)) {
      const code = text.replace(/\/\/.*$/, "");
      for (const [pattern, why] of rules) {
        if (pattern.test(code)) violations.push({ file, line: no, message: why });
      }
    }
  }
};

scan(walk("android", (n) => n.endsWith(".kt")).filter((f) => f.includes("/src/main/")), KOTLIN_RULES);
scan(walk("ios", (n) => n.endsWith(".swift")).filter((f) => !/Tests\//.test(f)), SWIFT_RULES);

report("check:tokens", violations, "no hardcoded colors or spacing outside tokens and brand/");
