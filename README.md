# daggerverse

My personal collection of [Dagger](https://dagger.io) modules.

## Modules

### [flutter-container](./flutter-container/)

Builds Flutter Docker images — flutter base, Android SDK, and emulator — and publishes them as multi-arch images to `ghcr.io/daniel-naegele/flutter`. Prebuilt images are available for immediate use without having to build locally.

### [flutter](./flutter/)

Dagger CI tasks for Flutter projects: static analysis, unit tests with coverage, Dart Code Metrics, license checks, and Android release builds via Fastlane. Each task accepts an optional prebuilt image from `flutter-container` to skip the local build step.

### [quarto](./quarto/)

Renders [Quarto](https://quarto.org) documentation projects. `Render` wraps `quarto render` directly for full control; `BuildDocs` is a sensible-default convenience — render a project's `docs/` subdirectory (or another via `docsDir`) and get the output back — callable directly from CI with no local Dagger module required:

```bash
dagger call -m github.com/daniel-naegele/daggerverse/quarto build-docs --source . export --path public
```
