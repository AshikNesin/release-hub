# Vendored: Fluid Functionalism command menu

Source: <https://www.fluidfunctionalism.com/docs/command-menu> (shadcn registry
`https://www.fluidfunctionalism.com/r/command-menu.json` and its registry
dependencies), MIT — see [`LICENSE`](./LICENSE). Configured as preset `kd8G`:
⌘K shortcut, descriptions, shortcut caps, suggestions, tabs and footer on,
filters off, rounded shape, default size.

The public entry is `@groot/ui/command-menu` (`packages/ui/src/command-menu.tsx`).
Everything in this folder is its private support code — dialog, tabs, scroll
area, button, fluid-hover hook and the shape/size/icon/surface contexts — kept
outside `src/` so it stays off the kit's public export map (see
`scripts/check-kit-exports.ts`). Do not import it from other packages.

Local changes from upstream:

- `@/…` alias imports rewritten to relative paths; `@radix-ui/react-*` imports
  rewritten to the `radix-ui` umbrella package (the Radix flavor is used
  because `@groot/ui` is built on Radix).
- Square `h-N w-N` sizing in `button.tsx` rewritten to `size-N`
  (`pnpm check:tokens`).
- Formatted with `vp fmt`.

The CSS the registry ships (surface, hover, tint and shadow tokens, scroll
fade/divider, tab scrollbar hiding, button spinner keyframes) lives in
`packages/shell/src/index.css` under "Fluid Functionalism tokens", mapped onto
the groot palette.

To update: re-fetch the registry items, re-apply the changes above, and run
`pnpm check`, `pnpm check:tokens`, `pnpm typecheck:client` and
`pnpm test:client`.
