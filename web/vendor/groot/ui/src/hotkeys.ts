"use client";

import { useEffect, useLayoutEffect, useRef, useSyncExternalStore, type RefObject } from "react";

// ---------------------------------------------------------------------------
// Hotkeys: the one keyboard-shortcut module.
//
// - The grammar: one string syntax for the combos the app binds and the caps
//   it displays (`parseShortcut`, `matchesShortcut`, `formatShortcut`).
// - `isEditable`: whether a keydown came from a text field.
// - `useHotkey`: binds a combo. Every binding shares one window listener and
//   one precedence, so a press runs at most one handler (see `pickBinding`).
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Grammar. Tokens join with "+": modifiers first, then one key.
//   "mod+k"        ⌘K on a Mac, Ctrl+K elsewhere
//   "mod+shift+p"  ⌘⇧P / Ctrl+Shift+P
//   "/"            a bare key
// Pre-formatted caps ("⌘K", "⌘⇧P") display as-is, one cap per glyph.
// ---------------------------------------------------------------------------

export interface ParsedShortcut {
  /** ⌘ on a Mac, Ctrl elsewhere. Matches either while listening, so a Ctrl
   *  press on a Mac still opens the menu. */
  mod: boolean;
  meta: boolean;
  ctrl: boolean;
  alt: boolean;
  shift: boolean;
  /** Lowercase `KeyboardEvent.key` ("k", "/", "escape"). */
  key: string;
}

const MODIFIER_TOKENS: Record<string, keyof Omit<ParsedShortcut, "key">> = {
  mod: "mod",
  cmd: "meta",
  command: "meta",
  meta: "meta",
  win: "meta",
  super: "meta",
  ctrl: "ctrl",
  control: "ctrl",
  alt: "alt",
  option: "alt",
  opt: "alt",
  shift: "shift",
};

/** Named keys, as `KeyboardEvent.key` spells them (lowercased). */
const KEY_ALIASES: Record<string, string> = {
  esc: "escape",
  return: "enter",
  space: " ",
  spacebar: " ",
  up: "arrowup",
  down: "arrowdown",
  left: "arrowleft",
  right: "arrowright",
  del: "delete",
  plus: "+",
};

/** Splits a combo on "+", keeping a trailing "+" as the key itself. */
function shortcutTokens(shortcut: string): string[] {
  const trimmed = shortcut.trim();
  if (trimmed === "") return [];
  const tokens = trimmed.split("+").map((t) => t.trim());
  // "mod++" splits into ["mod", "", ""]: the empties were a literal "+".
  const out: string[] = [];
  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i] === "" && i > 0) {
      if (out[out.length - 1] !== "+") out.push("+");
      continue;
    }
    if (tokens[i] !== "") out.push(tokens[i]);
  }
  return out;
}

export function parseShortcut(shortcut: string): ParsedShortcut {
  const parsed: ParsedShortcut = {
    mod: false,
    meta: false,
    ctrl: false,
    alt: false,
    shift: false,
    key: "",
  };
  for (const token of shortcutTokens(shortcut)) {
    const lower = token.toLowerCase();
    const modifier = MODIFIER_TOKENS[lower];
    if (modifier) parsed[modifier] = true;
    else parsed.key = KEY_ALIASES[lower] ?? lower;
  }
  return parsed;
}

/** Whether a keydown is the parsed combo. The physical key (`code`) stands
 *  in when `key` is not a Latin letter or digit — non-Latin layouts
 *  (Cyrillic, Greek) — or whenever ⌥ is held, since a Mac ⌥ combo usually
 *  turns `key` into a symbol but not on every layout. Without ⌥ a Latin
 *  letter is taken at face value, so on Dvorak or AZERTY the key that types
 *  "t" is never read as the physical K. */
export function matchesShortcut(
  e: Pick<KeyboardEvent, "key" | "code" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
  parsed: ParsedShortcut,
): boolean {
  if (parsed.key === "") return false;
  const key = e.key.toLowerCase();
  const byCode =
    parsed.key.length === 1 &&
    /[a-z0-9]/.test(parsed.key) &&
    (e.altKey || !/^[a-z0-9]$/i.test(e.key)) &&
    e.code.toLowerCase() === (/[a-z]/.test(parsed.key) ? `key${parsed.key}` : `digit${parsed.key}`);
  if (key !== parsed.key && !byCode) return false;
  if (parsed.mod) {
    if (!e.metaKey && !e.ctrlKey) return false;
  } else if (e.metaKey !== parsed.meta || e.ctrlKey !== parsed.ctrl) {
    return false;
  }
  if (e.altKey !== parsed.alt) return false;
  // Shift is implied by an uppercase letter or a shifted symbol, so it is
  // only checked when the combo names it.
  if (parsed.shift && !e.shiftKey) return false;
  return true;
}

/** Whether the parsed combo holds any modifier, i.e. is safe to fire while
 *  typing in a field. A bare key stays out of inputs. */
export function hasModifier(parsed: ParsedShortcut): boolean {
  return parsed.mod || parsed.meta || parsed.ctrl || parsed.alt;
}

const CAP_LABELS: Record<string, { mac: string; other: string }> = {
  mod: { mac: "⌘", other: "Ctrl" },
  meta: { mac: "⌘", other: "Win" },
  ctrl: { mac: "⌃", other: "Ctrl" },
  alt: { mac: "⌥", other: "Alt" },
  shift: { mac: "⇧", other: "Shift" },
  enter: { mac: "↵", other: "Enter" },
  escape: { mac: "Esc", other: "Esc" },
  backspace: { mac: "⌫", other: "Backspace" },
  delete: { mac: "⌦", other: "Del" },
  tab: { mac: "⇥", other: "Tab" },
  " ": { mac: "Space", other: "Space" },
  arrowup: { mac: "↑", other: "↑" },
  arrowdown: { mac: "↓", other: "↓" },
  arrowleft: { mac: "←", other: "←" },
  arrowright: { mac: "→", other: "→" },
};

const PREFORMATTED = /^[⌘⌃⌥⇧↵⌫⌦⇥↑↓←→]+[A-Za-z0-9]?$/;

/** The caps a shortcut displays, in order: modifiers as symbols on a Mac and
 *  words elsewhere, letters upper-cased, named keys spelled out. */
export function formatShortcut(shortcut: string, mac: boolean): string[] {
  const caps: string[] = [];
  for (const token of shortcutTokens(shortcut)) {
    if (PREFORMATTED.test(token)) {
      caps.push(...Array.from(token));
      continue;
    }
    const lower = token.toLowerCase();
    const modifier = MODIFIER_TOKENS[lower];
    const name = modifier ?? KEY_ALIASES[lower] ?? lower;
    const cap = CAP_LABELS[name];
    if (cap) caps.push(mac ? cap.mac : cap.other);
    else if (name.length === 1) caps.push(name.toUpperCase());
    else caps.push(name.charAt(0).toUpperCase() + name.slice(1));
  }
  return caps;
}

/** Whether the page runs on a Mac — the platform that draws ⌘ and ⌥. */
export function isMacPlatform(): boolean {
  if (typeof navigator === "undefined") return false;
  const nav = navigator as Navigator & { userAgentData?: { platform?: string } };
  const platform = nav.userAgentData?.platform ?? nav.platform ?? "";
  return /mac|iphone|ipad|ipod/i.test(platform);
}

// The platform never changes, so it is read once and served as a store
// snapshot: the server (and hydration) draw ⌘, other platforms swap to
// words in the render that follows, with no state or effect per cap.
let macPlatform: boolean | null = null;
const readMac = () => (macPlatform ??= isMacPlatform());
const serverMac = () => true;
const subscribeNever = () => () => {};

/** Whether to draw ⌘ and ⌥ rather than Ctrl and Alt. */
export function useIsMac(): boolean {
  return useSyncExternalStore(subscribeNever, readMac, serverMac);
}

// ---------------------------------------------------------------------------
// Fields and dialogs
// ---------------------------------------------------------------------------

/** Whether a key event's target takes typing: an input, textarea, select or
 *  contenteditable element. A bare-key hotkey stays out of these. */
export function isEditable(target: EventTarget | null): boolean {
  const el = target as Partial<HTMLElement> | null;
  if (!el || typeof el.tagName !== "string") return false;
  return (
    el.tagName === "INPUT" ||
    el.tagName === "TEXTAREA" ||
    el.tagName === "SELECT" ||
    el.isContentEditable === true
  );
}

// Radix dialogs (Dialog, AlertDialog, the command menu) mark their open
// panel with data-state="open"; other libraries use aria-modal or <dialog>.
const OPEN_DIALOG =
  '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [aria-modal="true"], dialog[open]';

/** Whether a dialog is open on the page: one a `"dialog"` hotkey says is
 *  open, or any open `role="dialog"` / `role="alertdialog"` panel. */
export function isDialogOpen(): boolean {
  if (bindings.some((b) => b.scope === "dialog" && b.isOpen())) return true;
  return typeof document !== "undefined" && document.querySelector(OPEN_DIALOG) !== null;
}

// ---------------------------------------------------------------------------
// useHotkey
// ---------------------------------------------------------------------------

/**
 * - `"global"`: a page-wide shortcut, such as a nav destination's. It does
 *   not fire while a dialog is open.
 * - `"dialog"`: the combo that opens and closes a dialog (the ⌘K palette).
 *   While its dialog is open (`open`) it answers first, so the press closes
 *   it; while another dialog is open it does nothing; otherwise it wins over
 *   a `"global"` hotkey on the same combo.
 */
export type HotkeyScope = "global" | "dialog";

export interface HotkeyOptions {
  /** @default "global" */
  scope?: HotkeyScope;
  /** Fire while focus is in a text field. Defaults to `true` for a combo
   *  with a modifier (⌘, Ctrl, ⌥) and `false` for a bare key such as "/". */
  inFields?: boolean;
  /** For a `"dialog"` hotkey: whether its dialog is open. */
  open?: boolean;
  /** Answer only while focus is inside this element, e.g. a demo or a pane
   *  that must not take an app-wide combo. An open dialog's own hotkey
   *  answers from anywhere. */
  within?: RefObject<HTMLElement | null>;
  /** `false` unbinds the hotkey. @default true */
  enabled?: boolean;
}

interface Binding {
  parsed: ParsedShortcut;
  scope: HotkeyScope;
  inFields: () => boolean;
  isOpen: () => boolean;
  inScope: () => boolean;
  fire: (e: KeyboardEvent) => void;
}

/** Every bound hotkey, in the order it was bound. */
const bindings: Binding[] = [];

/**
 * The one precedence: at most one binding answers a press.
 * 1. A `"dialog"` hotkey whose dialog is open (the press closes it).
 * 2. Otherwise, while any dialog is open, nothing: the page behind a modal
 *    takes no shortcuts.
 * 3. Otherwise the most recently bound `"dialog"` hotkey in scope, then the
 *    most recently bound `"global"` one.
 */
function pickBinding(hits: Binding[]): Binding | undefined {
  const closing = hits.find((b) => b.scope === "dialog" && b.isOpen());
  if (closing) return closing;
  if (isDialogOpen()) return undefined;
  const latestFirst = hits.slice().reverse();
  return (
    latestFirst.find((b) => b.scope === "dialog" && b.inScope()) ??
    latestFirst.find((b) => b.scope === "global" && b.inScope())
  );
}

function onKeyDown(e: KeyboardEvent) {
  if (e.repeat || e.defaultPrevented) return;
  const inField = isEditable(e.target);
  const hits = bindings.filter((b) => matchesShortcut(e, b.parsed) && (!inField || b.inFields()));
  if (hits.length === 0) return;
  const winner = pickBinding(hits);
  if (!winner) return;
  e.preventDefault();
  winner.fire(e);
}

function bind(binding: Binding): () => void {
  if (bindings.length === 0) window.addEventListener("keydown", onKeyDown);
  bindings.push(binding);
  return () => {
    const i = bindings.indexOf(binding);
    if (i !== -1) bindings.splice(i, 1);
    if (bindings.length === 0) window.removeEventListener("keydown", onKeyDown);
  };
}

// Layout effects have no server counterpart; on the server they are no-ops.
const useIsoLayoutEffect = typeof window === "undefined" ? useEffect : useLayoutEffect;

/**
 * Binds `combo` (in the shortcut syntax, e.g. `"mod+,"`) to `handler` while
 * the component is mounted. All hotkeys share one window listener and the
 * precedence in `HotkeyScope`; the winning press is `preventDefault`ed. A
 * repeat or an already-handled (`defaultPrevented`) press is ignored.
 * `null`, an empty combo or `enabled: false` binds nothing.
 */
export function useHotkey(
  combo: string | null | undefined,
  handler: (event: KeyboardEvent) => void,
  options: HotkeyOptions = {},
): void {
  // Read through a ref so a new handler or options object each render does
  // not rebind: rebinding moves the hotkey to the end of the order, which
  // would change who answers a shared combo.
  const latest = useRef({ handler, options });
  useIsoLayoutEffect(() => {
    latest.current = { handler, options };
  });
  const scope = options.scope ?? "global";
  const enabled = options.enabled ?? true;

  useEffect(() => {
    if (!combo || !enabled) return;
    const parsed = parseShortcut(combo);
    if (parsed.key === "") return;
    return bind({
      parsed,
      scope,
      inFields: () => latest.current.options.inFields ?? hasModifier(parsed),
      isOpen: () => latest.current.options.open ?? false,
      inScope: () => {
        const within = latest.current.options.within?.current;
        return !within || within.contains(document.activeElement);
      },
      fire: (e) => latest.current.handler(e),
    });
  }, [combo, scope, enabled]);
}
