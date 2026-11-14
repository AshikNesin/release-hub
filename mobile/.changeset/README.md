# Changesets

Add one with `pnpm changeset` in every PR, as in groot. CI fails a PR without one (`changeset status`). For a
docs- or CI-only PR, add an empty one with `pnpm changeset --empty`. Agents committing straight to `main` add
the changeset in the same commit.

- `patch`: fixes. `minor`: new kit features. `major`: a breaking kit API change, with a migration note.
- Merging the "Version Packages" PR bumps the version, writes `CHANGELOG.md`, tags `v*` and creates the
  GitHub Release that child apps sync from.
- Pending changesets (`.changeset/*.md`) are app-owned and never sync to children.
