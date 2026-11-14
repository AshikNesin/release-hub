# groot-mobile-app

A minimal native Android (Kotlin + Jetpack Compose) and iOS (Swift + SwiftUI) boilerplate that does for mobile apps what [groot](https://github.com/AshikNesin/groot) does for web apps: improvements land here once and sync down to every child app, and fixes made in a child go back upstream as PRs.

**Status: initial version.** The Android and iOS kits, the reference apps, sync, checks and CI are built, and CI builds and tests both platforms. Details are in §11 of the design doc.

- Design: [`docs/rfcs/0001-groot-mobile-app.md`](docs/rfcs/0001-groot-mobile-app.md)
- Discussion: [issue #1](https://github.com/AshikNesin/groot-mobile-app/issues/1)
- Rules for agents and humans: [`AGENTS.md`](AGENTS.md)
- Setup: [`docs/quick-start.md`](docs/quick-start.md)

```bash
pnpm install
pnpm check            # repo checks + Android lint and unit tests
pnpm screenshots      # renders the reference screens (light + dark) to android/app/build/outputs/roborazzi/
pnpm size:android     # release download size vs. the 3 MB budget
```

Start a new app with `scripts/new-app.sh ../my-app [--platforms android]`, then `pnpm groot:setup` inside it.
